package handlers

import (
	"html/template"
	"os"
	"path/filepath"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/hankmor/mymedia/tools/wechat-preview/models"
)

// HandleList 文章列表页面
func (h *Handler) HandleList(c *gin.Context) {
	// 按系列分组
	grouped := make(map[string][]models.Article)
	for _, article := range h.Articles {
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

// HandleArticle 文章详情页面
func (h *Handler) HandleArticle(c *gin.Context) {
	id := c.Param("id")

	// 查找文章
	var article *models.Article
	for i := range h.Articles {
		if h.Articles[i].ID == id {
			article = &h.Articles[i]
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

	// 处理内容
	contentStr := h.Processor.RemoveFrontmatter(string(content))
	markdownContent := h.Processor.RemoveTitle(contentStr)
	markdownContent = h.Processor.ReplaceRelRef(markdownContent)

	htmlContent, err := h.Processor.Convert(markdownContent)
	if err != nil {
		c.String(500, "渲染文章失败")
		return
	}

	// 优化列表项
	htmlContent = h.Processor.OptimizeListItems(htmlContent)

	// 处理图片路径
	articleDir := filepath.Dir(article.Path)
	htmlContent = h.Processor.ProcessImagePaths(htmlContent, articleDir)

	c.HTML(200, "article.html", gin.H{
		"title":  article.Title,
		"html":   template.HTML(htmlContent),
		"id":     article.ID,
		"series": article.Series,
	})
}

// APIArticles API: 文章列表
func (h *Handler) APIArticles(c *gin.Context) {
	c.JSON(200, h.Articles)
}

// APIArticleDetail API: 文章详情
func (h *Handler) APIArticleDetail(c *gin.Context) {
	id := c.Param("id")

	var article *models.Article
	for i := range h.Articles {
		if h.Articles[i].ID == id {
			article = &h.Articles[i]
			break
		}
	}

	if article == nil {
		c.JSON(404, gin.H{"error": "文章不存在"})
		return
	}

	content, err := os.ReadFile(article.Path)
	if err != nil {
		c.JSON(500, gin.H{"error": "读取文章失败"})
		return
	}

	contentStr := h.Processor.RemoveFrontmatter(string(content))
	contentStr = h.Processor.RemoveTitle(contentStr)
	htmlMarkdown := h.Processor.ReplaceRelRef(contentStr)

	htmlContent, err := h.Processor.Convert(htmlMarkdown)
	if err != nil {
		c.JSON(500, gin.H{"error": "渲染文章失败"})
		return
	}

	c.JSON(200, models.ArticleDetail{
		Article:     *article,
		HTML:        htmlContent,
		RawMarkdown: contentStr,
	})
}
