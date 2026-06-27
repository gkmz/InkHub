package handlers

import (
	"html/template"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/gkmz/mymedia/tools/wechat-preview/models"
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
	mermaidTheme := c.DefaultQuery("mermaidTheme", "handdrawn")

	article, ok := h.findArticleByID(id)
	if !ok {
		c.String(404, "文章不存在")
		return
	}

	rendered, err := h.renderArticle(article, mermaidTheme)
	if err != nil {
		c.String(500, "渲染文章失败")
		return
	}

	c.HTML(200, "article.html", gin.H{
		"title":        article.Title,
		"html":         template.HTML(rendered.HTML),
		"id":           article.ID,
		"series":       article.Series,
		"mermaidTheme": mermaidTheme,
	})
}

// APIArticles API: 文章列表
func (h *Handler) APIArticles(c *gin.Context) {
	c.JSON(200, h.Articles)
}

// APIArticleDetail API: 文章详情
func (h *Handler) APIArticleDetail(c *gin.Context) {
	id := c.Param("id")
	mermaidTheme := c.DefaultQuery("mermaidTheme", "handdrawn")

	article, ok := h.findArticleByID(id)
	if !ok {
		c.JSON(404, gin.H{"error": "文章不存在"})
		return
	}

	rendered, err := h.renderArticle(article, mermaidTheme)
	if err != nil {
		c.JSON(500, gin.H{"error": "渲染文章失败"})
		return
	}

	c.JSON(200, models.ArticleDetail{
		Article:     *rendered.Article,
		HTML:        rendered.HTML,
		RawMarkdown: rendered.RawMarkdown,
	})
}
