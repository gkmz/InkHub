package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/hankmor/mymedia/tools/wechat-preview/config"
	"github.com/hankmor/mymedia/tools/wechat-preview/services"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

//go:embed web
var embedFS embed.FS

// Article 文章元数据

// Article 文章元数据
type Article struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Series    string    `json:"series"`
	Path      string    `json:"path"`
	RelPath   string    `json:"relPath"` // 相对 posts 的路径，用于定位图片
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ArticleDetail 文章详情
type ArticleDetail struct {
	Article
	HTML        string `json:"html"`
	RawMarkdown string `json:"rawMarkdown"`
}

var (
	postsDir    string // 相对于 tools/wechat-preview 的路径
	projectRoot string // 项目根目录 (自动探测)
	articles    []Article
	md          goldmark.Markdown
)

func init() {
	// 初始化 Markdown 解析器
	md = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,   // GitHub Flavored Markdown
			extension.Table, // 表格
			extension.Strikethrough,
			extension.TaskList,
			highlighting.NewHighlighting(
				highlighting.WithStyle("monokai"), // 使用高对比度主题
				highlighting.WithFormatOptions(
					chromahtml.WithLineNumbers(false), // 微信里行号可能样式混乱，先关闭
					chromahtml.WithClasses(true),      // 使用 CSS 类
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(), // 允许 HTML 标签
		),
	)
}

func main() {
	// 1. 解析命令行参数
	dirFlag := flag.String("dir", "", "Markdown articles directory (default: current directory)")
	portFlag := flag.String("port", "8080", "Server port")
	flag.Parse()

	config.Load() // 加载配置

	// 2. 确定文章目录优先级：CLI > Env > Default(Current Dir)
	if *dirFlag != "" {
		postsDir = *dirFlag
	} else if config.AppConfig.PostsDir != "" {
		postsDir = config.AppConfig.PostsDir
	} else {
		// 默认为当前目录
		wd, _ := os.Getwd()
		postsDir = wd
	}

	// 转换为绝对路径，方便后续处理
	absPostsDir, err := filepath.Abs(postsDir)
	if err != nil {
		fmt.Printf("Error getting absolute path for %s: %v\n", postsDir, err)
		os.Exit(1)
	}
	postsDir = absPostsDir

	fmt.Printf("Using Posts Dir: %s\n", postsDir)

	// 扫描文章
	if err := scanArticles(); err != nil {
		fmt.Printf("扫描文章失败: %v\n", err)
		os.Exit(1)
	}

	// 3. 自动探测项目根目录
	projectRoot = findProjectRoot(postsDir)
	if projectRoot == "" {
		projectRoot = postsDir // 降级为文章目录
		fmt.Println("Warning: Could not detect project root (no .git or hugo.yaml found). using postsDir as root.")
	}

	fmt.Printf("Using Project Root: %s\n", projectRoot)

	fmt.Println("\n========================================")
	fmt.Printf("   Wechat Preview Tool - CLI Mode\n")
	fmt.Printf("   Articles: %d\n", len(articles))
	fmt.Printf("   Scanning: %s\n", postsDir)
	fmt.Println("========================================\n")

	// 初始化 Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 3. 处理静态资源 (Embed)
	// web/static -> /assets (避免占用 /static)
	staticFS, _ := fs.Sub(embedFS, "web/static")
	r.StaticFS("/assets", http.FS(staticFS))

	// 核心修改：挂载项目根目录到 /_local_fs，用于动态解析图片
	// Security: 仅本地运行，挂载只读文件系统
	if projectRoot != "" {
		r.StaticFS("/_local_fs", gin.Dir(projectRoot, false))
	}

	// 核心修改：映射 posts 目录为静态资源，用于预览本地图片
	r.Static("/posts-static", postsDir)

	// 4. 处理模板 (Embed)
	// web/templates -> templates
	templatesFS, _ := fs.Sub(embedFS, "web/templates")
	r.SetHTMLTemplate(loadTemplates(templatesFS))

	// 路由
	r.GET("/", handleList)
	r.GET("/article/:id", handleArticle)
	r.GET("/api/articles", apiArticles)
	r.GET("/api/articles/:id", apiArticleDetail)
	r.POST("/api/publish/:id", handlePublish)
	r.GET("/chroma.css", handleChromaCSS) // 提供 Chroma 样式

	// 启动服务
	addr := ":" + *portFlag
	fmt.Printf("Starting server on http://localhost%s\n", addr)
	fmt.Printf("Press Ctrl+C to stop.\n")
	r.Run(addr)
}

// findProjectRoot 向上查找项目根目录
func findProjectRoot(startPath string) string {
	curr := startPath
	for {
		// 检查常见标记文件
		markers := []string{".git", "hugo.yaml", "hugo.toml", "go.mod", "package.json"}
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(curr, m)); err == nil {
				return curr
			}
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break // 到达根目录
		}
		curr = parent
	}
	return ""
}

// loadTemplates 从 embed.FS 加载模板
func loadTemplates(fs fs.FS) *template.Template {
	// Gin 的 LoadHTMLGlob 不支持 FS，需要手动 ParseFS
	tmpl, err := template.ParseFS(fs, "*.html")
	if err != nil {
		panic(err)
	}
	return tmpl
}

// scanArticles 扫描文章目录
func scanArticles() error {
	articles = []Article{}

	err := filepath.WalkDir(postsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 处理 .md 和 .adoc 文件
		ext := filepath.Ext(path)
		if d.IsDir() || (ext != ".md" && ext != ".adoc") {
			return nil
		}

		// 读取文件获取标题和 Slug
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		title, slug := extractMetadata(string(content))
		if title == "" {
			title = filepath.Base(path)
		}

		// 如果没有 slug，使用文件名 (无扩展名)
		if slug == "" {
			slug = strings.TrimSuffix(filepath.Base(path), ext)
		}

		// 提取系列名（从目录结构）
		relPath, _ := filepath.Rel(postsDir, path)
		parts := strings.Split(relPath, string(os.PathSeparator))
		series := "其他"
		if len(parts) > 1 {
			series = parts[0]
		}

		// 获取修改时间和创建时间
		updatedAt := time.Now()
		createdAt := time.Now()
		info, err := d.Info()
		if err == nil && info != nil {
			updatedAt = info.ModTime()
			createdAt = getFileCreationTime(path)
		}

		// 生成 ID（去掉 posts/ 前缀和 .md/.adoc 后缀，替换路径分隔符为下划线）
		id := strings.TrimSuffix(relPath, ext)
		id = strings.ReplaceAll(id, string(os.PathSeparator), "_")

		articles = append(articles, Article{
			ID:        id,
			Title:     title,
			Series:    series,
			Path:      path,
			RelPath:   relPath,
			Slug:      slug,
			UpdatedAt: updatedAt,
			CreatedAt: createdAt,
		})

		return nil
	})

	// 按时间倒序排序 (最近的在前面)
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].CreatedAt.After(articles[j].CreatedAt)
	})

	return err
}

// extractMetadata 从 Markdown/Adoc 内容提取标题和 Slug
func extractMetadata(content string) (string, string) {
	var title, slug string

	// 1. 尝试从 Frontmatter 提取 (YAML)
	if strings.HasPrefix(content, "---") {
		lines := strings.Split(content, "\n")
		// 查找第二个 ---
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" {
				break
			}
			if strings.HasPrefix(line, "title:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "title:"))
				title = strings.Trim(val, "\"'")
			}
			if strings.HasPrefix(line, "slug:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "slug:"))
				slug = strings.Trim(val, "\"'")
			}
		}
	}

	// 2. 如果标题为空，回退到查找第一个 H1 (Markdown) or = (Adoc)
	if title == "" {
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimPrefix(line, "# ")
				break
			}
			if strings.HasPrefix(line, "= ") {
				title = strings.TrimPrefix(line, "= ")
				break
			}
		}
	}

	return title, slug
}

// replaceRelRef replace Hugo relref shortcode with local link
func replaceRelRef(content string) string {
	// Match both `ref` and `relref` with or without quotes
	// {{< relref "path" >}} or {{< ref "path" >}}
	re := regexp.MustCompile(`\{\{<\s*(?:relref|ref)\s+["']?([^"'\s}]+)["']?\s*>\}\}`)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		submatch := re.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		refPath := submatch[1]

		// Separate path and anchor
		var anchor string
		if idx := strings.LastIndex(refPath, "#"); idx != -1 {
			anchor = refPath[idx:]
			refPath = refPath[:idx]
		}

		// Helper to find article by path
		findArticle := func(path string) *Article {
			// Normalize path separators
			path = filepath.ToSlash(path)

			for i := range articles {
				art := &articles[i]
				// 1. Check strict RelPath match
				// Note: art.RelPath uses OS separators, convert to slash for comparison if needed
				artRelPath := filepath.ToSlash(art.RelPath)
				if artRelPath == path {
					return art
				}

				// 2. Check if path has /posts prefix or similar (common in hugo relref)
				// path: /posts/full/path.md -> artRelPath: full/path.md
				if strings.HasSuffix(path, artRelPath) {
					// Ensure simple suffix match is safe enough
					// e.g. path="posts/a/b.md", art="a/b.md" -> match
					// Check boundary to avoid "ba/b.md" matching "a/b.md"
					marker := "/" + artRelPath
					if strings.HasSuffix(path, marker) {
						return art
					}
				}
			}
			return nil
		}

		// Try to find article
		art := findArticle(refPath)

		// Fallback: try replacing extension (e.g. .adoc -> .md or .md -> .adoc)
		if art == nil {
			ext := filepath.Ext(refPath)
			if ext != "" {
				base := strings.TrimSuffix(refPath, ext)
				// Try .md if original was not .md, or .adoc if not .adoc
				candidates := []string{base + ".md", base + ".adoc"}
				for _, c := range candidates {
					if c == refPath {
						continue
					}
					if art = findArticle(c); art != nil {
						break
					}
				}
			}
		}

		if art != nil {
			// 如果配置了 BaseURL，生成完整的 URL
			if config.AppConfig.BaseURL != "" {
				// 格式: BaseURL/posts/Series/Slug#Anchor
				// 注意：这里假设 URL 结构是 /posts/:series/:slug
				baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
				targetSlug := art.Slug
				if targetSlug == "" {
					targetSlug = art.ID // Fallback ID if no slug
				}
				return fmt.Sprintf("%s/posts/%s/%s/%s", baseURL, art.Series, targetSlug, anchor)
			}

			// 默认本地预览链接
			return fmt.Sprintf("/article/%s%s", art.ID, anchor)
		}

		// If not found, keep original or show broken link?
		// Returning the original tag keeps it raw in markdown.
		// Returning a dead link might be better for preview.
		return fmt.Sprintf("#relref-not-found-%s", refPath)
	})
}

// removeFrontmatter 移除 Frontmatter
func removeFrontmatter(content string) string {
	content = strings.TrimPrefix(content, "\ufeff") // 处理 BOM
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return content
	}

	// 统一处理换行符，简化分割逻辑
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	endIndex := -1
	// 找到第一个非空的 --- 开始
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

// removeTitle 移除内容中的第一个 H1 标题
func removeTitle(content string) string {
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

// handleList 文章列表页面
func handleList(c *gin.Context) {
	// 按系列分组
	grouped := make(map[string][]Article)
	for _, article := range articles {
		grouped[article.Series] = append(grouped[article.Series], article)
	}

	// 对每个系列内的文章按更新时间倒序排序
	for series, articleList := range grouped {
		sort.Slice(articleList, func(i, j int) bool {
			return articleList[i].CreatedAt.After(articleList[j].CreatedAt)
		})
		grouped[series] = articleList
	}

	c.HTML(200, "list.html", gin.H{
		"groupedArticles": grouped,
	})
}

// handleArticle 文章详情页面
func handleArticle(c *gin.Context) {
	fmt.Println(">>> Entering handleArticle")
	id := c.Param("id")

	// 查找文章
	var article *Article
	for i := range articles {
		if articles[i].ID == id {
			article = &articles[i]
			break
		}
	}

	if article == nil {
		c.String(404, "文章不存在")
		return
	}

	// 读取并渲染 Markdown
	content, err := os.ReadFile(article.Path)
	if err != nil {
		c.String(500, "读取文章失败")
		return
	}

	// 1. 移除 Frontmatter
	contentStr := removeFrontmatter(string(content))

	// 2. 移除标题 (H1)
	markdownContent := removeTitle(contentStr)

	// 3. 处理 Hugo relref
	markdownContent = replaceRelRef(markdownContent)

	var buf strings.Builder
	if err := md.Convert([]byte(markdownContent), &buf); err != nil {
		c.String(500, "渲染文章失败")
		return
	}

	// 1. 列表项优化 (strong -> span, li wrap)
	htmlContent := buf.String()

	// 先替换 strong 为 span class="li-bold"
	reStrong := regexp.MustCompile(`<li><strong>([^<]+)</strong>`)
	htmlContent = reStrong.ReplaceAllString(htmlContent, `<li><span class="li-bold">$1</span>`)

	// 将 li 内部的所有内容包裹在 <span class="li-text"> 中
	reLiContent := regexp.MustCompile(`<li>(.*?)</li>`)
	htmlContent = reLiContent.ReplaceAllString(htmlContent, `<li><span class="li-text">$1</span></li>`)

	// 4. 本地图片路径修正 (动态解析)
	// Hugo 约定：static 目录下的文件会被映射到网站根目录
	// 文章中的相对路径可能指向 static 目录
	articleDir := filepath.Dir(article.Path)

	// 调试日志：打印文章目录
	fmt.Printf("Debug: Article Dir for %s is %s\n", article.ID, articleDir)
	fmt.Printf("Debug: Project Root is %s\n", projectRoot)

	// 正则匹配 img src，排除 http 开头的
	reImg := regexp.MustCompile(`src=["']([^"']+)["']`)

	htmlContent = reImg.ReplaceAllStringFunc(htmlContent, func(match string) string {
		// match: src="./images/foo.png"
		parts := strings.SplitN(match, "=", 2)
		if len(parts) != 2 {
			return match
		}
		quote := parts[1][0:1]
		src := parts[1][1 : len(parts[1])-1]

		// 忽略绝对路径（HTTP）
		if strings.HasPrefix(src, "http") || strings.HasPrefix(src, "//") {
			return match
		}

		// 尝试解析绝对路径
		var absImgPath string
		if filepath.IsAbs(src) {
			absImgPath = src
		} else {
			// 相对路径：基于文章所在目录解析
			absImgPath = filepath.Join(articleDir, src)
		}
		absImgPath = filepath.Clean(absImgPath)

		fmt.Printf("Debug: Original src: %s\n", src)
		fmt.Printf("Debug: Resolved absolute path: %s\n", absImgPath)

		// 特殊处理：如果路径包含 /content/static/，替换为 /static/
		// 这是因为 Hugo 的 static 目录在项目根目录，而不是 content 目录下
		if strings.Contains(absImgPath, "/content/static/") {
			absImgPath = strings.Replace(absImgPath, "/content/static/", "/static/", 1)
			fmt.Printf("Debug: Hugo static path detected, corrected to: %s\n", absImgPath)
		}

		// 检查是否在 ProjectRoot 内
		if strings.HasPrefix(absImgPath, projectRoot) {
			relPath, err := filepath.Rel(projectRoot, absImgPath)
			if err == nil {
				// 构造新 URL: /_local_fs/relPath
				// 注意 windows 下 filepath.Rel 返回反斜杠，需要转换
				relPath = filepath.ToSlash(relPath)
				newSrc := fmt.Sprintf("/_local_fs/%s", relPath)
				fmt.Printf("Debug: Rewriting %s -> %s\n", src, newSrc)
				return fmt.Sprintf("src=%s%s%s", quote, newSrc, quote)
			} else {
				fmt.Printf("Debug: Failed to get relative path: %v\n", err)
			}
		} else {
			fmt.Printf("Debug: Image path %s is not under project root %s\n", absImgPath, projectRoot)
		}

		fmt.Printf("Debug: Failed to rewrite image path: %s (Root: %s)\n", src, projectRoot)
		return match
	})

	c.HTML(200, "article.html", gin.H{
		"title":  article.Title,
		"html":   template.HTML(htmlContent),
		"id":     article.ID,
		"series": article.Series,
	})
}

// handlePublish 处理发布请求
func handlePublish(c *gin.Context) {
	id := c.Param("id")
	var article *Article
	for i := range articles {
		if articles[i].ID == id {
			article = &articles[i]
			break
		}
	}
	if article == nil {
		c.JSON(404, gin.H{"error": "文章不存在"})
		return
	}

	// 调用发布服务
	// projectRoot 需要绝对路径? or relative is fine
	// 我们用 ..
	result, err := services.PublishArticle(article.Path, projectRoot)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// 移除 Frontmatter (发布内容也不应包含)
	result.PublishContent = removeFrontmatter(result.PublishContent)

	// 渲染 Markdown 为 HTML 供复制
	var buf strings.Builder
	// 移除标题
	publishContent := removeTitle(result.PublishContent)
	md.Convert([]byte(publishContent), &buf)

	// 同样应用列表项优化
	htmlContent := buf.String()
	reStrong := regexp.MustCompile(`<li><strong>([^<]+)</strong>`)
	htmlContent = reStrong.ReplaceAllString(htmlContent, `<li><span class="li-bold">$1</span>`)
	reLiContent := regexp.MustCompile(`<li>(.*?)</li>`)
	htmlContent = reLiContent.ReplaceAllString(htmlContent, `<li><span class="li-text">$1</span></li>`)

	c.JSON(200, gin.H{
		"success": true,
		"content": map[string]string{
			"markdown": result.PublishContent,
			"html":     htmlContent, // 返回已处理的 HTML
		},
		"uploaded": result.UploadedImages,
		"logs":     result.Errors,
	})
}

// apiArticles API: 文章列表
func apiArticles(c *gin.Context) {
	c.JSON(200, articles)
}

// apiArticleDetail API: 文章详情
func apiArticleDetail(c *gin.Context) {
	id := c.Param("id")

	// 查找文章
	var article *Article
	for i := range articles {
		if articles[i].ID == id {
			article = &articles[i]
			break
		}
	}

	if article == nil {
		c.JSON(404, gin.H{"error": "文章不存在"})
		return
	}

	// 读取内容
	content, err := os.ReadFile(article.Path)
	if err != nil {
		c.JSON(500, gin.H{"error": "读取文章失败"})
		return
	}

	// 清洗内容：移除 Frontmatter 和 H1 标题
	contentStr := removeFrontmatter(string(content))
	contentStr = removeTitle(contentStr)

	// 处理 relref (仅用于渲染HTML，RawMarkdown保持原样或也替换？保持原样更便于编辑)
	htmlMarkdown := replaceRelRef(contentStr)

	// 渲染 HTML
	var buf strings.Builder
	if err := md.Convert([]byte(htmlMarkdown), &buf); err != nil { // 使用处理后的 markdown
		c.JSON(500, gin.H{"error": "渲染文章失败"})
		return
	}

	c.JSON(200, ArticleDetail{
		Article:     *article,
		HTML:        buf.String(),
		RawMarkdown: contentStr,
	})
}

// handleChromaCSS 提供 Chroma 语法高亮的 CSS
func handleChromaCSS(c *gin.Context) {
	// 生成 Monokai 主题的 CSS
	css := `
/* Chroma Monokai 主题 */
.chroma { background-color: #272822; color: #f8f8f2; white-space: pre !important; }
.chroma .err { color: #960050; background-color: #1e0010 }
.chroma .lntd { vertical-align: top; padding: 0; margin: 0; border: 0; }
.chroma .lntable { border-spacing: 0; padding: 0; margin: 0; border: 0; }
.chroma .hl { display: block; width: 100%; background-color: #3c3d37 }
.chroma .lnt { white-space: pre; user-select: none; margin-right: 0.4em; padding: 0 0.4em 0 0.4em; color: #7f7f7f }
.chroma .ln { white-space: pre; user-select: none; margin-right: 0.4em; padding: 0 0.4em 0 0.4em; color: #7f7f7f }
.chroma .line { display: inline; white-space: pre !important; }
.chroma .k { color: #66d9ef }
.chroma .kc { color: #66d9ef }
.chroma .kd { color: #66d9ef }
.chroma .kn { color: #f92672 }
.chroma .kp { color: #66d9ef }
.chroma .kr { color: #66d9ef }
.chroma .kt { color: #66d9ef }
.chroma .na { color: #a6e22e }
.chroma .nb { color: #f8f8f2 }
.chroma .nc { color: #a6e22e }
.chroma .no { color: #66d9ef }
.chroma .nd { color: #a6e22e }
.chroma .ni { color: #f8f8f2 }
.chroma .ne { color: #a6e22e }
.chroma .nf { color: #a6e22e }
.chroma .nl { color: #f8f8f2 }
.chroma .nn { color: #f8f8f2 }
.chroma .nx { color: #a6e22e }
.chroma .py { color: #f8f8f2 }
.chroma .nt { color: #f92672 }
.chroma .nv { color: #f8f8f2 }
.chroma .s { color: #e6db74 }
.chroma .sa { color: #e6db74 }
.chroma .sb { color: #e6db74 }
.chroma .sc { color: #e6db74 }
.chroma .dl { color: #e6db74 }
.chroma .sd { color: #e6db74 }
.chroma .s2 { color: #e6db74 }
.chroma .se { color: #ae81ff }
.chroma .sh { color: #e6db74 }
.chroma .si { color: #e6db74 }
.chroma .sx { color: #e6db74 }
.chroma .sr { color: #e6db74 }
.chroma .s1 { color: #e6db74 }
.chroma .ss { color: #e6db74 }
.chroma .m { color: #ae81ff }
.chroma .mb { color: #ae81ff }
.chroma .mf { color: #ae81ff }
.chroma .mh { color: #ae81ff }
.chroma .mi { color: #ae81ff }
.chroma .il { color: #ae81ff }
.chroma .mo { color: #ae81ff }
.chroma .o { color: #f92672 }
.chroma .ow { color: #f92672 }
.chroma .p { color: #f8f8f2 }
.chroma .c { color: #75715e }
.chroma .ch { color: #75715e }
.chroma .cm { color: #75715e }
.chroma .c1 { color: #75715e }
.chroma .cs { color: #75715e }
.chroma .cp { color: #75715e }
.chroma .cpf { color: #75715e }
.chroma .gd { color: #f92672 }
.chroma .ge { font-style: italic }
.chroma .gi { color: #a6e22e }
.chroma .gs { font-weight: bold }
.chroma .gu { color: #75715e }
`
	c.Header("Content-Type", "text/css")
	c.String(200, css)
}
