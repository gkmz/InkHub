package markdown

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/hankmor/mymedia/tools/wechat-preview/config"
	"github.com/hankmor/mymedia/tools/wechat-preview/models"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

// Processor Markdown 处理器
type Processor struct {
	md           goldmark.Markdown
	articles     []models.Article
	projectRoot  string
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
				highlighting.WithStyle("monokai"),
				highlighting.WithFormatOptions(
					html.WithLineNumbers(false),
					html.WithClasses(true),
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
	re := regexp.MustCompile(`\{\{<\s*(?:relref|ref)\s+["']?([^"'\s}]+)["']?\s*>\}\}`)
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

	for i := range p.articles {
		art := &p.articles[i]
		artRelPath := filepath.ToSlash(art.RelPath)
		if artRelPath == path {
			return art
		}

		if strings.HasSuffix(path, artRelPath) {
			marker := "/" + artRelPath
			if strings.HasSuffix(path, marker) {
				return art
			}
		}
	}

	// 尝试替换扩展名
	ext := filepath.Ext(path)
	if ext != "" {
		base := strings.TrimSuffix(path, ext)
		candidates := []string{base + ".md", base + ".adoc"}
		for _, c := range candidates {
			if c == path {
				continue
			}
			for i := range p.articles {
				art := &p.articles[i]
				if filepath.ToSlash(art.RelPath) == c {
					return art
				}
			}
		}
	}

	return nil
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

		var absImgPath string
		if filepath.IsAbs(src) {
			absImgPath = src
		} else {
			absImgPath = filepath.Join(articleDir, src)
		}
		absImgPath = filepath.Clean(absImgPath)

		if strings.Contains(absImgPath, "/content/static/") {
			absImgPath = strings.Replace(absImgPath, "/content/static/", "/static/", 1)
		}

		if strings.HasPrefix(absImgPath, p.projectRoot) {
			relPath, err := filepath.Rel(p.projectRoot, absImgPath)
			if err == nil {
				relPath = filepath.ToSlash(relPath)
				newSrc := fmt.Sprintf("/_local_fs/%s", relPath)
				return fmt.Sprintf("src=%s%s%s", quote, newSrc, quote)
			}
		}

		return match
	})
}

// OptimizeListItems 优化列表项样式
func (p *Processor) OptimizeListItems(htmlContent string) string {
	reStrong := regexp.MustCompile(`<li><strong>([^<]+)</strong>`)
	htmlContent = reStrong.ReplaceAllString(htmlContent, `<li><span class="li-bold">$1</span>`)

	reLiContent := regexp.MustCompile(`<li>(.*?)</li>`)
	htmlContent = reLiContent.ReplaceAllString(htmlContent, `<li><span class="li-text">$1</span></li>`)

	return htmlContent
}
