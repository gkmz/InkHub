package handlers

import (
	"github.com/hankmor/mymedia/tools/wechat-preview/markdown"
	"github.com/hankmor/mymedia/tools/wechat-preview/models"
	"github.com/hankmor/mymedia/tools/wechat-preview/services"
)

// Handler 处理器上下文
type Handler struct {
	Articles        []models.Article
	Processor       *markdown.Processor
	PlatformService *services.PlatformService
	StatusService   *services.StatusService
	ProjectRoot     string
}

// NewHandler 创建处理器
func NewHandler(
	articles []models.Article,
	processor *markdown.Processor,
	platformService *services.PlatformService,
	statusService *services.StatusService,
	projectRoot string,
) *Handler {
	return &Handler{
		Articles:        articles,
		Processor:       processor,
		PlatformService: platformService,
		StatusService:   statusService,
		ProjectRoot:     projectRoot,
	}
}
