package httptransport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gkmz/InkHub/internal/app/editorial"
	workspaceapp "github.com/gkmz/InkHub/internal/app/workspace"
	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/platform/dialog"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
	"github.com/gkmz/InkHub/internal/provider/source/folder"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// RuntimeOptions 提供运行期持久化目录和操作系统能力。
type RuntimeOptions struct {
	DataDir               string
	DirectoryPicker       dialog.DirectoryPicker
	AssetTokenKey         []byte
	AfterWorkspaceCreated func(context.Context) (string, error)
	ProviderRuntime       contracts.ProviderRuntime
	RefreshTaxonomy       func(context.Context) error
	AISecrets             AISecretStore
	HugoPreviews          HugoPreviewAPI
	PublicationWorkflows  PublicationWorkflowAPI
	WeChatPlans           WeChatPlanAPI
}

// NewRuntimeHandler 提供首次初始化和页面查询端点，并把领域命令交给核心 API。
func NewRuntimeHandler(db *sql.DB, core http.Handler, options ...RuntimeOptions) http.Handler {
	dataDir := os.TempDir()
	if len(options) > 0 && options[0].DataDir != "" {
		dataDir = options[0].DataDir
	}
	var directoryPicker dialog.DirectoryPicker = dialog.NativePicker{}
	if len(options) > 0 && options[0].DirectoryPicker != nil {
		directoryPicker = options[0].DirectoryPicker
	}
	assetTokenKey := newAssetTokenKey()
	if len(options) > 0 && len(options[0].AssetTokenKey) >= 32 {
		assetTokenKey = append([]byte(nil), options[0].AssetTokenKey...)
	}
	var afterWorkspaceCreated func(context.Context) (string, error)
	var providerRuntime contracts.ProviderRuntime
	var refreshTaxonomy func(context.Context) error
	var aiSecrets AISecretStore
	var hugoPreviews HugoPreviewAPI
	var publicationWorkflows PublicationWorkflowAPI
	var wechatPlans WeChatPlanAPI
	if len(options) > 0 {
		afterWorkspaceCreated = options[0].AfterWorkspaceCreated
		providerRuntime = options[0].ProviderRuntime
		refreshTaxonomy = options[0].RefreshTaxonomy
		aiSecrets = options[0].AISecrets
		hugoPreviews = options[0].HugoPreviews
		publicationWorkflows = options[0].PublicationWorkflows
		wechatPlans = options[0].WeChatPlans
	}
	handler := &runtimeHandler{db: db, core: core, dataDir: dataDir, directoryPicker: directoryPicker, assetTokenKey: assetTokenKey, afterWorkspaceCreated: afterWorkspaceCreated, providerRuntime: providerRuntime, refreshTaxonomy: refreshTaxonomy, aiSecrets: aiSecrets, hugoPreviews: hugoPreviews, publicationWorkflows: publicationWorkflows, wechatPlans: wechatPlans}
	return localOnly(handler)
}

type runtimeHandler struct {
	db                    *sql.DB
	core                  http.Handler
	dataDir               string
	directoryPicker       dialog.DirectoryPicker
	assetTokenKey         []byte
	afterWorkspaceCreated func(context.Context) (string, error)
	providerRuntime       contracts.ProviderRuntime
	refreshTaxonomy       func(context.Context) error
	aiSecrets             AISecretStore
	hugoPreviews          HugoPreviewAPI
	publicationWorkflows  PublicationWorkflowAPI
	wechatPlans           WeChatPlanAPI
	metadataWriteMu       sync.Mutex
}

func (h *runtimeHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/session":
		h.session(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/workspaces":
		if validateWriteRequest(response, request) {
			h.createWorkspace(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/workspace/refresh":
		if validateWriteRequest(response, request) {
			h.refreshWorkspace(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/workspace/initialize":
		if validateWriteRequest(response, request) {
			h.retryWorkspaceInitialization(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/directories/pick":
		if validateWriteRequest(response, request) {
			h.pickDirectory(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/directories/inspect":
		if validateWriteRequest(response, request) {
			h.inspectDirectories(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/directories/inspect-hugo":
		if validateWriteRequest(response, request) {
			h.inspectHugoDirectory(response, request)
		}
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/jobs/"):
		h.job(response, request)
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/hugo-sections") && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		h.hugoSections(response, request)
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/publication-workflow") && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		h.publicationWorkflow(response, request)
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/publication-history") && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		h.publicationHistory(response, request)
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/hugo-previews") && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		if validateWriteRequest(response, request) {
			h.createHugoPreview(response, request)
		}
	case request.Method == http.MethodGet && isHugoPreviewRenderPath(request.URL.Path):
		h.hugoPreviewRender(response, request)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/hugo-previews/"):
		h.hugoPreview(response, request)
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/confirm") && strings.HasPrefix(request.URL.Path, "/api/v1/hugo-previews/"):
		if validateWriteRequest(response, request) {
			h.confirmHugoPreview(response, request)
		}
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/taxonomy":
		h.taxonomyOverview(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/taxonomy/refresh":
		if validateWriteRequest(response, request) {
			h.refreshTaxonomyOverview(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/taxonomy/terms/preview":
		if validateWriteRequest(response, request) {
			h.previewTaxonomyTerm(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/taxonomy/terms/apply":
		if validateWriteRequest(response, request) {
			h.applyTaxonomyTerm(response, request)
		}
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/settings":
		h.settings(response, request)
	case request.Method == http.MethodPut && request.URL.Path == "/api/v1/settings/ai":
		if validateWriteRequest(response, request) {
			h.saveAISettings(response, request)
		}
	case request.Method == http.MethodPut && request.URL.Path == "/api/v1/settings/wechat":
		if validateWriteRequest(response, request) {
			h.saveWeChatSettings(response, request)
		}
	case request.Method == http.MethodPut && request.URL.Path == "/api/v1/settings/xiaohongshu":
		if validateWriteRequest(response, request) {
			h.saveXiaohongshuSettings(response, request)
		}
	case request.Method == http.MethodPut && request.URL.Path == "/api/v1/settings/hugo":
		if validateWriteRequest(response, request) {
			h.saveHugoSettings(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/settings/hugo/takeover/preview":
		if validateWriteRequest(response, request) {
			h.previewHugoTakeover(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/settings/hugo/takeover/confirm":
		if validateWriteRequest(response, request) {
			h.confirmHugoTakeover(response, request)
		}
	case request.Method == http.MethodPut && request.URL.Path == "/api/v1/settings/content-scope":
		if validateWriteRequest(response, request) {
			h.saveContentScope(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/settings/content-scope/preview":
		if validateWriteRequest(response, request) {
			h.previewContentScope(response, request)
		}
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/wechat-plans/confirm"):
		if validateWriteRequest(response, request) {
			h.confirmWeChatPlan(response, request)
		}
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/wechat-plans"):
		if validateWriteRequest(response, request) {
			h.createWeChatPlan(response, request)
		}
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/wechat/content/"):
		h.wechatContent(response, request)
	case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/assets/") && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		h.articleAsset(response, request)
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/suggestions"):
		if validateWriteRequest(response, request) {
			h.generateArticleSuggestions(response, request)
		}
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/actions") && strings.Contains(request.URL.Path, "/suggestions/"):
		if validateWriteRequest(response, request) {
			articleID, suggestionID, ok := parseSuggestionActionPath(request.URL.Path)
			if !ok {
				writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
			} else {
				h.updateSuggestionItems(response, request, articleID, suggestionID)
			}
		}
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/articles/") && strings.Contains(request.URL.Path, "/suggestions"):
		articleID, suggestionID, ok := parseSuggestionPath(request.URL.Path)
		if !ok {
			writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		} else if suggestionID == "" {
			h.suggestionHistory(response, request, articleID)
		} else {
			h.suggestionVersion(response, request, articleID, suggestionID)
		}
	case strings.HasPrefix(request.URL.Path, "/api/v1/articles/") && strings.Contains(request.URL.Path, "/xiaohongshu"):
		if request.Method != http.MethodGet && !validateWriteRequest(response, request) {
			return
		}
		h.xiaohongshu(response, request)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		h.articleDetail(response, request)
	case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/metadata"):
		if validateWriteRequest(response, request) {
			h.writeMetadata(response, request)
		}
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/review"):
		if validateWriteRequest(response, request) {
			h.reviewArticle(response, request)
		}
	default:
		h.core.ServeHTTP(response, request)
	}
}

// pickDirectory 通过本机原生对话框选择目录，并只向同源页面返回规范化路径。
func (h *runtimeHandler) pickDirectory(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Purpose string `json:"purpose"`
	}
	if decodeJSON(request, &input) != nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "目录选择请求无效")
		return
	}
	titles := map[string]string{
		"vault": "选择 Obsidian Vault",
		"hugo":  "选择 Hugo 项目根目录",
	}
	title, supported := titles[input.Purpose]
	if !supported {
		writeError(response, http.StatusBadRequest, "request.invalid", "目录选择用途无效")
		return
	}
	selected, err := h.directoryPicker.Pick(request.Context(), title)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "directory.pick_failed", "未能选择目录")
		return
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		writeError(response, http.StatusUnprocessableEntity, "directory.not_selected", "未选择目录")
		return
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "directory.path_invalid", "所选目录路径无效")
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"path": filepath.Clean(absolute)})
}

type metadataRequest struct {
	Metadata struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Category    string   `json:"category"`
		Series      string   `json:"series"`
		Tags        []string `json:"tags"`
		Keywords    []string `json:"keywords"`
		Slug        string   `json:"slug"`
		Cover       string   `json:"cover"`
	} `json:"metadata"`
}

func (h *runtimeHandler) writeMetadata(response http.ResponseWriter, request *http.Request) {
	// 本地单用户服务按请求串行化源文件写回，确保首次生成身份时文件与索引不会分叉。
	h.metadataWriteMu.Lock()
	defer h.metadataWriteMu.Unlock()
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/metadata")
	var input metadataRequest
	if decodeJSON(request, &input) != nil || input.Metadata.Title == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "元数据请求无效")
		return
	}
	var workspaceID, sourceID, stableID, relative, fingerprint string
	err := h.db.QueryRowContext(request.Context(), `SELECT articles.workspace_id,articles.source_id,articles.stable_id,articles.relative_path,articles.source_fingerprint FROM articles WHERE articles.id=?`, articleID).Scan(&workspaceID, &sourceID, &stableID, &relative, &fingerprint)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	sourceStableID := stableID
	patch := contracts.MetadataPatch{Title: &input.Metadata.Title, Description: &input.Metadata.Description, Category: &input.Metadata.Category, Series: &input.Metadata.Series, Tags: &input.Metadata.Tags, Keywords: &input.Metadata.Keywords, Slug: &input.Metadata.Slug, Cover: &input.Metadata.Cover}
	if stableID == "" {
		// 首次受控保存时补充稳定身份，扫描和普通读取不会主动修改用户文件。
		stableID, err = newArticleStableID()
		if err != nil {
			mapError(response, err)
			return
		}
		patch.StableID = &stableID
	}
	source, err := h.buildSource(request.Context(), sourceID, nil)
	if err != nil {
		mapError(response, err)
		return
	}
	written, err := source.WriteMetadata(request.Context(), contracts.MetadataWriteCommand{Ref: contracts.SourceRef{SourceID: sourceID, RelativePath: relative, StableID: sourceStableID}, ExpectedFingerprint: fingerprint, Patch: patch})
	if sourceConflict(err) {
		mapError(response, ErrStaleContent)
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if sourceStableID == "" {
		if err := h.adoptArticleStableID(request.Context(), articleID, stableID); err != nil {
			mapError(response, err)
			return
		}
	}
	if _, err := workspaceapp.ScanWorkspace(request.Context(), source, repository.NewArticleRepository(h.db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{}); err != nil {
		mapError(response, err)
		return
	}
	var indexedStableID, indexedFingerprint string
	if err := h.db.QueryRowContext(request.Context(), `SELECT stable_id,source_fingerprint FROM articles WHERE id=? AND deleted_at IS NULL`, articleID).Scan(&indexedStableID, &indexedFingerprint); err != nil || indexedStableID != stableID || indexedFingerprint != written.Fingerprint {
		writeError(response, http.StatusInternalServerError, "article.index_refresh_failed", "文章已写入，但索引刷新失败，请刷新工作区后重试")
		return
	}
	request.URL.Path = "/api/v1/articles/" + articleID
	h.articleDetail(response, request)
}

func (h *runtimeHandler) articleDetail(response http.ResponseWriter, request *http.Request) {
	id := strings.TrimPrefix(request.URL.Path, "/api/v1/articles/")
	var workspaceID, sourceID, stableID, relative, title, description, category, series, tagsJSON, keywordsJSON, slug, cover, contentHash, sourceFingerprint, modified, contentStage, contentStageIssue string
	err := h.db.QueryRowContext(request.Context(), `SELECT workspace_id,source_id,stable_id,relative_path,title,description,category,series,tags_json,keywords_json,slug,cover,content_hash,source_fingerprint,COALESCE(source_mtime,updated_at),content_stage,content_stage_issue FROM articles WHERE id=? AND deleted_at IS NULL`, id).Scan(&workspaceID, &sourceID, &stableID, &relative, &title, &description, &category, &series, &tagsJSON, &keywordsJSON, &slug, &cover, &contentHash, &sourceFingerprint, &modified, &contentStage, &contentStageIssue)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	source, err := h.buildSource(request.Context(), sourceID, nil)
	if err != nil {
		mapError(response, err)
		return
	}
	document, err := source.Read(request.Context(), contracts.SourceRef{SourceID: sourceID, RelativePath: relative, StableID: stableID})
	if err != nil {
		mapError(response, err)
		return
	}
	rendered, err := h.renderArticlePreview(request.Context(), source, document, id)
	if err != nil {
		mapError(response, err)
		return
	}
	var tags, keywords []string
	_ = json.Unmarshal([]byte(tagsJSON), &tags)
	_ = json.Unmarshal([]byte(keywordsJSON), &keywords)
	if tags == nil {
		tags = []string{}
	}
	if keywords == nil {
		keywords = []string{}
	}
	reviewState := "等待审核"
	_ = h.db.QueryRowContext(request.Context(), `SELECT CASE state WHEN 'approved' THEN '已通过' WHEN 'changed' THEN '内容已更新' WHEN 'blocked' THEN '处理失败' ELSE '等待审核' END FROM editorial_reviews WHERE article_id=?`, id).Scan(&reviewState)
	providers := map[string]string{"hugo": "", "wechat": "", "openai-compatible": ""}
	rows, _ := h.db.QueryContext(request.Context(), `SELECT provider_type,id FROM provider_instances WHERE workspace_id=? AND enabled=1`, workspaceID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var kind, providerID string
			if rows.Scan(&kind, &providerID) == nil {
				providers[kind] = providerID
			}
		}
	}
	hugoState, wechatState, wechatCopied := "尚未同步", "尚未准备", false
	xiaohongshuState := "尚未准备"
	xiaohongshuSettings, err := loadStoredXiaohongshuSettings(request.Context(), h.db, workspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	if !xiaohongshuSettings.Enabled {
		xiaohongshuState = "未配置"
	}
	if contentStage != "ready" {
		reviewState, hugoState, wechatState = "不适用", "未进入发布流程", "未进入发布流程"
	}
	for channel, providerID := range providers {
		if contentStage != "ready" {
			break
		}
		if providerID == "" {
			continue
		}
		var stored, storedHash string
		if h.db.QueryRowContext(request.Context(), `SELECT state,content_hash FROM publications WHERE article_id=? AND provider_instance_id=?`, id, providerID).Scan(&stored, &storedHash) == nil {
			if stored != "confirmed" && storedHash != contentHash {
				stored = "outdated"
			}
			label := runtimePublicationLabel(stored, channel)
			if channel == "hugo" {
				hugoState = label
			} else {
				wechatState, wechatCopied = label, stored == "copied" || stored == "confirmed"
			}
		}
	}
	var xhsDraftState, xhsDraftHash string
	if xiaohongshuSettings.Enabled && h.db.QueryRowContext(request.Context(), `SELECT state,source_content_hash FROM xiaohongshu_drafts WHERE article_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, id).Scan(&xhsDraftState, &xhsDraftHash) == nil {
		xiaohongshuState = runtimeXiaohongshuLabel(xhsDraftState, xhsDraftHash, contentHash)
	}
	metadata := map[string]any{"title": title, "description": description, "category": category, "series": series, "tags": tags, "keywords": keywords, "slug": slug, "cover": cover}
	suggestionItems := []articleSuggestionView{}
	suggestionsStale := false
	suggestionsID := ""
	suggestionsGeneratedAt := ""
	if latest, found, findErr := repository.NewSuggestionRepository(h.db).FindLatestByArticle(request.Context(), workspaceID, id); findErr == nil && found {
		suggestionItems = suggestionViews(pendingSuggestionItems(latest.Items))
		suggestionsStale = latest.InputContentHash != contentHash
		suggestionsID = latest.ID
		suggestionsGeneratedAt = latest.CreatedAt
	}
	disposition, err := h.effectiveArticleDisposition(request.Context(), workspaceID, id, contentHash)
	if err != nil {
		mapError(response, err)
		return
	}
	resourceDiagnostics := make([]map[string]any, 0)
	for _, diagnostic := range document.Diagnostics {
		if strings.HasPrefix(diagnostic.Code, "source.") {
			resourceDiagnostics = append(resourceDiagnostics, map[string]any{"code": diagnostic.Code, "message": diagnostic.Message, "blocking": diagnostic.Blocking})
		}
	}
	checks := articleCheckViews(editorial.CheckArticle(document.Article, document.Body))
	result := map[string]any{"id": id, "stable_id": stableID, "content_version": contentHash, "content_stage": contentStage, "content_stage_issue": contentStageIssue, "hugo_provider_id": providers["hugo"], "wechat_provider_id": providers["wechat"], "relative_path": relative, "modified_at": modified, "metadata": metadata, "preview_html": rendered, "source_changed": sourceFingerprint != document.Fingerprint, "review_state": reviewState, "hugo_state": hugoState, "wechat_state": wechatState, "xiaohongshu_enabled": xiaohongshuSettings.Enabled, "xiaohongshu_state": xiaohongshuState, "resource_diagnostics": resourceDiagnostics, "checks": checks, "ai_configured": providers["openai-compatible"] != "", "suggestions": suggestionItems, "suggestions_id": suggestionsID, "suggestions_generated_at": suggestionsGeneratedAt, "suggestions_stale": suggestionsStale, "wechat_copied": wechatCopied}
	if disposition != nil {
		result["disposition"] = disposition
	}
	writeJSON(response, http.StatusOK, result)
}

func newArticleStableID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "article_" + hex.EncodeToString(random[:]), nil
}

type articleDispositionView struct {
	Kind     string   `json:"kind"`
	Channels []string `json:"channels"`
}

func (h *runtimeHandler) effectiveArticleDisposition(ctx context.Context, workspaceID, articleID, contentHash string) (*articleDispositionView, error) {
	var kind string
	err := h.db.QueryRowContext(ctx, `SELECT kind FROM article_dispositions
WHERE article_id=? AND workspace_id=? AND cleared_at IS NULL
AND (kind='ignored' OR (kind='published' AND content_hash=?))`, articleID, workspaceID, contentHash).Scan(&kind)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	view := &articleDispositionView{Kind: kind, Channels: []string{}}
	if kind == "ignored" {
		return view, nil
	}
	// 详情只返回当前版本已标记的渠道名称，不暴露 Provider 实例 ID。
	rows, err := h.db.QueryContext(ctx, `SELECT providers.provider_type
FROM publications
JOIN provider_instances providers ON providers.id=publications.provider_instance_id AND providers.workspace_id=publications.workspace_id
WHERE publications.article_id=? AND publications.workspace_id=? AND publications.state='published'
AND publications.content_hash=? AND providers.enabled=1 AND providers.provider_type IN ('hugo','wechat')
ORDER BY CASE providers.provider_type WHEN 'hugo' THEN 1 ELSE 2 END`, articleID, workspaceID, contentHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return nil, err
		}
		view.Channels = append(view.Channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return view, nil
}

func runtimePublicationLabel(state, channel string) string {
	switch state {
	case "published":
		return "已同步"
	case "prepared":
		return "已准备"
	case "copied":
		return "已复制"
	case "confirmed":
		return "已确认草稿"
	case "failed":
		return "处理失败"
	case "outdated":
		if channel == "hugo" {
			return "需要同步"
		}
		return "草稿可能过期"
	}
	if channel == "hugo" {
		return "尚未同步"
	}
	return "尚未准备"
}

func (h *runtimeHandler) reviewArticle(response http.ResponseWriter, request *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/review")
	// 审核是文章首次进入发布流程的边界，在这里自动建立稳定身份。
	h.metadataWriteMu.Lock()
	defer h.metadataWriteMu.Unlock()
	var stableID, contentHash, bodyHash, frontmatterHash, contentStage string
	var workspaceID, sourceID, relativePath, fingerprint string
	if err := h.db.QueryRowContext(request.Context(), `SELECT workspace_id,source_id,stable_id,relative_path,source_fingerprint,content_hash,body_hash,frontmatter_hash,content_stage FROM articles WHERE id=?`, id).Scan(&workspaceID, &sourceID, &stableID, &relativePath, &fingerprint, &contentHash, &bodyHash, &frontmatterHash, &contentStage); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	if contentStage != "ready" {
		mapError(response, ErrArticleNotReady)
		return
	}
	if stableID != "" {
		if err := article.StableID(stableID).Validate(); err != nil {
			writeError(response, http.StatusUnprocessableEntity, "article.identity_invalid", "文章稳定 ID 格式无效，请修复源文件后重试")
			return
		}
	}
	source, buildErr := h.buildSource(request.Context(), sourceID, nil)
	if buildErr != nil {
		mapError(response, buildErr)
		return
	}
	document, readErr := source.Read(request.Context(), contracts.SourceRef{SourceID: sourceID, RelativePath: relativePath, StableID: stableID})
	if readErr != nil {
		mapError(response, readErr)
		return
	}
	if document.Fingerprint != fingerprint {
		mapError(response, ErrStaleContent)
		return
	}
	checks := editorial.CheckArticle(document.Article, document.Body)
	if hasBlockingCheck(checks) {
		writeJSON(response, http.StatusUnprocessableEntity, map[string]any{"error": map[string]string{"code": "article.checks_blocking", "message": "文章存在必须修复的 SEO 或正文问题"}, "checks": articleCheckViews(checks)})
		return
	}
	if stableID == "" {
		var err error
		stableID, err = newArticleStableID()
		if err != nil {
			mapError(response, err)
			return
		}
		_, writeErr := source.WriteMetadata(request.Context(), contracts.MetadataWriteCommand{
			Ref: contracts.SourceRef{SourceID: sourceID, RelativePath: relativePath}, ExpectedFingerprint: fingerprint,
			Patch: contracts.MetadataPatch{StableID: &stableID},
		})
		if sourceConflict(writeErr) {
			mapError(response, ErrStaleContent)
			return
		}
		if writeErr != nil {
			mapError(response, writeErr)
			return
		}
		if err := h.adoptArticleStableID(request.Context(), id, stableID); err != nil {
			mapError(response, err)
			return
		}
		if _, scanErr := workspaceapp.ScanWorkspace(request.Context(), source, repository.NewArticleRepository(h.db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{}); scanErr != nil {
			mapError(response, scanErr)
			return
		}
		if err := h.db.QueryRowContext(request.Context(), `SELECT stable_id,content_hash,body_hash,frontmatter_hash,content_stage FROM articles WHERE id=? AND deleted_at IS NULL`, id).Scan(&stableID, &contentHash, &bodyHash, &frontmatterHash, &contentStage); err != nil {
			writeError(response, http.StatusInternalServerError, "article.index_refresh_failed", "文章 ID 已写入，但索引刷新失败，请刷新工作区后重试")
			return
		}
	}
	if err := article.StableID(stableID).Validate(); err != nil {
		writeError(response, http.StatusUnprocessableEntity, "article.identity_invalid", "文章稳定 ID 格式无效，请修复源文件后重试")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := h.db.ExecContext(request.Context(), `INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_body_hash,approved_frontmatter_hash,approved_at,approved_by,updated_at) VALUES(?,'approved',?,?,?,?,'user',?) ON CONFLICT(article_id) DO UPDATE SET state='approved',approved_content_hash=excluded.approved_content_hash,approved_body_hash=excluded.approved_body_hash,approved_frontmatter_hash=excluded.approved_frontmatter_hash,approved_at=excluded.approved_at,approved_by='user',updated_at=excluded.updated_at`, id, contentHash, bodyHash, frontmatterHash, now, now)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"state": "approved"})
}

// articleCheckViews 将领域检查转换为页面可直接展示的检查结果。
func articleCheckViews(findings []editorial.Finding) []map[string]string {
	result := make([]map[string]string, 0, len(findings))
	for index, finding := range findings {
		result = append(result, map[string]string{"id": fmt.Sprintf("check-%d-%s", index, finding.Code), "level": string(finding.Severity), "title": finding.Code, "detail": finding.Message, "channel": "SEO · 内容"})
	}
	if len(result) == 0 {
		result = append(result, map[string]string{"id": "checks-passed", "level": "passed", "title": "SEO 检查通过", "detail": "未发现阻断发布的问题", "channel": "SEO · 内容"})
	}
	return result
}

func hasBlockingCheck(findings []editorial.Finding) bool {
	for _, finding := range findings {
		if finding.Severity == editorial.SeverityBlocking {
			return true
		}
	}
	return false
}

// adoptArticleStableID 在源文件写回后显式绑定内部文章记录，普通扫描不再猜测历史路径。
func (h *runtimeHandler) adoptArticleStableID(ctx context.Context, articleID, stableID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := h.db.ExecContext(ctx, `UPDATE articles SET stable_id=?,updated_at=? WHERE id=? AND stable_id=''`, stableID, now, articleID)
	if err != nil {
		return fmt.Errorf("绑定文章稳定身份: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取文章身份绑定结果: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("文章稳定身份已变化，请刷新后重试")
	}
	return nil
}

func (h *runtimeHandler) session(response http.ResponseWriter, request *http.Request) {
	var id, name string
	err := h.db.QueryRowContext(request.Context(), `SELECT id,name FROM workspaces ORDER BY last_used_at DESC LIMIT 1`).Scan(&id, &name)
	if err == sql.ErrNoRows {
		writeJSON(response, http.StatusOK, map[string]any{"has_workspace": false, "workspace": nil})
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	initialization := map[string]any{"required": false, "job_id": "", "state": "succeeded"}
	var initializationJobID, initializationState string
	err = h.db.QueryRowContext(request.Context(), `SELECT id,state FROM jobs WHERE workspace_id=? AND kind='workspace.initialize' ORDER BY created_at DESC LIMIT 1`, id).Scan(&initializationJobID, &initializationState)
	if err != nil && err != sql.ErrNoRows {
		mapError(response, err)
		return
	}
	if err == nil {
		initialization = map[string]any{"required": initializationState != "succeeded", "job_id": initializationJobID, "state": initializationState}
	}
	writeJSON(response, http.StatusOK, map[string]any{"has_workspace": true, "workspace": map[string]string{"id": id, "name": name}, "initialization": initialization})
}

type createWorkspaceRequest struct {
	Name             string   `json:"name"`
	VaultPath        string   `json:"vault_path"`
	ContentRoots     []string `json:"content_roots"`
	IgnoredFolders   []string `json:"ignored_folders"`
	IgnoredFileNames []string `json:"ignored_file_names"`
	HugoPath         string   `json:"hugo_path"`
	WeChatTemplate   string   `json:"wechat_template"`
	AIEnabled        bool     `json:"ai_enabled"`
}

func (h *runtimeHandler) createWorkspace(response http.ResponseWriter, request *http.Request) {
	var input createWorkspaceRequest
	key := request.Header.Get("Idempotency-Key")
	if decodeJSON(request, &input) != nil || key == "" || input.Name == "" || input.VaultPath == "" || len(input.ContentRoots) == 0 {
		writeError(response, http.StatusBadRequest, "request.invalid", "工作区请求无效")
		return
	}
	vault, err := filepath.Abs(input.VaultPath)
	if err != nil {
		writeError(response, http.StatusBadRequest, "workspace.path_invalid", "内容库路径无效")
		return
	}
	if info, err := os.Stat(filepath.Join(vault, ".obsidian")); err != nil || !info.IsDir() {
		writeError(response, http.StatusBadRequest, "workspace.not_obsidian", "所选目录不是 Obsidian Vault")
		return
	}
	ignoredFileNames := input.IgnoredFileNames
	if ignoredFileNames == nil {
		ignoredFileNames = folder.DefaultIgnoredFileNames()
	}
	scope, err := folder.NewScopeWithFileNames(input.ContentRoots, input.IgnoredFolders, ignoredFileNames)
	if err != nil {
		writeError(response, http.StatusBadRequest, "workspace.scope_invalid", "内容目录规则无效")
		return
	}
	var hugoSite hugo.SiteInfo
	defaultSection := "posts"
	if strings.TrimSpace(input.HugoPath) != "" {
		hugoSite, err = hugo.InspectSite(input.HugoPath)
		if err != nil {
			writeError(response, http.StatusUnprocessableEntity, "hugo.site_invalid", err.Error())
			return
		}
		sections, sectionErr := inspectHugoSections(hugoSite)
		if sectionErr != nil {
			writeError(response, http.StatusUnprocessableEntity, "hugo.content_unavailable", sectionErr.Error())
			return
		}
		defaultSection = preferredHugoSection(sections)
	}
	workspaceID := stableRuntimeID("workspace", key)
	sourceID := stableRuntimeID("source", key)
	jobID := stableRuntimeID("job", key)
	hugoID := stableRuntimeID("hugo", key)
	wechatID := stableRuntimeID("wechat", key)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := h.db.BeginTx(request.Context(), nil)
	if err != nil {
		mapError(response, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(request.Context(), `INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,last_used_at=excluded.last_used_at,updated_at=excluded.updated_at`, workspaceID, input.Name, h.dataDir, now, now, now)
	if err == nil {
		sourceConfig, _ := json.Marshal(map[string][]string{"content_roots": scope.ContentRoots(), "ignored_folders": scope.IgnoredFolders(), "ignored_file_names": scope.IgnoredFileNames()})
		_, err = tx.ExecContext(request.Context(), `INSERT INTO sources(id,workspace_id,provider_type,root_path,config_json,created_at,updated_at) VALUES(?,?,'obsidian',?,?,?,?) ON CONFLICT(id) DO NOTHING`, sourceID, workspaceID, vault, string(sourceConfig), now, now)
	}
	if err == nil {
		wechatConfig, _ := json.Marshal(map[string]string{"template": "default", "staging_root": filepath.Join(h.dataDir, "staging", "wechat", workspaceID)})
		_, err = tx.ExecContext(request.Context(), `INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES(?,?,'wechat','微信公众号',?,?,?) ON CONFLICT(id) DO NOTHING`, wechatID, workspaceID, string(wechatConfig), now, now)
	}
	if err == nil && hugoSite.Root != "" {
		config, _ := json.Marshal(map[string]string{"root": hugoSite.Root, "staging_root": filepath.Join(h.dataDir, "staging", "hugo", workspaceID), "content_dir": hugoSite.ContentDir, "section": defaultSection})
		_, err = tx.ExecContext(request.Context(), `INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES(?,?,'hugo','Hugo',?,?,?) ON CONFLICT(id) DO NOTHING`, hugoID, workspaceID, string(config), now, now)
	}
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO jobs(id,workspace_id,kind,dedupe_key,state,progress,result_json,available_at,created_at,updated_at) VALUES(?,?,'workspace.initialize',?,'running',5,'{}',?,?,?) ON CONFLICT(id) DO NOTHING`, jobID, workspaceID, "initialize:"+workspaceID, now, now, now)
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if err := tx.Commit(); err != nil {
		mapError(response, err)
		return
	}
	report, scanErr := h.initializeWorkspaceSource(request.Context(), sourceID, workspaceID, nil)
	state := "succeeded"
	progress := 100
	if scanErr != nil {
		state, progress = "failed", 5
	}
	result, _ := json.Marshal(report)
	_, _ = h.db.ExecContext(request.Context(), `UPDATE jobs SET state=?,progress=?,result_json=?,error_code=?,error_message=?,finished_at=?,updated_at=? WHERE id=?`, state, progress, string(result), nullableText(scanErr, "workspace.initialize_failed"), nullableError(scanErr), now, now, jobID)
	taxonomyState := "not_enabled"
	if scanErr == nil && h.afterWorkspaceCreated != nil {
		var refreshErr error
		taxonomyState, refreshErr = h.afterWorkspaceCreated(request.Context())
		if refreshErr != nil {
			taxonomyState = "needs_attention"
		} else if taxonomyState == "" {
			taxonomyState = "not_enabled"
		}
	}
	writeJSON(response, http.StatusCreated, map[string]any{"workspace": map[string]string{"id": workspaceID, "name": input.Name}, "job_id": jobID, "taxonomy_state": taxonomyState})
}

// refreshWorkspace 重新扫描最近工作区的内容源，并返回本次索引统计。
func (h *runtimeHandler) refreshWorkspace(response http.ResponseWriter, request *http.Request) {
	var sourceID, workspaceID string
	err := h.db.QueryRowContext(request.Context(), `SELECT sources.id,sources.workspace_id FROM sources JOIN workspaces ON workspaces.id=sources.workspace_id ORDER BY workspaces.last_used_at DESC LIMIT 1`).Scan(&sourceID, &workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			mapError(response, ErrNotFound)
			return
		}
		mapError(response, err)
		return
	}
	report, err := h.scanWorkspace(request.Context(), sourceID, workspaceID, nil)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]int{"indexed": report.Indexed, "failed": report.Failed})
}

func (h *runtimeHandler) scanWorkspace(ctx context.Context, sourceID, workspaceID string, overrideConfig []byte) (workspaceapp.ScanReport, error) {
	source, err := h.buildSource(ctx, sourceID, overrideConfig)
	if err != nil {
		return workspaceapp.ScanReport{}, err
	}
	return workspaceapp.ScanWorkspace(ctx, source, repository.NewArticleRepository(h.db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{})
}

// retryWorkspaceInitialization 重新执行最近工作区未完成的身份初始化任务。
func (h *runtimeHandler) retryWorkspaceInitialization(response http.ResponseWriter, request *http.Request) {
	var workspaceID, sourceID, jobID string
	err := h.db.QueryRowContext(request.Context(), `SELECT workspaces.id,sources.id,jobs.id
FROM workspaces
JOIN sources ON sources.workspace_id=workspaces.id
JOIN jobs ON jobs.workspace_id=workspaces.id AND jobs.kind='workspace.initialize'
ORDER BY workspaces.last_used_at DESC,jobs.created_at DESC LIMIT 1`).Scan(&workspaceID, &sourceID, &jobID)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = h.db.ExecContext(request.Context(), `UPDATE jobs SET state='running',progress=5,result_json='{}',error_code=NULL,error_message=NULL,finished_at=NULL,updated_at=? WHERE id=?`, now, jobID)
	report, initializationErr := h.initializeWorkspaceSource(request.Context(), sourceID, workspaceID, nil)
	state, progress := "succeeded", 100
	if initializationErr != nil {
		state, progress = "failed", 5
	}
	result, _ := json.Marshal(report)
	_, _ = h.db.ExecContext(request.Context(), `UPDATE jobs SET state=?,progress=?,result_json=?,error_code=?,error_message=?,finished_at=?,updated_at=? WHERE id=?`, state, progress, string(result), nullableText(initializationErr, "workspace.initialize_failed"), nullableError(initializationErr), now, now, jobID)
	writeJSON(response, http.StatusOK, map[string]string{"job_id": jobID, "state": state})
}

func nullableText(err error, value string) any {
	if err == nil {
		return nil
	}
	return value
}
func nullableError(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}

func (h *runtimeHandler) job(response http.ResponseWriter, request *http.Request) {
	id := strings.TrimPrefix(request.URL.Path, "/api/v1/jobs/")
	var state string
	var progress int
	var result, errorMessage string
	if err := h.db.QueryRowContext(request.Context(), `SELECT state,progress,result_json,COALESCE(error_message,'') FROM jobs WHERE id=?`, id).Scan(&state, &progress, &result, &errorMessage); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	report := workspaceInitializationReport{Issues: []workspaceInitializationIssue{}}
	_ = json.Unmarshal([]byte(result), &report)
	writeJSON(response, http.StatusOK, map[string]any{"id": id, "state": state, "progress": progress, "indexed": report.Indexed, "assigned_ids": report.AssignedIDs, "failed": report.Failed, "issues": report.Issues, "error_message": errorMessage})
}

func stableRuntimeID(kind, key string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + key))
	return kind + "_" + hex.EncodeToString(sum[:12])
}

func defaultSettings() map[string]any {
	return map[string]any{"ai_enabled": false, "ai_secret_saved": false, "hugo_enabled": false, "hugo_path": "", "hugo_base_url": "", "hugo_valid": false, "hugo_bundle_count": 0, "hugo_linked_count": 0, "hugo_unlinked_count": 0, "hugo_conflict_count": 0, "wechat_enabled": true, "wechat_secret_saved": false, "github_token_saved": false, "github_owner": "", "github_repository": "", "github_branch": "main", "github_prefix": "inkhub", "default_template": "default", "templates": []map[string]any{{"id": "default", "name": "InkHub 墨绿", "version": "1.0.0", "compatible": true}}, "xiaohongshu_enabled": true, "xiaohongshu_template": xiaohongshuDefaultTemplateID, "xiaohongshu_templates": xiaohongshuTemplateSummaries(), "diagnostics": []map[string]string{{"name": "工作区", "state": "正常", "message": "本地数据库可用"}, {"name": "AI", "state": "未启用", "message": "不影响手工审核"}}}
}
