package handlers

import (
	"github.com/gkmz/mymedia/tools/wechat-preview/markdown"
	"github.com/gkmz/mymedia/tools/wechat-preview/models"
	"github.com/gkmz/mymedia/tools/wechat-preview/services"
)

// Handler 处理器上下文
type Handler struct {
	Articles        []models.Article
	articleByID     map[string]*models.Article
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
	articleByID := make(map[string]*models.Article, len(articles))
	for i := range articles {
		articleByID[articles[i].ID] = &articles[i]
	}

	return &Handler{
		Articles:        articles,
		articleByID:     articleByID,
		Processor:       processor,
		PlatformService: platformService,
		StatusService:   statusService,
		ProjectRoot:     projectRoot,
	}
}
