package handlers

import (
	"github.com/gin-gonic/gin"
)

// HandleGetPlatforms 获取所有平台
func (h *Handler) HandleGetPlatforms(c *gin.Context) {
	platforms := h.PlatformService.GetAll()
	c.JSON(200, gin.H{
		"platforms": platforms,
	})
}

// HandleGetAllStatus 获取所有文章的状态
func (h *Handler) HandleGetAllStatus(c *gin.Context) {
	allStatus := h.StatusService.GetAllStatus()
	c.JSON(200, gin.H{
		"articles": allStatus,
	})
}

// HandleGetStatus 获取文章状态
func (h *Handler) HandleGetStatus(c *gin.Context) {
	articleID := c.Param("articleID")
	status := h.StatusService.GetArticleStatus(articleID)
	c.JSON(200, status)
}

// MarkPublishRequest 标记发布请求
type MarkPublishRequest struct {
	URL string `json:"url"`
}

// HandleMarkPublished 标记为已发布
func (h *Handler) HandleMarkPublished(c *gin.Context) {
	articleID := c.Param("articleID")
	platformID := c.Param("platformID")

	if h.PlatformService.GetByID(platformID) == nil {
		c.JSON(400, gin.H{"error": "平台不存在"})
		return
	}

	var req MarkPublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.URL = ""
	}

	if err := h.StatusService.MarkPublished(articleID, platformID, req.URL); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"status":  h.StatusService.GetArticleStatus(articleID),
	})
}

// HandleUnmarkPublished 取消发布标记
func (h *Handler) HandleUnmarkPublished(c *gin.Context) {
	articleID := c.Param("articleID")
	platformID := c.Param("platformID")

	if err := h.StatusService.UnmarkPublished(articleID, platformID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"status":  h.StatusService.GetArticleStatus(articleID),
	})
}
