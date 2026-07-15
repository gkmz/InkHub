package httptransport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	workspaceapp "github.com/gkmz/InkHub/internal/app/workspace"
	"github.com/gkmz/InkHub/internal/platform/dialog"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/source/folder"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// RuntimeOptions 提供运行期持久化目录和操作系统能力。
type RuntimeOptions struct {
	DataDir               string
	DirectoryPicker       dialog.DirectoryPicker
	AssetTokenKey         []byte
	AfterWorkspaceCreated func(context.Context) (string, error)
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
	if len(options) > 0 {
		afterWorkspaceCreated = options[0].AfterWorkspaceCreated
	}
	handler := &runtimeHandler{db: db, core: core, dataDir: dataDir, directoryPicker: directoryPicker, assetTokenKey: assetTokenKey, afterWorkspaceCreated: afterWorkspaceCreated}
	return localOnly(handler)
}

type runtimeHandler struct {
	db                    *sql.DB
	core                  http.Handler
	dataDir               string
	directoryPicker       dialog.DirectoryPicker
	assetTokenKey         []byte
	afterWorkspaceCreated func(context.Context) (string, error)
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
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/directories/pick":
		if validateWriteRequest(response, request) {
			h.pickDirectory(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/directories/inspect":
		if validateWriteRequest(response, request) {
			h.inspectDirectories(response, request)
		}
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/jobs/"):
		h.job(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/dashboard":
		writeJSON(response, http.StatusOK, map[string]any{"items": []any{}})
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/taxonomy":
		writeJSON(response, http.StatusOK, map[string]any{"source": "尚未配置", "loaded_at": "-", "readonly": true, "issues": []any{}})
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/settings":
		h.settings(response, request)
	case request.Method == http.MethodPut && request.URL.Path == "/api/v1/settings/content-scope":
		if validateWriteRequest(response, request) {
			h.saveContentScope(response, request)
		}
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/settings/content-scope/preview":
		if validateWriteRequest(response, request) {
			h.previewContentScope(response, request)
		}
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/wechat/content/"):
		h.wechatContent(response, request)
	case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/assets/") && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		h.articleAsset(response, request)
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
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/metadata")
	var input metadataRequest
	if decodeJSON(request, &input) != nil || input.Metadata.Title == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "元数据请求无效")
		return
	}
	var workspaceID, sourceID, stableID, relative, fingerprint, root string
	err := h.db.QueryRowContext(request.Context(), `SELECT articles.workspace_id,articles.source_id,articles.stable_id,articles.relative_path,articles.source_fingerprint,sources.root_path FROM articles JOIN sources ON sources.id=articles.source_id WHERE articles.id=?`, articleID).Scan(&workspaceID, &sourceID, &stableID, &relative, &fingerprint, &root)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	source, err := obsidian.New(obsidian.Config{SourceID: sourceID, Root: root})
	if err != nil {
		mapError(response, err)
		return
	}
	_, err = source.WriteMetadata(request.Context(), contracts.MetadataWriteCommand{Ref: contracts.SourceRef{SourceID: sourceID, RelativePath: relative, StableID: stableID}, ExpectedFingerprint: fingerprint, Patch: contracts.MetadataPatch{Title: &input.Metadata.Title, Description: &input.Metadata.Description, Category: &input.Metadata.Category, Series: &input.Metadata.Series, Tags: &input.Metadata.Tags, Keywords: &input.Metadata.Keywords, Slug: &input.Metadata.Slug, Cover: &input.Metadata.Cover}})
	if errors.Is(err, obsidian.ErrSourceChanged) {
		mapError(response, ErrStaleContent)
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if _, err := workspaceapp.ScanWorkspace(request.Context(), source, repository.NewArticleRepository(h.db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{}); err != nil {
		mapError(response, err)
		return
	}
	request.URL.Path = "/api/v1/articles/" + articleID
	h.articleDetail(response, request)
}

func (h *runtimeHandler) wechatContent(response http.ResponseWriter, request *http.Request) {
	articleID := strings.TrimPrefix(request.URL.Path, "/api/v1/wechat/content/")
	var resultJSON string
	err := h.db.QueryRowContext(request.Context(), `SELECT jobs.result_json FROM jobs WHERE jobs.kind='wechat_prepare' AND jobs.state='succeeded' AND json_extract(jobs.payload_json,'$.article_id')=? ORDER BY jobs.finished_at DESC LIMIT 1`, articleID).Scan(&resultJSON)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	var result struct {
		Location string `json:"location"`
	}
	if json.Unmarshal([]byte(resultJSON), &result) != nil || result.Location == "" {
		mapError(response, ErrNotFound)
		return
	}
	content, err := os.ReadFile(result.Location)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"html": string(content)})
}

func (h *runtimeHandler) articleDetail(response http.ResponseWriter, request *http.Request) {
	id := strings.TrimPrefix(request.URL.Path, "/api/v1/articles/")
	var workspaceID, sourceID, stableID, relative, title, description, category, series, tagsJSON, keywordsJSON, slug, cover, contentHash, modified string
	err := h.db.QueryRowContext(request.Context(), `SELECT workspace_id,source_id,stable_id,relative_path,title,description,category,series,tags_json,keywords_json,slug,cover,content_hash,COALESCE(source_mtime,updated_at) FROM articles WHERE id=? AND deleted_at IS NULL`, id).Scan(&workspaceID, &sourceID, &stableID, &relative, &title, &description, &category, &series, &tagsJSON, &keywordsJSON, &slug, &cover, &contentHash, &modified)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	var root string
	if err := h.db.QueryRowContext(request.Context(), `SELECT root_path FROM sources WHERE id=?`, sourceID).Scan(&root); err != nil {
		mapError(response, err)
		return
	}
	source, err := obsidian.New(obsidian.Config{SourceID: sourceID, Root: root})
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
	providers := map[string]string{"hugo": "", "wechat": ""}
	rows, _ := h.db.QueryContext(request.Context(), `SELECT provider_type,id FROM provider_instances WHERE workspace_id=?`, workspaceID)
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
	for channel, providerID := range providers {
		if providerID == "" {
			continue
		}
		var stored string
		if h.db.QueryRowContext(request.Context(), `SELECT CASE WHEN content_hash<>? THEN 'outdated' ELSE state END FROM publications WHERE article_id=? AND provider_instance_id=?`, contentHash, id, providerID).Scan(&stored) == nil {
			label := runtimePublicationLabel(stored, channel)
			if channel == "hugo" {
				hugoState = label
			} else {
				wechatState, wechatCopied = label, stored == "copied" || stored == "confirmed"
			}
		}
	}
	metadata := map[string]any{"title": title, "description": description, "category": category, "series": series, "tags": tags, "keywords": keywords, "slug": slug, "cover": cover}
	writeJSON(response, http.StatusOK, map[string]any{"id": id, "content_version": contentHash, "hugo_provider_id": providers["hugo"], "wechat_provider_id": providers["wechat"], "relative_path": relative, "modified_at": modified, "metadata": metadata, "preview_html": rendered, "source_changed": false, "review_state": reviewState, "hugo_state": hugoState, "wechat_state": wechatState, "checks": []map[string]string{{"id": "metadata", "level": "passed", "title": "元数据已读取", "detail": "文章来自当前 Vault 内容", "channel": "Hugo · 微信"}}, "ai_configured": false, "suggestions": []any{}, "suggestions_stale": false, "wechat_copied": wechatCopied})
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
	var contentHash, frontmatterHash string
	if err := h.db.QueryRowContext(request.Context(), `SELECT content_hash,frontmatter_hash FROM articles WHERE id=?`, id).Scan(&contentHash, &frontmatterHash); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := h.db.ExecContext(request.Context(), `INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,approved_at,approved_by,updated_at) VALUES(?,'approved',?,?,?,'user',?) ON CONFLICT(article_id) DO UPDATE SET state='approved',approved_content_hash=excluded.approved_content_hash,approved_frontmatter_hash=excluded.approved_frontmatter_hash,approved_at=excluded.approved_at,approved_by='user',updated_at=excluded.updated_at`, id, contentHash, frontmatterHash, now, now)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"state": "approved"})
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
	writeJSON(response, http.StatusOK, map[string]any{"has_workspace": true, "workspace": map[string]string{"id": id, "name": name}})
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
	if err == nil && input.HugoPath != "" {
		config, _ := json.Marshal(map[string]string{"root": input.HugoPath, "staging_root": filepath.Join(h.dataDir, "staging", "hugo", workspaceID), "section": "posts"})
		_, err = tx.ExecContext(request.Context(), `INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES(?,?,'hugo','Hugo',?,?,?) ON CONFLICT(id) DO NOTHING`, hugoID, workspaceID, string(config), now, now)
	}
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO jobs(id,workspace_id,kind,dedupe_key,state,progress,result_json,available_at,created_at,updated_at) VALUES(?,?,'workspace.scan',?,'running',5,'{}',?,?,?) ON CONFLICT(id) DO NOTHING`, jobID, workspaceID, "scan:"+workspaceID, now, now, now)
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if err := tx.Commit(); err != nil {
		mapError(response, err)
		return
	}
	report, scanErr := scanInitialWorkspace(request, sourceID, workspaceID, vault, scope.ContentRoots(), scope.IgnoredFolders(), scope.IgnoredFileNames(), h.db)
	state := "succeeded"
	progress := 100
	if scanErr != nil {
		state, progress = "failed", 5
	}
	result, _ := json.Marshal(map[string]int{"indexed": report.Indexed, "failed": report.Failed})
	_, _ = h.db.ExecContext(request.Context(), `UPDATE jobs SET state=?,progress=?,result_json=?,error_code=?,error_message=?,finished_at=?,updated_at=? WHERE id=?`, state, progress, string(result), nullableText(scanErr, "workspace.scan_failed"), nullableError(scanErr), now, now, jobID)
	taxonomyState := "not_enabled"
	if h.afterWorkspaceCreated != nil {
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

func scanInitialWorkspace(request *http.Request, sourceID, workspaceID, vault string, contentRoots, ignoredFolders, ignoredFileNames []string, db *sql.DB) (workspaceapp.ScanReport, error) {
	source, err := obsidian.New(obsidian.Config{SourceID: sourceID, Root: vault, ContentRoots: contentRoots, IgnoredFolders: ignoredFolders, IgnoredFileNames: ignoredFileNames})
	if err != nil {
		return workspaceapp.ScanReport{}, err
	}
	return workspaceapp.ScanWorkspace(request.Context(), source, repository.NewArticleRepository(db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{})
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
	var result string
	if err := h.db.QueryRowContext(request.Context(), `SELECT state,progress,result_json FROM jobs WHERE id=?`, id).Scan(&state, &progress, &result); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	counts := map[string]int{"indexed": 0, "failed": 0}
	_ = json.Unmarshal([]byte(result), &counts)
	writeJSON(response, http.StatusOK, map[string]any{"id": id, "state": state, "progress": progress, "indexed": counts["indexed"], "failed": counts["failed"]})
}

func stableRuntimeID(kind, key string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + key))
	return kind + "_" + hex.EncodeToString(sum[:12])
}

func defaultSettings() map[string]any {
	return map[string]any{"ai_enabled": false, "ai_secret_saved": false, "hugo_enabled": false, "wechat_enabled": true, "wechat_secret_saved": false, "default_template": "default", "templates": []map[string]any{{"id": "default", "name": "InkHub Default", "version": "1.0.0", "compatible": true}, {"id": "minimal", "name": "InkHub Minimal", "version": "1.0.0", "compatible": true}}, "diagnostics": []map[string]string{{"name": "工作区", "state": "正常", "message": "本地数据库可用"}, {"name": "AI", "state": "未启用", "message": "不影响手工审核"}}}
}
