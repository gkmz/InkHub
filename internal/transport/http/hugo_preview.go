package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gkmz/InkHub/internal/app/publication"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// HugoPreviewAPI 定义 HTTP 层需要的 Hugo 预览编排能力。
type HugoPreviewAPI interface {
	DiscoverSections(ctx context.Context, articleID string) (contracts.SectionDiscovery, error)
	Queue(ctx context.Context, request publication.PreviewRequest) (domainjob.Job, error)
	Find(ctx context.Context, previewID string) (publication.PreviewView, error)
	Confirm(ctx context.Context, request publication.ConfirmPreviewRequest) (domainjob.Job, error)
}

func (h *runtimeHandler) hugoSections(response http.ResponseWriter, request *http.Request) {
	if h.hugoPreviews == nil {
		writeError(response, http.StatusServiceUnavailable, "hugo.preview_unavailable", "Hugo 发布预览暂不可用")
		return
	}
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/hugo-sections")
	discovery, err := h.hugoPreviews.DiscoverSections(request.Context(), articleID)
	if err != nil {
		writeHugoPreviewError(response, err)
		return
	}
	sections := make([]map[string]any, 0, len(discovery.Sections))
	for _, section := range discovery.Sections {
		sections = append(sections, map[string]any{"name": section.Name, "article_count": section.ArticleCount})
	}
	writeJSON(response, http.StatusOK, map[string]any{"sections": sections, "existing_section": discovery.ExistingSection, "selection_locked": discovery.SelectionLocked})
}

func (h *runtimeHandler) createHugoPreview(response http.ResponseWriter, request *http.Request) {
	if h.hugoPreviews == nil {
		writeError(response, http.StatusServiceUnavailable, "hugo.preview_unavailable", "Hugo 发布预览暂不可用")
		return
	}
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/hugo-previews")
	var input struct {
		ContentHash string `json:"content_hash"`
		Section     string `json:"section"`
	}
	if decodeJSON(request, &input) != nil || articleID == "" || input.ContentHash == "" || input.Section == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "Hugo 预览请求不完整")
		return
	}
	job, err := h.hugoPreviews.Queue(request.Context(), publication.PreviewRequest{ArticleID: articleID, ContentHash: input.ContentHash, Section: input.Section})
	if err != nil {
		writeHugoPreviewError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]string{"id": job.ID, "job_id": job.ID, "state": string(job.State)})
}

func (h *runtimeHandler) hugoPreview(response http.ResponseWriter, request *http.Request) {
	if h.hugoPreviews == nil {
		writeError(response, http.StatusServiceUnavailable, "hugo.preview_unavailable", "Hugo 发布预览暂不可用")
		return
	}
	id := strings.TrimPrefix(request.URL.Path, "/api/v1/hugo-previews/")
	view, err := h.hugoPreviews.Find(request.Context(), id)
	if err != nil {
		writeHugoPreviewError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, safeHugoPreviewView(view))
}

func (h *runtimeHandler) confirmHugoPreview(response http.ResponseWriter, request *http.Request) {
	if h.hugoPreviews == nil {
		writeError(response, http.StatusServiceUnavailable, "hugo.preview_unavailable", "Hugo 发布预览暂不可用")
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/hugo-previews/"), "/confirm")
	job, err := h.hugoPreviews.Confirm(request.Context(), publication.ConfirmPreviewRequest{PreviewID: id})
	if err != nil {
		writeHugoPreviewError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]string{"job_id": job.ID, "state": string(job.State)})
}

func safeHugoPreviewView(view publication.PreviewView) map[string]any {
	files := make([]map[string]any, 0, len(view.Files))
	for _, file := range view.Files {
		files = append(files, map[string]any{"relative_path": file.RelativePath, "media_type": file.MediaType, "size": file.Size})
	}
	diagnostics := make([]map[string]string, 0, len(view.Diagnostics))
	for _, diagnostic := range view.Diagnostics {
		diagnostics = append(diagnostics, map[string]string{"code": diagnostic.Code, "level": diagnostic.Level, "message": diagnostic.Message})
	}
	result := map[string]any{"id": view.ID, "article_id": view.ArticleID, "content_hash": view.ContentHash, "section": view.Section, "target_path": view.TargetPath, "change": view.Change, "files": files, "diagnostics": diagnostics, "preview_url": view.PreviewURL, "expires_at": view.ExpiresAt, "state": view.State, "job_id": view.JobID, "error": view.Error}
	if view.Failure != nil {
		result["failure"] = safePublicationFailure(view.Failure)
	}
	return result
}

func safePublicationFailure(failure *publication.PublicationFailure) map[string]any {
	return map[string]any{"stage": failure.Stage, "code": failure.Code, "message": failure.Message, "action": failure.Action, "retryable": failure.Retryable}
}

func writeHugoPreviewError(response http.ResponseWriter, err error) {
	status, code, message := http.StatusUnprocessableEntity, "hugo.preview_failed", "Hugo 发布预览操作失败"
	switch {
	case errors.Is(err, publication.ErrPreviewStale):
		status, code, message = http.StatusConflict, "hugo.preview_stale", "文章内容已变化，请重新生成预览"
	case errors.Is(err, publication.ErrPreviewExpired):
		status, code, message = http.StatusConflict, "hugo.preview_expired", "Hugo 发布预览已过期，请重新生成"
	case errors.Is(err, publication.ErrPreviewNotReady):
		status, code, message = http.StatusConflict, "hugo.preview_not_ready", "Hugo 发布预览尚未准备完成"
	case errors.Is(err, publication.ErrPreviewInvalid):
		code, message = "hugo.preview_invalid", "Hugo 发布预览数据无效"
	case errors.Is(err, publication.ErrArticleNotReady):
		code, message = "article.not_ready", "文章尚未标记为已就绪"
	}
	logHTTPError(response, err, status, code)
	writeError(response, status, code, message)
}
