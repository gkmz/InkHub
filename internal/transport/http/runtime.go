package httptransport

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	workspaceapp "github.com/gkmz/InkHub/internal/app/workspace"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
	"github.com/yuin/goldmark"
)

// NewRuntimeHandler 提供首次初始化和页面查询端点，并把领域命令交给核心 API。
func NewRuntimeHandler(db *sql.DB, core http.Handler) http.Handler {
	handler := &runtimeHandler{db: db, core: core}
	return localOnly(handler)
}

type runtimeHandler struct {
	db   *sql.DB
	core http.Handler
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
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/jobs/"):
		h.job(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/dashboard":
		writeJSON(response, http.StatusOK, map[string]any{"items": []any{}})
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/taxonomy":
		writeJSON(response, http.StatusOK, map[string]any{"source": "尚未配置", "loaded_at": "-", "readonly": true, "issues": []any{}})
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/settings":
		writeJSON(response, http.StatusOK, defaultSettings())
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/articles/"):
		h.articleDetail(response, request)
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/review"):
		if validateWriteRequest(response, request) {
			h.reviewArticle(response, request)
		}
	default:
		h.core.ServeHTTP(response, request)
	}
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
	var rendered bytes.Buffer
	if err := goldmark.Convert([]byte(document.Body), &rendered); err != nil {
		mapError(response, err)
		return
	}
	var tags, keywords []string
	_ = json.Unmarshal([]byte(tagsJSON), &tags)
	_ = json.Unmarshal([]byte(keywordsJSON), &keywords)
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
	metadata := map[string]any{"title": title, "description": description, "category": category, "series": series, "tags": tags, "keywords": keywords, "slug": slug, "cover": cover}
	writeJSON(response, http.StatusOK, map[string]any{"id": id, "content_version": contentHash, "hugo_provider_id": providers["hugo"], "wechat_provider_id": providers["wechat"], "relative_path": relative, "modified_at": modified, "metadata": metadata, "preview_html": rendered.String(), "source_changed": false, "review_state": reviewState, "hugo_state": "尚未同步", "wechat_state": "尚未准备", "checks": []map[string]string{{"id": "metadata", "level": "passed", "title": "元数据已读取", "detail": "文章来自当前 Vault 内容", "channel": "Hugo · 微信"}}, "ai_configured": false, "suggestions": []any{}, "suggestions_stale": false, "wechat_copied": false})
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
	Name           string `json:"name"`
	VaultPath      string `json:"vault_path"`
	HugoPath       string `json:"hugo_path"`
	WeChatTemplate string `json:"wechat_template"`
	AIEnabled      bool   `json:"ai_enabled"`
}

func (h *runtimeHandler) createWorkspace(response http.ResponseWriter, request *http.Request) {
	var input createWorkspaceRequest
	key := request.Header.Get("Idempotency-Key")
	if decodeJSON(request, &input) != nil || key == "" || input.Name == "" || input.VaultPath == "" {
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
	_, err = tx.ExecContext(request.Context(), `INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,last_used_at=excluded.last_used_at,updated_at=excluded.updated_at`, workspaceID, input.Name, filepath.Dir(vault), now, now, now)
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES(?,?,'obsidian',?,?,?) ON CONFLICT(id) DO NOTHING`, sourceID, workspaceID, vault, now, now)
	}
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES(?,?,'wechat','微信公众号','{"template":"default"}',?,?) ON CONFLICT(id) DO NOTHING`, wechatID, workspaceID, now, now)
	}
	if err == nil && input.HugoPath != "" {
		config, _ := json.Marshal(map[string]string{"root": input.HugoPath})
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
	report, scanErr := scanInitialWorkspace(request, sourceID, workspaceID, vault, h.db)
	state := "succeeded"
	progress := 100
	if scanErr != nil {
		state, progress = "failed", 5
	}
	result, _ := json.Marshal(map[string]int{"indexed": report.Indexed, "failed": report.Failed})
	_, _ = h.db.ExecContext(request.Context(), `UPDATE jobs SET state=?,progress=?,result_json=?,error_code=?,error_message=?,finished_at=?,updated_at=? WHERE id=?`, state, progress, string(result), nullableText(scanErr, "workspace.scan_failed"), nullableError(scanErr), now, now, jobID)
	writeJSON(response, http.StatusCreated, map[string]any{"workspace": map[string]string{"id": workspaceID, "name": input.Name}, "job_id": jobID})
}

func scanInitialWorkspace(request *http.Request, sourceID, workspaceID, vault string, db *sql.DB) (workspaceapp.ScanReport, error) {
	source, err := obsidian.New(obsidian.Config{SourceID: sourceID, Root: vault})
	if err != nil {
		return workspaceapp.ScanReport{}, err
	}
	return workspaceapp.ScanWorkspace(request.Context(), source, repository.NewArticleRepository(db), workspaceapp.ScanOptions{WorkspaceID: workspaceID}, contracts.ScanCursor{})
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
