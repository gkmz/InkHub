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
	"github.com/gkmz/InkHub/internal/editorial"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// XiaohongshuDraftView 是返回给前端的完整小红书草稿版本。
type XiaohongshuDraftView struct {
	ID                string                   `json:"id"`
	ArticleID         string                   `json:"article_id"`
	SourceContentHash string                   `json:"source_content_hash"`
	Mode              string                   `json:"mode"`
	Title             string                   `json:"title"`
	BodyHTML          string                   `json:"body_html"`
	Pages             []xiaohongshu.Page       `json:"pages"`
	ScriptPages       []xiaohongshu.ScriptPage `json:"script_pages"`
	Topics            string                   `json:"topics"`
	SourceNote        string                   `json:"source_note"`
	CommentCopy       string                   `json:"comment_copy"`
	AIModel           string                   `json:"ai_model"`
	PromptVersion     string                   `json:"prompt_version"`
	State             string                   `json:"state"`
	Stale             bool                     `json:"stale"`
	CreatedAt         string                   `json:"created_at"`
	UpdatedAt         string                   `json:"updated_at"`
}

// XiaohongshuView 汇总当前文章的小红书草稿和发布状态。
type XiaohongshuView struct {
	ArticleID          string                      `json:"article_id"`
	CurrentContentHash string                      `json:"current_content_hash"`
	TemplateID         string                      `json:"template_id"`
	Mode               string                      `json:"mode"`
	State              string                      `json:"state"`
	Latest             *XiaohongshuDraftView       `json:"latest"`
	History            []XiaohongshuDraftView      `json:"history"`
	Diagnostics        []xiaohongshuDiagnosticView `json:"diagnostics"`
}

type xiaohongshuDiagnosticView struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

type xiaohongshuDraftInput struct {
	DraftID     string                   `json:"draft_id"`
	Mode        string                   `json:"mode"`
	Title       string                   `json:"title"`
	BodyHTML    string                   `json:"body_html"`
	Pages       []xiaohongshu.Page       `json:"pages,omitempty"`
	ScriptPages []xiaohongshu.ScriptPage `json:"script_pages,omitempty"`
	Topics      string                   `json:"topics"`
	SourceNote  string                   `json:"source_note"`
	CommentCopy string                   `json:"comment_copy"`
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

const xiaohongshuOutputSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["title","body_html","topics","source_note","comment_copy"],
  "properties":{
    "title":{"type":"string","description":"适合小红书的短标题，20字以内"},
    "body_html":{"type":"string","description":"提炼后的短正文 HTML，只允许 p、strong、em、ul、ol、li、a，不要 h1"},
	"topics":{"type":"string","description":"直接输出可复制到小红书的格式，例如 #AI编程 #效率工具，话题内部不能有空格"},
    "source_note":{"type":"string"},
    "comment_copy":{"type":"string"}
  }
}`

// xiaohongshu handles article-scoped draft, render and manual publication commands.
func (h *runtimeHandler) xiaohongshu(response http.ResponseWriter, request *http.Request) {
	articleID, suffix, ok := parseXiaohongshuPath(request.URL.Path)
	if !ok {
		writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		return
	}
	var workspaceID string
	if err := h.db.QueryRowContext(request.Context(), `SELECT workspace_id FROM articles WHERE id=? AND deleted_at IS NULL`, articleID).Scan(&workspaceID); errors.Is(err, sql.ErrNoRows) {
		mapError(response, ErrNotFound)
		return
	} else if err != nil {
		mapError(response, err)
		return
	}
	settings, err := loadStoredXiaohongshuSettings(request.Context(), h.db, workspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	if !settings.Enabled {
		writeError(response, http.StatusConflict, "xiaohongshu.not_enabled", "请先在设置中启用小红书发布")
		return
	}
	switch {
	case request.Method == http.MethodGet && suffix == "":
		h.xiaohongshuView(response, request, articleID)
	case request.Method == http.MethodGet && suffix == "history":
		h.xiaohongshuView(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "drafts/outline":
		h.xiaohongshuOutline(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "drafts/rewrite":
		h.xiaohongshuRewrite(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "drafts/generate":
		h.xiaohongshuGenerate(response, request, articleID)
	case request.Method == http.MethodPost && suffix == "drafts/storyboard":
		h.xiaohongshuStoryboard(response, request, articleID)
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
	workspaceID, contentHash, _, rendered, diagnostics, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	settings, err := loadStoredXiaohongshuSettings(request.Context(), h.db, workspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	mode, err := requestedXiaohongshuMode(request)
	if err != nil {
		writeError(response, http.StatusBadRequest, "xiaohongshu.mode_invalid", err.Error())
		return
	}
	drafts, err := repository.NewXiaohongshuRepository(h.db).ListDraftsByMode(request.Context(), workspaceID, articleID, mode, 50)
	if err != nil {
		mapError(response, err)
		return
	}
	history := make([]XiaohongshuDraftView, 0, len(drafts))
	for _, draft := range drafts {
		// 本地图片地址使用进程级签名，读取历史草稿时必须替换为当前有效地址。
		if draft.Mode == xiaohongshu.DraftModeLongCard {
			draft = refreshXiaohongshuDraftAssets(draft, rendered)
		}
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
	diagnosticViews := make([]xiaohongshuDiagnosticView, 0, len(diagnostics))
	for _, item := range diagnostics {
		if item.Code == "publish.section_excluded" {
			diagnosticViews = append(diagnosticViews, xiaohongshuDiagnosticView{Code: item.Code, Message: item.Message, Blocking: item.Blocking})
		}
	}
	writeJSON(response, http.StatusOK, XiaohongshuView{ArticleID: articleID, CurrentContentHash: contentHash, TemplateID: settings.TemplateID, Mode: string(mode), State: state, Latest: latest, History: history, Diagnostics: diagnosticViews})
}

func requestedXiaohongshuMode(request *http.Request) (xiaohongshu.DraftMode, error) {
	mode := xiaohongshu.DraftMode(strings.TrimSpace(request.URL.Query().Get("mode")))
	if mode == "" {
		return xiaohongshu.DraftModeLongCard, nil
	}
	if mode != xiaohongshu.DraftModeLongCard && mode != xiaohongshu.DraftModeVisualScript {
		return "", errors.New("小红书内容模式不可用")
	}
	return mode, nil
}

func (h *runtimeHandler) xiaohongshuGenerate(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, title, rendered, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	source, err := prepareXiaohongshuRewriteSource(rendered)
	if err != nil {
		mapError(response, err)
		return
	}
	provider, err := h.buildXiaohongshuAIProvider(request.Context(), workspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	points, _, err := generateXiaohongshuOutline(request.Context(), provider, title, contentHash, source)
	if err != nil {
		mapError(response, err)
		return
	}
	result, model, err := generateXiaohongshuRewrite(request.Context(), provider, title, contentHash, source, points)
	if err != nil {
		mapError(response, err)
		return
	}
	draft, err := h.saveXiaohongshuAIDraft(request.Context(), articleID, workspaceID, contentHash, model, result, len(points), len(source.Media))
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, xiaohongshuDraftView(draft, contentHash))
}

type xiaohongshuAIGenerated struct {
	Title       string
	BodyHTML    string
	Topics      []string
	SourceNote  string
	CommentCopy string
}

// parseXiaohongshuAIOutput 将 Provider 的结构化字段转换为可持久化的小红书草稿。
func parseXiaohongshuAIOutput(items []contracts.Suggestion) (xiaohongshuAIGenerated, error) {
	values := make(map[string]json.RawMessage, len(items))
	for _, item := range items {
		values[item.Field] = item.Value
	}
	var result xiaohongshuAIGenerated
	if err := json.Unmarshal(values["title"], &result.Title); err != nil || strings.TrimSpace(result.Title) == "" {
		return xiaohongshuAIGenerated{}, errors.New("AI 小红书标题无效")
	}
	if err := json.Unmarshal(values["body_html"], &result.BodyHTML); err != nil || strings.TrimSpace(result.BodyHTML) == "" {
		return xiaohongshuAIGenerated{}, errors.New("AI 小红书正文无效")
	}
	var topicsText string
	if err := json.Unmarshal(values["topics"], &topicsText); err != nil {
		return xiaohongshuAIGenerated{}, errors.New("AI 小红书话题无效")
	}
	result.Topics = parseXiaohongshuTopics(topicsText)
	_ = json.Unmarshal(values["source_note"], &result.SourceNote)
	_ = json.Unmarshal(values["comment_copy"], &result.CommentCopy)
	result.Title, result.SourceNote, result.CommentCopy = strings.TrimSpace(result.Title), strings.TrimSpace(result.SourceNote), strings.TrimSpace(result.CommentCopy)
	return result, nil
}

func (h *runtimeHandler) xiaohongshuSave(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, _, _, _, err := h.xiaohongshuArticle(request.Context(), articleID)
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
	if input.Mode != "" && xiaohongshu.DraftMode(input.Mode) != draft.Mode {
		writeError(response, http.StatusConflict, "xiaohongshu.mode_conflict", "草稿内容模式不匹配")
		return
	}
	if draft.Mode == xiaohongshu.DraftModeVisualScript && len(input.ScriptPages) == 0 {
		writeError(response, http.StatusBadRequest, "request.invalid", "分镜草稿至少需要一页提示词")
		return
	}
	draft.Title, draft.BodyHTML, draft.Topics, draft.SourceNote, draft.CommentCopy = strings.TrimSpace(input.Title), input.BodyHTML, parseXiaohongshuTopics(input.Topics), input.SourceNote, input.CommentCopy
	if input.Pages != nil {
		draft.Pages = input.Pages
	}
	if input.ScriptPages != nil {
		draft.ScriptPages = input.ScriptPages
	}
	if err := repo.SaveDraft(request.Context(), draft); err != nil {
		mapError(response, err)
		return
	}
	_ = repo.SaveEvent(request.Context(), xiaohongshu.Event{ID: stableXiaohongshuID("event", draft.ID, "saved", time.Now().UTC().Format(time.RFC3339Nano)), DraftID: draft.ID, EventType: "saved"})
	writeJSON(response, http.StatusOK, xiaohongshuDraftView(draft, contentHash))
}

func (h *runtimeHandler) xiaohongshuRender(response http.ResponseWriter, request *http.Request, articleID string) {
	workspaceID, contentHash, _, _, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	var input xiaohongshuRenderInput
	if decodeJSON(request, &input) != nil || input.DraftID == "" || input.TemplateID == "" || input.PageCount < 1 || input.ViewportWidth < 280 {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书渲染请求无效")
		return
	}
	if !validXiaohongshuTemplate(input.TemplateID) {
		writeError(response, http.StatusBadRequest, "xiaohongshu.template_invalid", "小红书模板不可用")
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
	workspaceID, contentHash, _, _, _, err := h.xiaohongshuArticle(request.Context(), articleID)
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
func (h *runtimeHandler) xiaohongshuArticle(ctx context.Context, articleID string) (workspaceID, contentHash, title, rendered string, diagnostics []contracts.Diagnostic, err error) {
	var sourceID, stableID, relative string
	if err = h.db.QueryRowContext(ctx, `SELECT workspace_id,source_id,stable_id,relative_path,title,content_hash FROM articles WHERE id=? AND deleted_at IS NULL`, articleID).Scan(&workspaceID, &sourceID, &stableID, &relative, &title, &contentHash); err != nil {
		return "", "", "", "", nil, ErrNotFound
	}
	source, sourceErr := h.buildSource(ctx, sourceID, nil)
	if sourceErr != nil {
		return "", "", "", "", nil, sourceErr
	}
	document, readErr := source.Read(ctx, contracts.SourceRef{SourceID: sourceID, RelativePath: relative, StableID: stableID})
	if readErr != nil {
		return "", "", "", "", nil, readErr
	}
	document, _, err = editorial.ApplyPublicationSectionExclusions(ctx, h.db, workspaceID, document)
	if err != nil {
		return "", "", "", "", nil, err
	}
	// wiki 链接预处理（小红书渠道），将交叉引用转为博客外链，未发布目标保留纯文本。
	linkResolver := editorial.NewArticleLinkResolver(h.db, workspaceID)
	document.Body = editorial.ProcessWebWikiLinks(ctx, linkResolver, document.Body, h.db, workspaceID).Body
	rendered, err = h.renderArticlePreview(ctx, source, document, articleID)
	diagnostics = document.Diagnostics
	return
}

func xiaohongshuDraftView(draft xiaohongshu.Draft, currentHash string) XiaohongshuDraftView {
	return XiaohongshuDraftView{ID: draft.ID, ArticleID: draft.ArticleID, SourceContentHash: draft.SourceContentHash, Mode: string(draft.Mode), Title: draft.Title, BodyHTML: draft.BodyHTML, Pages: draft.Pages, ScriptPages: draft.ScriptPages, Topics: formatXiaohongshuTopics(draft.Topics), SourceNote: draft.SourceNote, CommentCopy: draft.CommentCopy, AIModel: draft.AIModel, PromptVersion: draft.PromptVersion, State: string(draft.State), Stale: draft.SourceContentHash != currentHash, CreatedAt: draft.CreatedAt, UpdatedAt: draft.UpdatedAt}
}

// parseXiaohongshuTopics 将用户或 AI 输出的单行话题文本转换为内部话题数组。
func parseXiaohongshuTopics(value string) []string {
	result := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	for _, item := range strings.Fields(value) {
		item = strings.TrimLeft(strings.TrimSpace(item), "#")
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
		if len(result) == 8 {
			break
		}
	}
	return result
}

// formatXiaohongshuTopics 将内部话题数组格式化为可以直接复制发布的文本。
func formatXiaohongshuTopics(values []string) string {
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimLeft(strings.TrimSpace(value), "#")
		if value != "" {
			formatted = append(formatted, "#"+value)
		}
	}
	return strings.Join(formatted, " ")
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
