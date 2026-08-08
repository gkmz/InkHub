package httptransport

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gkmz/InkHub/internal/app/publication"
)

// PublicationWorkflowAPI 定义文章级恢复和统一历史查询能力。
type PublicationWorkflowAPI interface {
	Find(ctx context.Context, articleID string) (publication.WorkflowView, error)
	History(ctx context.Context, articleID, cursor string, limit int) (publication.HistoryPage, error)
}

func (h *runtimeHandler) publicationWorkflow(response http.ResponseWriter, request *http.Request) {
	if h.publicationWorkflows == nil {
		writeError(response, http.StatusServiceUnavailable, "publication.workflow_unavailable", "发布状态恢复暂不可用")
		return
	}
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/publication-workflow")
	view, err := h.publicationWorkflows.Find(request.Context(), articleID)
	if err != nil {
		writePublicationWorkflowError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, safeWorkflowView(view))
}

func (h *runtimeHandler) publicationHistory(response http.ResponseWriter, request *http.Request) {
	if h.publicationWorkflows == nil {
		writeError(response, http.StatusServiceUnavailable, "publication.workflow_unavailable", "发布历史暂不可用")
		return
	}
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/publication-history")
	limit := 20
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 50 {
			writeError(response, http.StatusBadRequest, "request.invalid", "发布历史分页大小无效")
			return
		}
		limit = parsed
	}
	page, err := h.publicationWorkflows.History(request.Context(), articleID, request.URL.Query().Get("cursor"), limit)
	if err != nil {
		writePublicationWorkflowError(response, err)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, map[string]any{"id": item.ID, "channel": item.Channel, "state": item.State, "title": item.Title, "detail": item.Detail, "occurred_at": item.OccurredAt})
	}
	writeJSON(response, http.StatusOK, map[string]any{"items": items, "next_cursor": page.NextCursor})
}

func safeWorkflowView(view publication.WorkflowView) map[string]any {
	result := map[string]any{"article_id": view.ArticleID, "hugo": nil}
	if view.Hugo == nil {
		return result
	}
	hugo := map[string]any{"state": view.Hugo.State, "progress": view.Hugo.Progress, "stage": view.Hugo.Stage, "error": view.Hugo.Error}
	if view.Hugo.Failure != nil {
		hugo["failure"] = safePublicationFailure(view.Hugo.Failure)
	}
	if view.Hugo.Delivery != nil {
		delivery := map[string]any{"state": view.Hugo.Delivery.State, "progress": view.Hugo.Delivery.Progress, "stage": view.Hugo.Delivery.Stage, "error": view.Hugo.Delivery.Error}
		if view.Hugo.Delivery.Failure != nil {
			delivery["failure"] = safePublicationFailure(view.Hugo.Delivery.Failure)
		}
		hugo["delivery"] = delivery
	}
	if view.Hugo.Preview != nil {
		preview := view.Hugo.Preview
		files := make([]map[string]any, 0, len(preview.Files))
		for _, file := range preview.Files {
			files = append(files, map[string]any{"relative_path": file.RelativePath, "media_type": file.MediaType, "size": file.Size})
		}
		diagnostics := make([]map[string]string, 0, len(preview.Diagnostics))
		for _, diagnostic := range preview.Diagnostics {
			diagnostics = append(diagnostics, map[string]string{"code": diagnostic.Code, "level": diagnostic.Level, "message": diagnostic.Message})
		}
		previewView := map[string]any{"preview_id": preview.ID, "section": preview.Section, "target_path": preview.TargetPath, "change": preview.Change, "files": files, "diagnostics": diagnostics, "preview_url": preview.PreviewURL, "render_url": hugoPreviewRenderURL(preview.ID, preview.RenderPath), "expires_at": preview.ExpiresAt, "state": preview.State, "error": preview.Error}
		if preview.Failure != nil {
			previewView["failure"] = safePublicationFailure(preview.Failure)
		}
		hugo["preview"] = previewView
	}
	result["hugo"] = hugo
	return result
}

func writePublicationWorkflowError(response http.ResponseWriter, err error) {
	if errors.Is(err, publication.ErrHistoryCursorInvalid) {
		logHTTPError(response, err, http.StatusBadRequest, "request.cursor_invalid")
		writeError(response, http.StatusBadRequest, "request.cursor_invalid", "发布历史分页位置无效")
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		mapError(response, ErrNotFound)
		return
	}
	logHTTPError(response, err, http.StatusUnprocessableEntity, "publication.workflow_failed")
	writeError(response, http.StatusUnprocessableEntity, "publication.workflow_failed", "发布状态读取失败")
}
