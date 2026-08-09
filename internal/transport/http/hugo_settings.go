package httptransport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	workspaceapp "github.com/gkmz/InkHub/internal/app/workspace"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

type storedHugoConfig struct {
	Root               string `json:"root"`
	StagingRoot        string `json:"staging_root"`
	ContentDir         string `json:"content_dir,omitempty"`
	Section            string `json:"section"`
	BaseURL            string `json:"base_url,omitempty"`
	ArtifactTTLSeconds int64  `json:"artifact_ttl_seconds,omitempty"`
}

type hugoSettingsRequest struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	BaseURL string `json:"base_url"`
}

// hugoTakeoverReport 汇总一次无副作用的历史内容接管计划。
type hugoTakeoverReport struct {
	BundleCount       int                       `json:"bundle_count"`
	LinkedCount       int                       `json:"linked_count"`
	MatchedCount      int                       `json:"matched_count"`
	ConflictCount     int                       `json:"conflict_count"`
	UnmatchedCount    int                       `json:"unmatched_count"`
	ArticlesMissingID int                       `json:"articles_missing_id"`
	SourceIssueCount  int                       `json:"source_issue_count"`
	SourceIssues      []hugoTakeoverSourceIssue `json:"source_issues"`
	Candidates        []hugoTakeoverCandidate   `json:"candidates"`
}

type hugoTakeoverSourceIssue struct {
	ArticlePath string `json:"article_path"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

type hugoTakeoverCandidate struct {
	ArticleID   string `json:"-"`
	IndexPath   string `json:"-"`
	BundlePath  string `json:"bundle_path"`
	ArticlePath string `json:"article_path,omitempty"`
	Title       string `json:"title"`
	StableID    string `json:"stable_id,omitempty"`
	Status      string `json:"status"`
	MatchReason string `json:"match_reason,omitempty"`
}

type takeoverArticle struct {
	ID          string
	SourceID    string
	StableID    string
	Path        string
	Title       string
	URL         string
	Date        string
	Fingerprint string
	ContentHash string
}

type takeoverIdentityWriter interface {
	WriteTakeoverIdentity(ctx context.Context, command contracts.MetadataWriteCommand) (contracts.SourceDocument, error)
}

// saveHugoSettings 校验并保存当前工作区的 Hugo 目录配置。
func (h *runtimeHandler) saveHugoSettings(response http.ResponseWriter, request *http.Request) {
	var input hugoSettingsRequest
	if decodeJSON(request, &input) != nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "Hugo 设置请求无效")
		return
	}
	workspaceID, err := h.currentWorkspaceID(request.Context())
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	root := strings.TrimSpace(input.Path)
	var site hugo.SiteInfo
	if input.Enabled {
		var inspectErr error
		site, inspectErr = hugo.InspectSite(root)
		if inspectErr != nil {
			writeError(response, http.StatusUnprocessableEntity, "hugo.site_invalid", inspectErr.Error())
			return
		}
		root = site.Root
		if _, scanErr := hugo.ScanTakeoverBundlesAt(root, site.ContentDir); scanErr != nil {
			writeError(response, http.StatusUnprocessableEntity, "hugo.site_invalid", scanErr.Error())
			return
		}
	}
	providerID, existing, _, found := loadStoredHugoConfig(request.Context(), h.db, workspaceID)
	if !found {
		providerID = stableRuntimeID("hugo", workspaceID)
		existing.Section = "posts"
		existing.StagingRoot = filepath.Join(h.dataDir, "staging", "hugo", workspaceID)
	}
	if existing.Section == "" {
		existing.Section = "posts"
	}
	if existing.StagingRoot == "" {
		existing.StagingRoot = filepath.Join(h.dataDir, "staging", "hugo", workspaceID)
	}
	existing.Root = root
	if input.Enabled {
		existing.ContentDir = site.ContentDir
	}
	existing.BaseURL = strings.TrimSpace(input.BaseURL)
	encoded, _ := json.Marshal(existing)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = h.db.ExecContext(request.Context(), `INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at)
VALUES(?,?,'hugo','Hugo',?,?,?,?)
ON CONFLICT(workspace_id,provider_type) DO UPDATE SET enabled=excluded.enabled,config_json=excluded.config_json,updated_at=excluded.updated_at`, providerID, workspaceID, input.Enabled, string(encoded), now, now)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"hugo_enabled": input.Enabled, "hugo_path": root, "hugo_base_url": existing.BaseURL, "hugo_valid": input.Enabled})
}

// previewHugoTakeover 计算 Hugo 与 Obsidian 的确定匹配和冲突，不修改文件。
func (h *runtimeHandler) previewHugoTakeover(response http.ResponseWriter, request *http.Request) {
	report, err := h.buildHugoTakeoverReport(request.Context())
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "hugo.takeover_preview_failed", err.Error())
		return
	}
	writeJSON(response, http.StatusOK, report)
}

// confirmHugoTakeover 批量补齐源身份，并只接管无歧义的历史 Bundle。
func (h *runtimeHandler) confirmHugoTakeover(response http.ResponseWriter, request *http.Request) {
	h.metadataWriteMu.Lock()
	defer h.metadataWriteMu.Unlock()
	report, err := h.buildHugoTakeoverReport(request.Context())
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "hugo.takeover_preview_failed", err.Error())
		return
	}
	if report.ConflictCount > 0 {
		writeError(response, http.StatusConflict, "hugo.takeover_conflict", "存在无法唯一确认的历史 Bundle，请先处理冲突")
		return
	}
	workspaceID, sourceID, source, articles, err := h.takeoverArticles(request.Context())
	if err != nil {
		mapError(response, err)
		return
	}
	assigned := 0
	recovered := 0
	identityWriter, supportsTakeoverWrite := source.(takeoverIdentityWriter)
	for _, value := range articles {
		if value.StableID != "" {
			continue
		}
		stableID, idErr := newArticleStableID()
		if idErr != nil {
			mapError(response, idErr)
			return
		}
		command := contracts.MetadataWriteCommand{
			Ref: contracts.SourceRef{SourceID: sourceID, RelativePath: value.Path}, ExpectedFingerprint: value.Fingerprint,
			Patch: contracts.MetadataPatch{StableID: &stableID},
		}
		var writeErr error
		if supportsTakeoverWrite {
			_, writeErr = identityWriter.WriteTakeoverIdentity(request.Context(), command)
		} else {
			_, writeErr = source.WriteMetadata(request.Context(), command)
		}
		if writeErr != nil {
			writeError(response, http.StatusConflict, "hugo.takeover_source_changed", "补齐文章 ID 时源文件发生变化，请重新扫描后重试")
			return
		}
		if err := h.adoptArticleStableID(request.Context(), value.ID, stableID); err != nil {
			mapError(response, err)
			return
		}
		assigned++
	}
	if !supportsTakeoverWrite {
		writeError(response, http.StatusUnprocessableEntity, "hugo.takeover_source_unsupported", "当前内容源不支持历史文章接管")
		return
	}
	// 未进入索引的旧文章也必须在一次性接管中修复，否则它们会永久游离在内容库之外。
	sourceScan, err := source.Scan(request.Context(), contracts.ScanCursor{})
	if err != nil {
		mapError(response, err)
		return
	}
	for _, reference := range sourceScan.Documents {
		if !hasBlockingTakeoverDiagnostic(reference.Diagnostics, "obsidian.read_failed") {
			continue
		}
		stableID, idErr := newArticleStableID()
		if idErr != nil {
			mapError(response, idErr)
			return
		}
		_, writeErr := identityWriter.WriteTakeoverIdentity(request.Context(), contracts.MetadataWriteCommand{
			Ref: reference.Ref, ExpectedFingerprint: reference.Fingerprint, Patch: contracts.MetadataPatch{StableID: &stableID},
		})
		if writeErr != nil {
			writeError(response, http.StatusUnprocessableEntity, "hugo.takeover_source_repair_failed", fmt.Sprintf("无法规范化历史文章 %s: %v", reference.Ref.RelativePath, writeErr))
			return
		}
		assigned++
		recovered++
	}
	// 所有 Obsidian 文件写回完成后统一重扫，保留文章内部 ID 和既有业务历史。
	if _, err := workspaceapp.ScanWorkspace(request.Context(), source, repository.NewArticleRepository(h.db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{}); err != nil {
		mapError(response, err)
		return
	}
	refreshed, err := h.buildHugoTakeoverReport(request.Context())
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "hugo.takeover_refresh_failed", err.Error())
		return
	}
	for _, candidate := range refreshed.Candidates {
		if candidate.Status != "matched" {
			continue
		}
		bundle := hugo.TakeoverBundle{IndexPath: candidate.IndexPath, BundlePath: candidate.BundlePath}
		if err := hugo.WriteTakeoverIdentity(bundle, candidate.StableID, candidate.ArticlePath); err != nil {
			writeError(response, http.StatusUnprocessableEntity, "hugo.takeover_write_failed", err.Error())
			return
		}
	}
	if err := h.seedHugoTakeover(request.Context(), workspaceID, refreshed.Candidates); err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"assigned_ids": assigned, "recovered_articles": recovered, "linked_bundles": refreshed.MatchedCount, "remaining_source_issues": refreshed.SourceIssueCount, "state": "completed"})
}

func hasBlockingTakeoverDiagnostic(diagnostics []contracts.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Blocking && diagnostic.Code == code {
			return true
		}
	}
	return false
}

func (h *runtimeHandler) buildHugoTakeoverReport(ctx context.Context) (hugoTakeoverReport, error) {
	workspaceID, _, source, articles, err := h.takeoverArticles(ctx)
	if err != nil {
		return hugoTakeoverReport{}, err
	}
	_, config, enabled, found := loadStoredHugoConfig(ctx, h.db, workspaceID)
	if !found || !enabled || strings.TrimSpace(config.Root) == "" {
		return hugoTakeoverReport{}, fmt.Errorf("请先启用并保存 Hugo 根目录")
	}
	bundles, err := hugo.ScanTakeoverBundlesAt(config.Root, config.ContentDir)
	if err != nil {
		return hugoTakeoverReport{}, err
	}
	report := hugoTakeoverReport{BundleCount: len(bundles), Candidates: make([]hugoTakeoverCandidate, 0, len(bundles)), SourceIssues: []hugoTakeoverSourceIssue{}}
	scan, scanErr := source.Scan(ctx, contracts.ScanCursor{})
	if scanErr != nil {
		return hugoTakeoverReport{}, scanErr
	}
	for _, reference := range scan.Documents {
		for _, diagnostic := range reference.Diagnostics {
			if !diagnostic.Blocking {
				continue
			}
			report.SourceIssues = append(report.SourceIssues, hugoTakeoverSourceIssue{ArticlePath: reference.Ref.RelativePath, Code: diagnostic.Code, Message: diagnostic.Message})
		}
	}
	report.SourceIssueCount = len(report.SourceIssues)
	for _, value := range articles {
		if value.StableID == "" {
			report.ArticlesMissingID++
		}
	}
	for _, bundle := range bundles {
		if bundle.SourceID != "" {
			report.LinkedCount++
		}
		report.Candidates = append(report.Candidates, matchTakeoverBundle(bundle, articles))
	}
	resolveDuplicateTakeoverMatches(report.Candidates)
	for _, candidate := range report.Candidates {
		switch candidate.Status {
		case "matched":
			report.MatchedCount++
		case "conflict":
			report.ConflictCount++
		default:
			report.UnmatchedCount++
		}
	}
	return report, nil
}

func resolveDuplicateTakeoverMatches(candidates []hugoTakeoverCandidate) {
	groups := map[string][]int{}
	for index, candidate := range candidates {
		if candidate.Status == "matched" {
			groups[candidate.ArticleID] = append(groups[candidate.ArticleID], index)
		}
	}
	for _, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		authoritative := -1
		for _, index := range indexes {
			if candidates[index].MatchReason == "source_id" {
				if authoritative != -1 {
					authoritative = -2
					break
				}
				authoritative = index
			}
		}
		for _, index := range indexes {
			if index == authoritative {
				continue
			}
			if authoritative >= 0 {
				candidates[index].Status = "unmatched"
				candidates[index].MatchReason = "同一文章已有 source_id 明确关联的 Bundle"
			} else {
				candidates[index].Status = "conflict"
				candidates[index].MatchReason = "同一文章对应多个历史 Bundle，无法自动选择"
			}
		}
	}
}

func (h *runtimeHandler) takeoverArticles(ctx context.Context) (string, string, contracts.SourceProvider, []takeoverArticle, error) {
	var workspaceID, sourceID string
	if err := h.db.QueryRowContext(ctx, `SELECT workspaces.id,sources.id FROM workspaces JOIN sources ON sources.workspace_id=workspaces.id ORDER BY workspaces.last_used_at DESC LIMIT 1`).Scan(&workspaceID, &sourceID); err != nil {
		return "", "", nil, nil, err
	}
	source, err := h.buildSource(ctx, sourceID, nil)
	if err != nil {
		return "", "", nil, nil, err
	}
	rows, err := h.db.QueryContext(ctx, `SELECT id,stable_id,relative_path,title,source_fingerprint,content_hash FROM articles WHERE workspace_id=? AND source_id=? AND deleted_at IS NULL ORDER BY relative_path`, workspaceID, sourceID)
	if err != nil {
		return "", "", nil, nil, err
	}
	defer rows.Close()
	articles := make([]takeoverArticle, 0)
	for rows.Next() {
		var value takeoverArticle
		value.SourceID = sourceID
		if err := rows.Scan(&value.ID, &value.StableID, &value.Path, &value.Title, &value.Fingerprint, &value.ContentHash); err != nil {
			return "", "", nil, nil, err
		}
		articles = append(articles, value)
	}
	if err := rows.Err(); err != nil {
		return "", "", nil, nil, err
	}
	for index := range articles {
		document, readErr := source.Read(ctx, contracts.SourceRef{SourceID: sourceID, RelativePath: articles[index].Path, StableID: articles[index].StableID})
		if readErr != nil {
			// 旧 frontmatter 的类型问题由确认接管时的一次性写回规范化，不阻断预览。
			continue
		}
		articles[index].URL = document.Article.URL
		articles[index].Date = document.Article.PublishDate
	}
	return workspaceID, sourceID, source, articles, nil
}

func matchTakeoverBundle(bundle hugo.TakeoverBundle, articles []takeoverArticle) hugoTakeoverCandidate {
	base := hugoTakeoverCandidate{IndexPath: bundle.IndexPath, BundlePath: bundle.BundlePath, Title: bundle.Title, Status: "unmatched"}
	match := func(reason string, predicate func(takeoverArticle) bool) hugoTakeoverCandidate {
		values := make([]takeoverArticle, 0, 1)
		for _, article := range articles {
			if predicate(article) {
				values = append(values, article)
			}
		}
		if len(values) == 1 {
			value := values[0]
			return hugoTakeoverCandidate{ArticleID: value.ID, IndexPath: bundle.IndexPath, BundlePath: bundle.BundlePath, ArticlePath: value.Path, Title: value.Title, StableID: value.StableID, Status: "matched", MatchReason: reason}
		}
		if len(values) > 1 {
			base.Status = "conflict"
			base.MatchReason = reason + " 匹配到多篇文章"
		}
		return base
	}
	if bundle.SourceID != "" {
		candidate := match("source_id", func(value takeoverArticle) bool { return value.StableID == bundle.SourceID })
		if candidate.Status == "unmatched" {
			// Hugo 中可能保留当前管理范围之外的历史文章；这类 Bundle 保持原样，不阻断其他文章接管。
			candidate.MatchReason = "Hugo source_id 未在当前内容库中找到，保持原样"
		}
		return candidate
	}
	if bundle.SourcePath != "" {
		candidate := match("source_path", func(value takeoverArticle) bool {
			return filepath.ToSlash(value.Path) == filepath.ToSlash(bundle.SourcePath)
		})
		if candidate.Status != "unmatched" {
			return candidate
		}
	}
	if normalized := normalizeTakeoverText(bundle.URL); normalized != "" {
		candidate := match("url", func(value takeoverArticle) bool { return normalizeTakeoverText(value.URL) == normalized })
		if candidate.Status != "unmatched" {
			return candidate
		}
	}
	title, date := normalizeTakeoverText(bundle.Title), normalizeTakeoverDate(bundle.Date)
	if title != "" && date != "" {
		candidate := match("path_title_date", func(value takeoverArticle) bool {
			return normalizeTakeoverText(value.Title) == title && normalizeTakeoverDate(value.Date) == date && takeoverPathEvidence(bundle.BundlePath, value.Path, title)
		})
		if candidate.Status != "unmatched" {
			return candidate
		}
	}
	return base
}

func normalizeTakeoverText(value string) string {
	var result strings.Builder
	for _, current := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			result.WriteRune(current)
		}
	}
	return result.String()
}

func normalizeTakeoverDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func takeoverPathEvidence(bundlePath, articlePath, normalizedTitle string) bool {
	bundle := normalizeTakeoverText(bundlePath)
	articleName := strings.TrimSuffix(filepath.Base(articlePath), filepath.Ext(articlePath))
	article := normalizeTakeoverText(articleName)
	if article != "" && strings.Contains(bundle, article) {
		return true
	}
	runes := []rune(normalizedTitle)
	if len(runes) > 8 {
		runes = runes[:8]
	}
	return len(runes) >= 4 && strings.Contains(bundle, string(runes))
}

func (h *runtimeHandler) seedHugoTakeover(ctx context.Context, workspaceID string, candidates []hugoTakeoverCandidate) error {
	providerID, _, enabled, found := loadStoredHugoConfig(ctx, h.db, workspaceID)
	if !found || !enabled {
		return fmt.Errorf("Hugo Provider 未启用")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, candidate := range candidates {
		if candidate.Status != "matched" {
			continue
		}
		var contentHash string
		if err := tx.QueryRowContext(ctx, `SELECT content_hash FROM articles WHERE id=? AND workspace_id=?`, candidate.ArticleID, workspaceID).Scan(&contentHash); err != nil {
			return err
		}
		publicationID := stableRuntimeID("publication", candidate.ArticleID+"\x00"+providerID)
		_, err = tx.ExecContext(ctx, `INSERT INTO publications(id,article_id,provider_instance_id,workspace_id,state,content_hash,last_processed_at,created_at,updated_at)
VALUES(?,?,?,?,'published',?,?,?,?)
ON CONFLICT(article_id,provider_instance_id) DO UPDATE SET state='published',content_hash=excluded.content_hash,last_error_code=NULL,last_error_message=NULL,last_processed_at=excluded.last_processed_at,updated_at=excluded.updated_at`, publicationID, candidate.ArticleID, providerID, workspaceID, contentHash, now, now, now)
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM publications WHERE article_id=? AND provider_instance_id=?`, candidate.ArticleID, providerID).Scan(&publicationID); err != nil {
			return err
		}
		eventID := stableRuntimeID("event", publicationID+"\x00takeover\x00"+contentHash)
		payload, _ := json.Marshal(map[string]any{"source": "hugo_takeover", "external": true, "bundle_path": candidate.BundlePath})
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO publication_events(id,publication_id,event_type,content_hash,payload_json,created_at) VALUES(?,?,'marked_published',?,?,?)`, eventID, publicationID, contentHash, string(payload), now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,cleared_at,created_at,updated_at)
VALUES(?,?,'published',?,NULL,?,?)
ON CONFLICT(article_id) DO UPDATE SET workspace_id=excluded.workspace_id,kind='published',content_hash=excluded.content_hash,cleared_at=NULL,updated_at=excluded.updated_at`, candidate.ArticleID, workspaceID, contentHash, now, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h *runtimeHandler) currentWorkspaceID(ctx context.Context) (string, error) {
	var workspaceID string
	err := h.db.QueryRowContext(ctx, `SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1`).Scan(&workspaceID)
	return workspaceID, err
}

func loadStoredHugoConfig(ctx context.Context, db *sql.DB, workspaceID string) (string, storedHugoConfig, bool, bool) {
	var providerID, raw string
	var enabled bool
	err := db.QueryRowContext(ctx, `SELECT id,config_json,enabled FROM provider_instances WHERE workspace_id=? AND provider_type='hugo'`, workspaceID).Scan(&providerID, &raw, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return "", storedHugoConfig{}, false, false
	}
	if err != nil {
		return "", storedHugoConfig{}, false, false
	}
	var config storedHugoConfig
	if json.Unmarshal([]byte(raw), &config) != nil {
		return providerID, storedHugoConfig{}, enabled, true
	}
	return providerID, config, enabled, true
}
