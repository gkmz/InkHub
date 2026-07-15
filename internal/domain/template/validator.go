package template

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"gopkg.in/yaml.v3"
)

const (
	maxTemplateFiles = 200
	maxTemplateBytes = 25 << 20
	maxFileBytes     = 8 << 20
)

var (
	templateIDPattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)
	templateTargetPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
	semverPattern         = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)
	digestPattern         = regexp.MustCompile(`^[0-9a-f]{64}$`)
	regexpColor           = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)
)

// ValidateDirectory 对已解包模板目录执行完整结构和安全校验。
func ValidateDirectory(root string) (Validated, error) {
	manifestContent, err := os.ReadFile(filepath.Join(root, "template.yaml"))
	if err != nil {
		return Validated{}, fmt.Errorf("读取 template.yaml: %w", err)
	}
	manifest, err := parseManifest(manifestContent)
	if err != nil {
		return Validated{}, err
	}
	if filepath.Base(root) != manifest.ID {
		return Validated{}, fmt.Errorf("模板根目录必须与 id 一致")
	}
	files, total, err := collectFiles(root)
	if err != nil {
		return Validated{}, err
	}
	if len(files) > maxTemplateFiles || total > maxTemplateBytes {
		return Validated{}, fmt.Errorf("模板文件数量或总大小超过限制")
	}
	if err := validateDigests(root, manifest, files); err != nil {
		return Validated{}, err
	}
	if err := validateAssets(root, manifest, files); err != nil {
		return Validated{}, err
	}
	if err := validatePreview(filepath.Join(root, manifest.Preview.Image)); err != nil {
		return Validated{}, err
	}
	cssContent, err := os.ReadFile(filepath.Join(root, manifest.Entry))
	if err != nil {
		return Validated{}, fmt.Errorf("读取模板 CSS: %w", err)
	}
	if err := validateCSS(string(cssContent), manifest.Variables); err != nil {
		return Validated{}, err
	}
	digest := sha256.New()
	for _, path := range files {
		content, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		_, _ = digest.Write([]byte(path))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(content)
	}
	return Validated{Root: root, Manifest: manifest, CSS: string(cssContent), Digest: hex.EncodeToString(digest.Sum(nil))}, nil
}

func validateAssets(root string, manifest Manifest, files []string) error {
	declared := map[string]bool{}
	for _, asset := range manifest.Assets {
		if !safeRelativePath(asset.Path) || !strings.HasPrefix(asset.Path, "assets/") || asset.Source == "" || asset.License == "" || !digestPattern.MatchString(asset.SHA256) || declared[asset.Path] {
			return fmt.Errorf("模板 asset 声明无效: %s", asset.Path)
		}
		source, err := url.Parse(asset.Source)
		if err != nil || source.Scheme != "https" || source.Host == "" {
			return fmt.Errorf("模板 asset 来源必须是 HTTPS: %s", asset.Path)
		}
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(asset.Path)))
		if err != nil {
			return fmt.Errorf("读取模板 asset: %w", err)
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != asset.SHA256 {
			return fmt.Errorf("模板 asset 摘要不匹配: %s", asset.Path)
		}
		detected := mimetype.Detect(content).String()
		if detected != asset.MediaType {
			return fmt.Errorf("模板 asset 媒体类型不匹配: %s", asset.Path)
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(content))
		if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
			return fmt.Errorf("模板 asset 图片无效或像素过大: %s", asset.Path)
		}
		if asset.MediaType == "image/gif" {
			decoded, err := gif.DecodeAll(bytes.NewReader(content))
			if err != nil || len(decoded.Image) != 1 {
				return fmt.Errorf("模板 asset 禁止动画 GIF: %s", asset.Path)
			}
		}
		declared[asset.Path] = true
	}
	for _, path := range files {
		if strings.HasPrefix(path, "assets/") && !declared[path] {
			return fmt.Errorf("模板 asset 文件未声明: %s", path)
		}
	}
	return nil
}

func parseManifest(content []byte) (Manifest, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return Manifest{}, fmt.Errorf("解析模板 manifest: %w", err)
	}
	if err := validateYAMLNode(&document, 0); err != nil {
		return Manifest{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("解码模板 manifest: %w", err)
	}
	if manifest.SpecVersion == "1.0" && manifest.Target == "" && manifest.Format == "" && manifest.Renderer == "" {
		// 1.0 只有微信 CSS 一种语义，加载时升级为显式目标但不改写原模板包。
		manifest.Target = TargetWeChatHTML
		manifest.Format = "css"
		manifest.Renderer = RendererWeChatHTMLV1
		manifest.Compatibility = Compatibility{Providers: []string{"wechat"}, RendererVersion: "1"}
	}
	if (manifest.SpecVersion != "1.0" && manifest.SpecVersion != "1.1") || !templateIDPattern.MatchString(manifest.ID) || !semverPattern.MatchString(manifest.Version) {
		return Manifest{}, fmt.Errorf("模板规范版本、id 或版本号无效")
	}
	if !templateTargetPattern.MatchString(manifest.Target) || !templateTargetPattern.MatchString(manifest.Format) || !templateTargetPattern.MatchString(manifest.Renderer) || len(manifest.Compatibility.Providers) == 0 || manifest.Compatibility.RendererVersion == "" {
		return Manifest{}, fmt.Errorf("模板目标、格式、Renderer 或兼容性无效")
	}
	for _, provider := range manifest.Compatibility.Providers {
		if !templateTargetPattern.MatchString(provider) {
			return Manifest{}, fmt.Errorf("模板兼容 Provider 无效: %s", provider)
		}
	}
	if manifest.Name == "" || manifest.Description == "" || manifest.Author.Name == "" || manifest.License == "" || manifest.InkHubVersion == "" {
		return Manifest{}, fmt.Errorf("模板必填元数据不完整")
	}
	if manifest.Author.URL != "" {
		parsed, err := url.Parse(manifest.Author.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return Manifest{}, fmt.Errorf("模板作者 URL 必须是 HTTPS")
		}
	}
	if manifest.Entry != "styles.css" || manifest.Preview.Markdown == "" || manifest.Preview.Image == "" || len(manifest.Elements) == 0 {
		return Manifest{}, fmt.Errorf("模板入口、预览或元素声明无效")
	}
	if len(manifest.Variables) > 20 {
		return Manifest{}, fmt.Errorf("模板变量超过限制")
	}
	for name, variable := range manifest.Variables {
		if err := validateVariable(name, variable); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func validateYAMLNode(node *yaml.Node, depth int) error {
	if depth > 32 {
		return fmt.Errorf("模板 YAML 嵌套超过限制")
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return fmt.Errorf("模板 YAML 禁止 anchor 和 alias")
	}
	if node.Tag != "" && !strings.HasPrefix(node.Tag, "!!") {
		return fmt.Errorf("模板 YAML 禁止自定义 tag")
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index].Value
			if seen[key] {
				return fmt.Errorf("模板 YAML key 重复: %s", key)
			}
			seen[key] = true
		}
	}
	for _, child := range node.Content {
		if err := validateYAMLNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func collectFiles(root string) ([]string, int64, error) {
	var files []string
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("模板禁止符号链接: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileBytes {
			return fmt.Errorf("模板单文件超过限制: %s", path)
		}
		extension := strings.ToLower(filepath.Ext(path))
		allowed := map[string]bool{".yaml": true, ".css": true, ".md": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}
		if !allowed[extension] {
			return fmt.Errorf("模板包含禁止文件类型: %s", path)
		}
		relative, _ := filepath.Rel(root, path)
		files = append(files, filepath.ToSlash(relative))
		total += info.Size()
		return nil
	})
	sort.Strings(files)
	return files, total, err
}

func validateDigests(root string, manifest Manifest, actual []string) error {
	expected := map[string]string{}
	for _, file := range manifest.Files {
		if !safeRelativePath(file.Path) || !digestPattern.MatchString(file.SHA256) || expected[file.Path] != "" {
			return fmt.Errorf("模板文件清单无效: %s", file.Path)
		}
		expected[file.Path] = file.SHA256
	}
	for _, path := range actual {
		if path == "template.yaml" {
			continue
		}
		digestValue, exists := expected[path]
		if !exists {
			return fmt.Errorf("模板存在未声明文件: %s", path)
		}
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != digestValue {
			return fmt.Errorf("模板文件摘要不匹配: %s", path)
		}
		delete(expected, path)
	}
	if len(expected) != 0 {
		return fmt.Errorf("模板文件清单包含缺失文件")
	}
	return nil
}

func safeRelativePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return path != "" && clean == path && !strings.HasPrefix(clean, "../") && clean != ".." && !filepath.IsAbs(path)
}

func validatePreview(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开模板预览图: %w", err)
	}
	defer file.Close()
	config, format, err := image.DecodeConfig(file)
	if err != nil || format != "png" {
		return fmt.Errorf("模板预览图必须是有效 PNG")
	}
	if config.Width < 1200 || config.Width > 2400 || config.Height < 1600 || config.Height > 3200 {
		return fmt.Errorf("模板预览图尺寸不符合规范")
	}
	return nil
}

func validateVariable(name string, variable Variable) error {
	if name == "" || variable.Label == "" || variable.Default == nil {
		return fmt.Errorf("模板变量声明不完整: %s", name)
	}
	switch variable.Type {
	case "color":
		value, ok := variable.Default.(string)
		if !ok || !regexpColor.MatchString(value) {
			return fmt.Errorf("颜色变量默认值无效: %s", name)
		}
	case "font-family", "enum":
		value, ok := variable.Default.(string)
		if !ok || len(variable.Options) < 1 || !contains(variable.Options, value) {
			return fmt.Errorf("枚举变量默认值无效: %s", name)
		}
	case "font-size", "spacing":
		if variable.Min == nil || variable.Max == nil || variable.Unit != "px" {
			return fmt.Errorf("数值变量边界无效: %s", name)
		}
	case "boolean":
		if _, ok := variable.Default.(bool); !ok || !contains([]string{"block", "inline", "inline-block"}, variable.TrueValue) {
			return fmt.Errorf("布尔变量映射无效: %s", name)
		}
	default:
		return fmt.Errorf("不支持模板变量类型: %s", variable.Type)
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
