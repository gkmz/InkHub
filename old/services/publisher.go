package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gkmz/InkHub/old/config"
)

// PublishResult 发布结果
type PublishResult struct {
	OriginalContent string
	PublishContent  string
	UploadedImages  []string
	Errors          []string
}

// imageRef holds a resolved local image reference ready for upload
type imageRef struct {
	absPath   string // absolute local path
	cleanPath string // key used in replacement (the raw src/filename as it appears in markdown)
	isWiki    bool   // true = Obsidian ![[...]] syntax
}

// PublishArticle 处理文章发布逻辑
// 1. 扫描 markdown 中的本地图片（支持标准格式和 Obsidian WikiLink 格式）
// 2. 上传到 GitHub
// 3. 替换链接
func PublishArticle(postPath string, projectRoot string) (*PublishResult, error) {
	contentBytes, err := os.ReadFile(postPath)
	if err != nil {
		return nil, err
	}
	content := string(contentBytes)

	mdDir := filepath.Dir(postPath)
	assetsDir := filepath.Join(projectRoot, "assets")

	// 0. 预处理 Mermaid 代码块：先转成本地 SVG，再走后续图片上传流程
	content, err = PreprocessMermaidBlocks(content, projectRoot, mdDir, "handdrawn")
	if err != nil {
		return nil, err
	}

	// 收集所有图片引用，去重
	uniquePaths := make(map[string]bool)
	var refs []imageRef

	// 1. 标准 Markdown 图片: ![alt](path)
	reStd := regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`)
	for _, match := range reStd.FindAllStringSubmatch(content, -1) {
		if len(match) < 3 {
			continue
		}
		imgURL := match[2]
		if strings.HasPrefix(imgURL, "http") {
			continue
		}
		cleanPath := strings.Split(imgURL, " ")[0]
		if uniquePaths[cleanPath] {
			continue
		}

		absPath := resolveStandardImagePath(cleanPath, mdDir, projectRoot)
		if strings.Contains(absPath, "/content/static/") {
			absPath = strings.Replace(absPath, "/content/static/", "/static/", 1)
		}
		absPath = materializeExternalImageForPublish(absPath, projectRoot)

		uniquePaths[cleanPath] = true
		refs = append(refs, imageRef{absPath: absPath, cleanPath: cleanPath, isWiki: false})
	}

	// 2. Obsidian WikiLink 图片: ![[filename]] 或 ![[filename|alt]]
	reWiki := regexp.MustCompile(`!\[\[([^\]|]+?)(?:\|[^\]]+)?\]\]`)
	for _, match := range reWiki.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		filename := strings.TrimSpace(match[1])
		if uniquePaths[filename] {
			continue
		}

		absPath := filepath.Join(assetsDir, filepath.Base(filename))
		uniquePaths[filename] = true
		refs = append(refs, imageRef{absPath: absPath, cleanPath: filename, isWiki: true})
	}

	log.Printf("Debug: Scanning article %s, found %d image refs\n", postPath, len(refs))

	uploader := &GitHubUploader{}
	result := &PublishResult{
		OriginalContent: content,
	}

	// urlMap: cleanPath -> CDN URL
	urlMap := make(map[string]string)

	for _, ref := range refs {
		log.Printf("Debug: Processing image: %s -> %s (wiki=%v)\n", ref.cleanPath, ref.absPath, ref.isWiki)

		if _, err := os.Stat(ref.absPath); os.IsNotExist(err) {
			log.Printf("Debug: File not found at %s\n", ref.absPath)
			result.Errors = append(result.Errors, fmt.Sprintf("Image not found: %s", ref.cleanPath))
			continue
		}

		remotePath, _ := filepath.Rel(projectRoot, ref.absPath)
		if config.AppConfig.GitHubPathPrefix != "" {
			remotePath = filepath.Join(config.AppConfig.GitHubPathPrefix, remotePath)
		}

		cdnURL, err := uploader.Upload(ref.absPath, remotePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Upload failed for %s: %v", ref.cleanPath, err))
		} else {
			urlMap[ref.cleanPath] = cdnURL
			result.UploadedImages = append(result.UploadedImages, cdnURL)
		}
	}

	// 替换内容：先替换 WikiLink，再替换标准格式
	newContent := content

	// WikiLink 替换: ![[filename]] -> ![filename](cdnURL)  /  ![[filename|alt]] -> ![alt](cdnURL)
	reWikiReplace := regexp.MustCompile(`!\[\[([^\]|]+?)(?:\|([^\]]+))?\]\]`)
	newContent = reWikiReplace.ReplaceAllStringFunc(newContent, func(match string) string {
		sub := reWikiReplace.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		filename := strings.TrimSpace(sub[1])
		alt := strings.TrimSpace(sub[2])
		if alt == "" {
			alt = filename
		}
		cdnURL, ok := urlMap[filename]
		if !ok {
			return match
		}
		return fmt.Sprintf("![%s](%s)", alt, cdnURL)
	})

	// 标准格式替换: (localPath -> (cdnURL
	for local, remote := range urlMap {
		// 只替换非 WikiLink 来源的路径（WikiLink 来源的 cleanPath 不会出现在 `(...)` 中）
		newContent = strings.ReplaceAll(newContent, "("+local, "("+remote)
	}

	result.PublishContent = newContent
	return result, nil
}

// resolveStandardImagePath 解析 Markdown 原生图片路径，支持回退到 projectRoot/assets。
func resolveStandardImagePath(imgPath, mdDir, projectRoot string) string {
	clean := strings.TrimSpace(imgPath)
	clean = strings.Split(clean, " ")[0]

	var candidates []string
	if filepath.IsAbs(clean) {
		candidates = append(candidates, filepath.Clean(clean))
	} else {
		candidates = append(candidates, filepath.Clean(filepath.Join(mdDir, clean)))
	}

	normalized := filepath.ToSlash(clean)
	normalized = strings.TrimPrefix(normalized, "./")
	if idx := strings.Index(normalized, "assets/"); idx >= 0 {
		assetTail := normalized[idx+len("assets/"):]
		if assetTail != "" {
			candidates = append(candidates, filepath.Join(projectRoot, "assets", filepath.FromSlash(assetTail)))
		}
	}

	base := filepath.Base(normalized)
	if base != "." && base != "/" && base != "" {
		candidates = append(candidates, filepath.Join(projectRoot, "assets", base))
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	// 保留原行为：至少返回基于文章目录的解析路径，便于错误日志定位。
	if filepath.IsAbs(clean) {
		return filepath.Clean(clean)
	}
	return filepath.Clean(filepath.Join(mdDir, clean))
}

// materializeExternalImageForPublish 将 projectRoot 外部图片复制到 projectRoot/assets/imported，
// 这样后续 GitHub 上传路径稳定且不包含 ../。
func materializeExternalImageForPublish(absPath, projectRoot string) string {
	clean := filepath.Clean(absPath)
	if strings.HasPrefix(clean, projectRoot) {
		return clean
	}

	info, err := os.Stat(clean)
	if err != nil || info.IsDir() {
		return clean
	}

	in, err := os.Open(clean)
	if err != nil {
		return clean
	}
	defer in.Close()

	h := sha256.New()
	if _, err := io.Copy(h, in); err != nil {
		return clean
	}
	sum := hex.EncodeToString(h.Sum(nil))[:16]

	ext := strings.ToLower(filepath.Ext(clean))
	if ext == "" {
		ext = ".img"
	}

	targetDir := filepath.Join(projectRoot, "assets", "imported")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return clean
	}
	target := filepath.Join(targetDir, "external-"+sum+ext)
	if _, err := os.Stat(target); err == nil {
		return target
	}

	if _, err := in.Seek(0, 0); err != nil {
		return clean
	}
	out, err := os.Create(target)
	if err != nil {
		return clean
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return clean
	}
	return target
}
