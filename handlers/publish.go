package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/gkmz/mymedia/tools/wechat-preview/services"
)

// HandlePublish 处理发布请求
func (h *Handler) HandlePublish(c *gin.Context) {
	id := c.Param("id")
	article, ok := h.findArticleByID(id)
	if !ok {
		c.JSON(404, gin.H{"error": "文章不存在"})
		return
	}

	result, err := services.PublishArticle(article.Path, h.ProjectRoot)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	publishMarkdown, htmlContent, err := h.renderPublishHTML(result.PublishContent)
	if err != nil {
		c.JSON(500, gin.H{"error": "渲染失败"})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"content": map[string]string{
			"markdown": publishMarkdown,
			"html":     htmlContent,
		},
		"uploaded": result.UploadedImages,
		"logs":     result.Errors,
	})
}
