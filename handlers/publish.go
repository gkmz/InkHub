package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/hankmor/mymedia/tools/wechat-preview/models"
	"github.com/hankmor/mymedia/tools/wechat-preview/services"
)

// HandlePublish 处理发布请求
func (h *Handler) HandlePublish(c *gin.Context) {
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

	result, err := services.PublishArticle(article.Path, h.ProjectRoot)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	result.PublishContent = h.Processor.RemoveFrontmatter(result.PublishContent)
	publishContent := h.Processor.RemoveTitle(result.PublishContent)

	htmlContent, err := h.Processor.Convert(publishContent)
	if err != nil {
		c.JSON(500, gin.H{"error": "渲染失败"})
		return
	}

	htmlContent = h.Processor.OptimizeListItems(htmlContent)

	c.JSON(200, gin.H{
		"success": true,
		"content": map[string]string{
			"markdown": result.PublishContent,
			"html":     htmlContent,
		},
		"uploaded": result.UploadedImages,
		"logs":     result.Errors,
	})
}
