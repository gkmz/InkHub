package markdown

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/gkmz/InkHub/old/config"
	"github.com/gkmz/InkHub/old/models"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

// Processor Markdown 处理器
type Processor struct {
	md          goldmark.Markdown
	articles    []models.Article
	projectRoot string
	assetsDir   string // 全局 assets 目录绝对路径
}

// NewProcessor 创建 Markdown 处理器
func NewProcessor(articles []models.Article, projectRoot string) *Processor {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
			highlighting.NewHighlighting(
				highlighting.WithStyle("tokyonight-night"),
				highlighting.WithFormatOptions(
					html.WithLineNumbers(false),
					html.WithClasses(false), // 使用内联样式，确保复制时保留代码高亮
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			goldmarkhtml.WithHardWraps(),
			goldmarkhtml.WithXHTML(),
			goldmarkhtml.WithUnsafe(),
		),
	)

	return &Processor{
		md:          md,
		articles:    articles,
		projectRoot: projectRoot,
		assetsDir:   filepath.Join(projectRoot, "assets"),
	}
}

// Convert 转换 Markdown 为 HTML
func (p *Processor) Convert(content string) (string, error) {
	var buf strings.Builder
	if err := p.md.Convert([]byte(content), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RemoveFrontmatter 移除 Frontmatter
func (p *Processor) RemoveFrontmatter(content string) string {
	content = strings.TrimPrefix(content, "\ufeff") // 处理 BOM
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return content
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	endIndex := -1
	startIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if startIndex == -1 {
				startIndex = i
			} else {
				endIndex = i
				break
			}
		}
	}

	if startIndex != -1 && endIndex != -1 {
		return strings.Join(lines[endIndex+1:], "\n")
	}
	return content
}

// RemoveTitle 移除内容中的第一个 H1 标题
func (p *Processor) RemoveTitle(content string) string {
	lines := strings.Split(content, "\n")
	var newLines []string
	removed := false
	for _, line := range lines {
		if !removed && strings.HasPrefix(strings.TrimSpace(line), "# ") {
			removed = true
			continue
		}
		newLines = append(newLines, line)
	}
	return strings.Join(newLines, "\n")
}

// ReplaceRelRef 替换 Hugo relref shortcode
func (p *Processor) ReplaceRelRef(content string) string {
	// 改进正则：支持 {{< >}} 和 {{% %}} 两种语法，支持文件名中的空格和特殊字符
	// 要求必须使用引号包裹路径
	re := regexp.MustCompile(`\{\{[<%]\s*(?:relref|ref)\s+["']([^"']+)["']\s*[>%]\}\}`)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		submatch := re.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		refPath := submatch[1]

		var anchor string
		if idx := strings.LastIndex(refPath, "#"); idx != -1 {
			anchor = refPath[idx:]
			refPath = refPath[:idx]
		}

		art := p.findArticle(refPath)
		if art != nil {
			if config.AppConfig.BaseURL != "" {
				baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
				targetSlug := art.Slug
				if targetSlug == "" {
					targetSlug = art.ID
				}
				return fmt.Sprintf("%s/posts/%s/%s/%s", baseURL, art.Series, targetSlug, anchor)
			}
			return fmt.Sprintf("/article/%s%s", art.ID, anchor)
		}

		return fmt.Sprintf("#relref-not-found-%s", refPath)
	})
}

// findArticle 查找文章
func (p *Processor) findArticle(path string) *models.Article {
	path = filepath.ToSlash(path)
	pathBase := filepath.Base(path) // 提取文件名部分

	for i := range p.articles {
		art := &p.articles[i]
		artRelPath := filepath.ToSlash(art.RelPath)
		artBase := filepath.Base(artRelPath)

		// 1. 完全匹配相对路径
		if artRelPath == path {
			return art
		}

		// 2. 匹配文件名（忽略目录结构）
		if artBase == pathBase {
			return art
		}

		// 3. 匹配文件名（忽略扩展名差异）
		pathExt := filepath.Ext(pathBase)
		artExt := filepath.Ext(artBase)
		if pathExt != "" && artExt != "" {
			pathNameNoExt := strings.TrimSuffix(pathBase, pathExt)
			artNameNoExt := strings.TrimSuffix(artBase, artExt)
			if pathNameNoExt == artNameNoExt {
				return art
			}
		}

		// 4. 后缀匹配（支持部分路径匹配）
		if strings.HasSuffix(artRelPath, path) {
			return art
		}
	}

	return nil
}

// ResolveAbstract 将 Obsidian [!abstract] callout 转换为带标记的 HTML 块
// 复制时会被 JS 自动剔除，不影响页面展示
func (p *Processor) ResolveAbstract(content string) string {
	// 匹配整个 callout 块：
	//   > [!abstract]
	//   > 内容行1
	//   > 内容行2（可选）
	re := regexp.MustCompile(`(?m)^> \[!abstract\][^\n]*\n((?:> [^\n]*\n?)*)`)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		// 提取每行 "> " 之后的内容
		lines := strings.Split(strings.TrimRight(match, "\n"), "\n")
		var bodyLines []string
		for i, line := range lines {
			if i == 0 {
				// 第一行是 > [!abstract]，跳过
				continue
			}
			bodyLines = append(bodyLines, strings.TrimPrefix(line, "> "))
		}
		body := strings.Join(bodyLines, "<br>")
		return fmt.Sprintf(
			`<div data-abstract class="obsidian-abstract"><strong>摘要</strong><br>%s</div>`+"\n\n",
			body,
		)
	})
}

// ResolveWikiLinks 将 Obsidian WikiLink 图片格式转换为标准 Markdown 格式
// ![[filename.png]] -> ![filename](/_local_fs/assets/filename.png)
// ![[filename.png|alt]] -> ![alt](/_local_fs/assets/filename.png)
func (p *Processor) ResolveWikiLinks(content string) string {
	reWiki := regexp.MustCompile(`!\[\[([^\]|]+?)(?:\|([^\]]+))?\]\]`)
	return reWiki.ReplaceAllStringFunc(content, func(match string) string {
		sub := reWiki.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		filename := strings.TrimSpace(sub[1])
		alt := strings.TrimSpace(sub[2])
		if alt == "" {
			alt = filename
		}

		// 在 assets 目录中查找文件
		absPath := filepath.Join(p.assetsDir, filepath.Base(filename))
		if _, err := os.Stat(absPath); err != nil {
			// 找不到文件，保留原文本（避免渲染出破损图片）
			return match
		}

		relPath, err := filepath.Rel(p.projectRoot, absPath)
		if err != nil {
			return match
		}
		return fmt.Sprintf("![%s](/_local_fs/%s)", alt, filepath.ToSlash(relPath))
	})
}

// ProcessImagePaths 处理图片路径
func (p *Processor) ProcessImagePaths(htmlContent, articleDir string) string {
	reImg := regexp.MustCompile(`src=["']([^"']+)["']`)

	return reImg.ReplaceAllStringFunc(htmlContent, func(match string) string {
		parts := strings.SplitN(match, "=", 2)
		if len(parts) != 2 {
			return match
		}
		quote := parts[1][0:1]
		src := parts[1][1 : len(parts[1])-1]

		if strings.HasPrefix(src, "http") || strings.HasPrefix(src, "//") {
			return match
		}

		absImgPath := p.resolveLocalImagePath(src, articleDir)
		if absImgPath == "" {
			if assetTail := extractAssetsTail(src); assetTail != "" {
				newSrc := fmt.Sprintf("/_project_assets/%s", assetTail)
				return fmt.Sprintf("src=%s%s%s", quote, newSrc, quote)
			}
			return match
		}

		if strings.Contains(absImgPath, "/content/static/") {
			absImgPath = strings.Replace(absImgPath, "/content/static/", "/static/", 1)
		}

		// 若图片位于 projectRoot 之外，先复制到 projectRoot/assets/imported，确保可通过 /_local_fs 访问
		if !strings.HasPrefix(absImgPath, p.projectRoot) {
			if localized, err := p.materializeExternalImage(absImgPath); err == nil {
				absImgPath = localized
			} else {
				return match
			}
		}

		if strings.HasPrefix(absImgPath, p.projectRoot) {
			relPath, err := filepath.Rel(p.projectRoot, absImgPath)
			if err == nil {
				relPath = filepath.ToSlash(relPath)
				newSrc := fmt.Sprintf("/_local_fs/%s", relPath)
				return fmt.Sprintf("src=%s%s%s", quote, newSrc, quote)
			}
		}

		if assetTail := extractAssetsTail(src); assetTail != "" {
			newSrc := fmt.Sprintf("/_project_assets/%s", assetTail)
			return fmt.Sprintf("src=%s%s%s", quote, newSrc, quote)
		}

		return match
	})
}

// resolveLocalImagePath 解析 Markdown 原生图片路径，支持路径过深时回退到 projectRoot/assets。
func (p *Processor) resolveLocalImagePath(src, articleDir string) string {
	var candidates []string

	if filepath.IsAbs(src) {
		candidates = append(candidates, filepath.Clean(src))
	} else {
		candidates = append(candidates, filepath.Clean(filepath.Join(articleDir, src)))
	}

	normalized := filepath.ToSlash(strings.TrimSpace(src))
	normalized = strings.TrimPrefix(normalized, "./")
	if idx := strings.Index(normalized, "assets/"); idx >= 0 {
		assetTail := normalized[idx+len("assets/"):]
		if assetTail != "" {
			candidates = append(candidates, filepath.Join(p.projectRoot, "assets", filepath.FromSlash(assetTail)))
		}
	}

	base := filepath.Base(normalized)
	if base != "." && base != "/" && base != "" {
		candidates = append(candidates, filepath.Join(p.projectRoot, "assets", base))
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func extractAssetsTail(src string) string {
	normalized := filepath.ToSlash(strings.TrimSpace(src))
	normalized = strings.TrimPrefix(normalized, "./")
	idx := strings.Index(normalized, "assets/")
	if idx < 0 {
		return ""
	}
	tail := normalized[idx+len("assets/"):]
	tail = strings.TrimPrefix(tail, "/")
	return tail
}

// materializeExternalImage 将 projectRoot 外部图片复制到 projectRoot/assets/imported，供预览访问。
func (p *Processor) materializeExternalImage(absPath string) (string, error) {
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("invalid external image path: %s", absPath)
	}

	file, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))[:16]
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext == "" {
		ext = ".img"
	}

	targetDir := filepath.Join(p.projectRoot, "assets", "imported")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	targetPath := filepath.Join(targetDir, "external-"+sum+ext)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}

	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	out, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}
	return targetPath, nil
}

// OptimizeListItems 优化列表项样式
func (p *Processor) OptimizeListItems(htmlContent string) string {
	reStrong := regexp.MustCompile(`<li><strong>([^<]+)</strong>`)
	htmlContent = reStrong.ReplaceAllString(htmlContent, `<li><span class="li-bold">$1</span>`)

	reLiContent := regexp.MustCompile(`<li>(.*?)</li>`)
	htmlContent = reLiContent.ReplaceAllString(htmlContent, `<li><span class="li-text">$1</span></li>`)

	return htmlContent
}
