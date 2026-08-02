package httptransport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/domain/xiaohongshu"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// XiaohongshuDraftView 是返回给前端的完整小红书草稿版本。
type XiaohongshuDraftView struct {
	ID                string   `json:"id"`
	ArticleID         string   `json:"article_id"`
	SourceContentHash string   `json:"source_content_hash"`
	Title             string   `json:"title"`
	BodyHTML          string   `json:"body_html"`
	Topics            []string `json:"topics"`
	SourceNote        string   `json:"source_note"`
	CommentCopy       string   `json:"comment_copy"`
	AIModel           string   `json:"ai_model"`
	PromptVersion     string   `json:"prompt_version"`
	State             string   `json:"state"`
	Stale             bool     `json:"stale"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

// XiaohongshuView 汇总当前文章的小红书草稿和发布状态。
type XiaohongshuView struct {
	ArticleID          string                 `json:"article_id"`
	CurrentContentHash string                 `json:"current_content_hash"`
	State              string                 `json:"state"`
	Latest             *XiaohongshuDraftView  `json:"latest"`
	History            []XiaohongshuDraftView `json:"history"`
}

type xiaohongshuDraftInput struct {
	DraftID     string   `json:"draft_id"`
	Title       string   `json:"title"`
	BodyHTML    string   `json:"body_html"`
	Topics      []string `json:"topics"`
	SourceNote  string   `json:"source_note"`
	CommentCopy string   `json:"comment_copy"`
}

type xiaohongshuRenderInput struct {
	DraftID         string `json:"draft_id"`
	TemplateID      string `json:"template_id"`
	TemplateVersion string `json:"template_version"`
	ViewportWidth   int    `json:"viewport_width"`
	PageHeight      int    `json:"page_height"`
	HTMLHash        string `json:"html_hash"`
	PageCount       int    `json:"page_count"`
}

// xiaohongshu handles article-scoped draft, render and manual publication commands.
func (h *runtimeHandler) xiaohongshu(response http.ResponseWriter, request *http.Request) {
	articleID, suffix, ok := parseXiaohongshuPath(request.URL.Path)
	if !ok {
		writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		return
	}
	switch {
	case request.Method == http.MethodGet && suffix == "":
		h.xiaohongshuView(response, request, articleID)
	case request.Method == http.MethodGet && suffix == "history":
		h.xiaohongshuView(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "drafts/generate":
		h.xiaohongshuGenerate(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "drafts":
		h.xiaohongshuSave(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "renders":
		h.xiaohongshuRender(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "published":
		h.xiaohongshuPublished(response, request, articleID)
	default:
		writeError(response, http.StatusNotFound, "route.not_found", "接口不存在")
	}
}

func parseXiaohongshuPath(path string) (string, string, bool) {
	const prefix = "/api/v1/articles/"
	if !strings.HasPrefix(path, prefix) || !strings.Contains(path, "/xiaohongshu") {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(rest, "/xiaohongshu", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	suffix := strings.TrimPrefix(parts[1], "/")
	return parts[0], suffix, true
}

func (h *runtimeHandler) xiaohongshuView(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, _, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	drafts, err := repository.NewXiaohongshuRepository(h.db).ListDrafts(request.Context(), workspaceID, articleID, 50)
	if err != nil {
		mapError(response, err)
		return
	}
	history := make([]XiaohongshuDraftView, 0, len(drafts))
	for _, draft := range drafts {
		history = append(history, xiaohongshuDraftView(draft, contentHash))
	}
	var latest *XiaohongshuDraftView
	if len(history) > 0 {
		latest = &history[0]
	}
	state := "尚未准备"
	if latest != nil {
		state = xiaohongshuState(*latest)
	}
	writeJSON(response, http.StatusOK, XiaohongshuView{ArticleID: articleID, CurrentContentHash: contentHash, State: state, Latest: latest, History: history})
}

func (h *runtimeHandler) xiaohongshuGenerate(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, title, rendered, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	var tagsJSON string
	if err := h.db.QueryRowContext(request.Context(), `SELECT tags_json FROM articles WHERE id=?`, articleID).Scan(&tagsJSON); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	var topics []string
	_ = json.Unmarshal([]byte(tagsJSON), &topics)
	if topics == nil {
		topics = []string{}
	}
	if len(topics) > 8 {
		topics = topics[:8]
	}
	if len(topics) == 0 {
		topics = []string{"AI应用", "效率工具"}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	draft := xiaohongshu.Draft{ID: stableXiaohongshuID("draft", articleID, contentHash, now), ArticleID: articleID, WorkspaceID: workspaceID, SourceContentHash: contentHash, Title: title, BodyHTML: rendered, Topics: topics, SourceNote: "内容由 InkHub 根据原文适配，可在发布前修改", CommentCopy: "你怎么看？欢迎在评论区交流。", AIModel: "inkhub-adapter-v1", PromptVersion: "xiaohongshu-v1", State: xiaohongshu.DraftStateDraft}
	draft.CreatedAt, draft.UpdatedAt = now, now
	repo := repository.NewXiaohongshuRepository(h.db)
	if err := repo.SaveDraft(request.Context(), draft); err != nil {
		mapError(response, err)
		return
	}
	payload, _ := json.Marshal(map[string]string{"source": "generate"})
	_ = repo.SaveEvent(request.Context(), xiaohongshu.Event{ID: stableXiaohongshuID("event", draft.ID, "generated"), DraftID: draft.ID, EventType: "generated", Payload: string(payload)})
	writeJSON(response, http.StatusCreated, xiaohongshuDraftView(draft, contentHash))
}

func (h *runtimeHandler) xiaohongshuSave(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, _, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	var input xiaohongshuDraftInput
	if decodeJSON(request, &input) != nil || input.DraftID == "" || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.BodyHTML) == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书草稿请求无效")
		return
	}
	repo := repository.NewXiaohongshuRepository(h.db)
	draft, err := repo.FindDraft(request.Context(), workspaceID, articleID, input.DraftID)
	if errors.Is(err, sql.ErrNoRows) {
		mapError(response, ErrNotFound)
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if draft.SourceContentHash != contentHash {
		writeError(response, http.StatusConflict, "content.stale", "文章内容已变化，请基于最新版本重新生成")
		return
	}
	draft.Title, draft.BodyHTML, draft.Topics, draft.SourceNote, draft.CommentCopy = strings.TrimSpace(input.Title), input.BodyHTML, input.Topics, input.SourceNote, input.CommentCopy
	if draft.Topics == nil {
		draft.Topics = []string{}
	}
	if err := repo.SaveDraft(request.Context(), draft); err != nil {
		mapError(response, err)
		return
	}
	_ = repo.SaveEvent(request.Context(), xiaohongshu.Event{ID: stableXiaohongshuID("event", draft.ID, "saved", time.Now().UTC().Format(time.RFC3339Nano)), DraftID: draft.ID, EventType: "saved"})
	writeJSON(response, http.StatusOK, xiaohongshuDraftView(draft, contentHash))
}

func (h *runtimeHandler) xiaohongshuRender(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, _, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	var input xiaohongshuRenderInput
	if decodeJSON(request, &input) != nil || input.DraftID == "" || input.TemplateID == "" || input.PageCount < 1 || input.ViewportWidth < 280 {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书渲染请求无效")
		return
	}
	repo := repository.NewXiaohongshuRepository(h.db)
	draft, err := repo.FindDraft(request.Context(), workspaceID, articleID, input.DraftID)
	if errors.Is(err, sql.ErrNoRows) {
		mapError(response, ErrNotFound)
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if draft.SourceContentHash != contentHash {
		writeError(response, http.StatusConflict, "content.stale", "草稿对应的文章版本已过期")
		return
	}
	if input.TemplateVersion == "" {
		input.TemplateVersion = "1"
	}
	render := xiaohongshu.Render{ID: stableXiaohongshuID("render", draft.ID, input.TemplateID, input.HTMLHash), DraftID: draft.ID, ArticleID: articleID, TemplateID: input.TemplateID, TemplateVersion: input.TemplateVersion, ViewportWidth: input.ViewportWidth, PageHeight: input.PageHeight, HTMLHash: input.HTMLHash, PageCount: input.PageCount, State: "ready"}
	if err := repo.SaveRender(request.Context(), render); err != nil {
		mapError(response, err)
		return
	}
	_ = repo.SaveEvent(request.Context(), xiaohongshu.Event{ID: stableXiaohongshuID("event", render.ID, "rendered"), DraftID: draft.ID, RenderID: render.ID, EventType: "rendered"})
	writeJSON(response, http.StatusCreated, map[string]any{"id": render.ID, "draft_id": draft.ID, "state": render.State, "page_count": render.PageCount})
}

func (h *runtimeHandler) xiaohongshuPublished(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, _, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	var input struct {
		DraftID string `json:"draft_id"`
	}
	if decodeJSON(request, &input) != nil || input.DraftID == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书发布确认请求无效")
		return
	}
	repo := repository.NewXiaohongshuRepository(h.db)
	draft, err := repo.FindDraft(request.Context(), workspaceID, articleID, input.DraftID)
	if errors.Is(err, sql.ErrNoRows) {
		mapError(response, ErrNotFound)
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if draft.SourceContentHash != contentHash {
		writeError(response, http.StatusConflict, "content.stale", "草稿对应的文章版本已过期")
		return
	}
	draft.State = xiaohongshu.DraftStatePublished
	if err := repo.SaveDraft(request.Context(), draft); err != nil {
		mapError(response, err)
		return
	}
	_ = repo.SaveEvent(request.Context(), xiaohongshu.Event{ID: stableXiaohongshuID("event", draft.ID, "published"), DraftID: draft.ID, EventType: "published"})
	writeJSON(response, http.StatusOK, map[string]string{"state": "已发布"})
}

// xiaohongshuArticle 读取当前文章版本并复用现有安全 Markdown 渲染器。
func (h *runtimeHandler) xiaohongshuArticle(ctx context.Context, articleID string) (workspaceID, contentHash, title, rendered string, err error) {
	var sourceID, stableID, relative string
	if err = h.db.QueryRowContext(ctx, `SELECT workspace_id,source_id,stable_id,relative_path,title,content_hash FROM articles WHERE id=? AND deleted_at IS NULL`, articleID).Scan(&workspaceID, &sourceID, &stableID, &relative, &title, &contentHash); err != nil {
		return "", "", "", "", ErrNotFound
	}
	source, sourceErr := h.buildSource(ctx, sourceID, nil)
	if sourceErr != nil {
		return "", "", "", "", sourceErr
	}
	document, readErr := source.Read(ctx, contracts.SourceRef{SourceID: sourceID, RelativePath: relative, StableID: stableID})
	if readErr != nil {
		return "", "", "", "", readErr
	}
	rendered, err = h.renderArticlePreview(ctx, source, document, articleID)
	return
}

func xiaohongshuDraftView(draft xiaohongshu.Draft, currentHash string) XiaohongshuDraftView {
	return XiaohongshuDraftView{ID: draft.ID, ArticleID: draft.ArticleID, SourceContentHash: draft.SourceContentHash, Title: draft.Title, BodyHTML: draft.BodyHTML, Topics: draft.Topics, SourceNote: draft.SourceNote, CommentCopy: draft.CommentCopy, AIModel: draft.AIModel, PromptVersion: draft.PromptVersion, State: string(draft.State), Stale: draft.SourceContentHash != currentHash, CreatedAt: draft.CreatedAt, UpdatedAt: draft.UpdatedAt}
}

func xiaohongshuState(draft XiaohongshuDraftView) string {
	if draft.Stale {
		return "内容已更新"
	}
	if draft.State == string(xiaohongshu.DraftStatePublished) {
		return "已发布"
	}
	return "草稿"
}

// runtimeXiaohongshuLabel 将数据库状态映射为文章详情中的渠道标签。
func runtimeXiaohongshuLabel(state, processedHash, currentHash string) string {
	if state == "published" && processedHash == currentHash {
		return "已发布"
	}
	if state != "" && state != "never" && processedHash != currentHash {
		return "内容已更新"
	}
	if state == "draft" {
		return "草稿"
	}
	return "尚未准备"
}

func stableXiaohongshuID(kind string, values ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return fmt.Sprintf("xhs_%s_%s", kind, hex.EncodeToString(sum[:12]))
}
