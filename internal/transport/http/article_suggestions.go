package httptransport

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	editorialapp "github.com/gkmz/InkHub/internal/app/editorial"
	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

type articleSuggestionView struct {
	ID         string `json:"id"`
	Field      string `json:"field"`
	Name       string `json:"name"`
	Value      any    `json:"value,omitempty"`
	Reason     string `json:"reason"`
	NewTerm    bool   `json:"new_term"`
	UsageCount int    `json:"usage_count"`
	Accepted   bool   `json:"accepted"`
	Ignored    bool   `json:"ignored"`
	Status     string `json:"status"`
}

type articleSuggestionHistoryView struct {
	ID               string `json:"id"`
	GeneratedAt      string `json:"generated_at"`
	Model            string `json:"model"`
	InputContentHash string `json:"input_content_hash"`
	State            string `json:"state"`
	SuggestionCount  int    `json:"suggestion_count"`
	Current          bool   `json:"current"`
}

type articleSuggestionHistoryResponse struct {
	Items    []articleSuggestionHistoryView `json:"items"`
	LatestID string                         `json:"latest_id,omitempty"`
}

type articleSuggestionDetailView struct {
	ID               string                  `json:"id"`
	GeneratedAt      string                  `json:"generated_at"`
	Model            string                  `json:"model"`
	InputContentHash string                  `json:"input_content_hash"`
	State            string                  `json:"state"`
	Suggestions      []articleSuggestionView `json:"suggestions"`
	SuggestionsStale bool                    `json:"suggestions_stale"`
}

type suggestionActionRequest struct {
	Action  string   `json:"action"`
	ItemIDs []string `json:"item_ids"`
}

// updateSuggestionItems 持久化一批建议项的采用或忽略动作，并返回完整版本供历史审计使用。
func (h *runtimeHandler) updateSuggestionItems(response http.ResponseWriter, request *http.Request, articleID, suggestionID string) {
	current, err := h.loadSuggestionArticle(request, articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	var input suggestionActionRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "AI 建议操作参数无效")
		return
	}
	if input.Action != "accepted" && input.Action != "ignored" {
		writeError(response, http.StatusBadRequest, "request.invalid", "AI 建议操作必须是 accepted 或 ignored")
		return
	}
	store := repository.NewSuggestionRepository(h.db)
	storedVersion, err := store.FindByArticleID(request.Context(), current.WorkspaceID, current.ID, suggestionID)
	if errors.Is(err, sql.ErrNoRows) {
		mapError(response, ErrNotFound)
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	if storedVersion.InputContentHash != current.ContentHash {
		writeError(response, http.StatusConflict, "suggestion.stale", "文章已更新，请重新生成 AI 建议")
		return
	}
	version, err := store.UpdateItemStates(request.Context(), current.WorkspaceID, current.ID, suggestionID, input.Action, input.ItemIDs)
	if err != nil {
		if strings.Contains(err.Error(), "已经处理") {
			writeError(response, http.StatusConflict, "suggestion.already_processed", err.Error())
			return
		}
		if strings.Contains(err.Error(), "找不到") || strings.Contains(err.Error(), "不能为空") {
			writeError(response, http.StatusBadRequest, "request.invalid", err.Error())
			return
		}
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, suggestionDetailView(version, current.ContentHash))
}

// generateArticleSuggestions 通过当前工作区 AI Provider 生成并持久化结构化建议。
func (h *runtimeHandler) generateArticleSuggestions(response http.ResponseWriter, request *http.Request) {
	if h.providerRuntime == nil {
		writeError(response, http.StatusConflict, "ai.not_configured", "尚未配置 AI Provider")
		return
	}
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/suggestions")
	current, err := h.loadSuggestionArticle(request, articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	var providerID, rawConfig string
	if err := h.db.QueryRowContext(request.Context(), `SELECT id,config_json FROM provider_instances WHERE workspace_id=? AND provider_type='openai-compatible' AND enabled=1`, current.WorkspaceID).Scan(&providerID, &rawConfig); err != nil {
		writeError(response, http.StatusConflict, "ai.not_configured", "尚未配置 AI Provider")
		return
	}
	var stored storedAIConfig
	if json.Unmarshal([]byte(rawConfig), &stored) != nil {
		writeError(response, http.StatusInternalServerError, "ai.config_invalid", "AI Provider 配置损坏")
		return
	}
	providerConfig, _ := json.Marshal(map[string]any{"base_url": stored.BaseURL, "model": stored.Model, "timeout": stored.Timeout})
	provider, err := h.providerRuntime.BuildAI(request.Context(), contracts.ProviderRef{ID: providerID, Type: contracts.ProviderOpenAI}, contracts.ConfigView{Data: providerConfig, SecretRefs: map[string]string{"api_key": stored.SecretRef}})
	if err != nil {
		mapError(response, err)
		return
	}
	taxonomy, candidates, err := h.loadSuggestionTaxonomy(request, current.WorkspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	suggestionID, err := newSuggestionID()
	if err != nil {
		mapError(response, err)
		return
	}
	result, err := editorialapp.GenerateSuggestions(request.Context(), provider, repository.NewSuggestionRepository(h.db), editorialapp.GenerateSuggestionOptions{
		SuggestionID: suggestionID, ProviderInstanceID: providerID,
		Article: current, AllowBody: false, TagCandidates: candidates, Taxonomy: taxonomy,
	})
	if err != nil {
		mapError(response, err)
		return
	}
	detail := suggestionDetailView(result, current.ContentHash)
	writeJSON(response, http.StatusOK, detail)
}

// suggestionHistory 返回当前文章的 AI 建议生成历史摘要。
func (h *runtimeHandler) suggestionHistory(response http.ResponseWriter, request *http.Request, articleID string) {
	current, err := h.loadSuggestionArticle(request, articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	limit := 20
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 || parsed > 100 {
			writeError(response, http.StatusBadRequest, "request.invalid", "建议历史分页参数无效")
			return
		}
		limit = parsed
	}
	history, err := repository.NewSuggestionRepository(h.db).ListByArticle(request.Context(), current.WorkspaceID, current.ID, limit)
	if err != nil {
		mapError(response, err)
		return
	}
	items := make([]articleSuggestionHistoryView, 0, len(history))
	for _, version := range history {
		items = append(items, articleSuggestionHistoryView{
			ID: version.ID, GeneratedAt: version.CreatedAt, Model: version.Model,
			InputContentHash: version.InputContentHash, State: string(version.State),
			SuggestionCount: len(version.Items), Current: version.InputContentHash == current.ContentHash,
		})
	}
	result := articleSuggestionHistoryResponse{Items: items}
	if len(items) > 0 {
		result.LatestID = items[0].ID
	}
	writeJSON(response, http.StatusOK, result)
}

// suggestionVersion 返回单个 AI 建议版本的只读详情。
func (h *runtimeHandler) suggestionVersion(response http.ResponseWriter, request *http.Request, articleID, suggestionID string) {
	current, err := h.loadSuggestionArticle(request, articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	version, err := repository.NewSuggestionRepository(h.db).FindByArticleID(request.Context(), current.WorkspaceID, current.ID, suggestionID)
	if errors.Is(err, sql.ErrNoRows) {
		mapError(response, ErrNotFound)
		return
	}
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, suggestionDetailView(version, current.ContentHash))
}

func suggestionDetailView(version editorialapp.SuggestionSet, currentHash string) articleSuggestionDetailView {
	return articleSuggestionDetailView{
		ID: version.ID, GeneratedAt: version.CreatedAt, Model: version.Model,
		InputContentHash: version.InputContentHash, State: string(version.State),
		Suggestions: suggestionViews(version.Items), SuggestionsStale: version.InputContentHash != currentHash,
	}
}

func parseSuggestionPath(path string) (articleID, suggestionID string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/articles/"), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] != "suggestions" {
		return "", "", false
	}
	if len(parts) == 3 && parts[2] == "" {
		return "", "", false
	}
	if len(parts) == 2 {
		return parts[0], "", true
	}
	return parts[0], parts[2], true
}

func parseSuggestionActionPath(path string) (articleID, suggestionID string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/articles/"), "/")
	if len(parts) != 4 || parts[0] == "" || parts[1] != "suggestions" || parts[2] == "" || parts[3] != "actions" {
		return "", "", false
	}
	return parts[0], parts[2], true
}

// newSuggestionID 为每次 AI 生成创建不可预测且不会覆盖历史的建议版本 ID。
func newSuggestionID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("生成 AI 建议版本 ID: %w", err)
	}
	return "suggestion_" + hex.EncodeToString(random[:]), nil
}

func (h *runtimeHandler) loadSuggestionArticle(request *http.Request, id string) (article.Article, error) {
	var current article.Article
	var stableID, tagsJSON, keywordsJSON string
	err := h.db.QueryRowContext(request.Context(), `SELECT id,workspace_id,source_id,stable_id,relative_path,title,description,category,series,tags_json,keywords_json,slug,cover,source_fingerprint,content_hash,frontmatter_hash FROM articles WHERE id=? AND workspace_id=(SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1) AND deleted_at IS NULL`, id).Scan(&current.ID, &current.WorkspaceID, &current.SourceID, &stableID, &current.RelativePath, &current.Title, &current.Description, &current.Category, &current.Series, &tagsJSON, &keywordsJSON, &current.Slug, &current.Cover, &current.SourceFingerprint, &current.ContentHash, &current.FrontmatterHash)
	if err != nil {
		return article.Article{}, ErrNotFound
	}
	current.StableID = article.StableID(stableID)
	_ = json.Unmarshal([]byte(tagsJSON), &current.Tags)
	_ = json.Unmarshal([]byte(keywordsJSON), &current.Keywords)
	return current, nil
}

func (h *runtimeHandler) loadSuggestionTaxonomy(request *http.Request, workspaceID string) (contracts.TaxonomyContext, []editorialapp.TagCandidate, error) {
	rows, err := h.db.QueryContext(request.Context(), `SELECT kind,name,usage_count FROM taxonomy_terms WHERE workspace_id=? ORDER BY usage_count DESC,name`, workspaceID)
	if err != nil {
		return contracts.TaxonomyContext{}, nil, err
	}
	defer rows.Close()
	var taxonomy contracts.TaxonomyContext
	var candidates []editorialapp.TagCandidate
	for rows.Next() {
		var kind, name string
		var usage int
		if err := rows.Scan(&kind, &name, &usage); err != nil {
			return contracts.TaxonomyContext{}, nil, err
		}
		switch kind {
		case "category":
			taxonomy.Categories = append(taxonomy.Categories, name)
		case "series":
			taxonomy.Series = append(taxonomy.Series, name)
		case "tag":
			taxonomy.Tags = append(taxonomy.Tags, name)
			candidates = append(candidates, editorialapp.TagCandidate{Name: name, UsageCount: usage})
		}
	}
	return taxonomy, candidates, rows.Err()
}

func suggestionViews(items []editorialapp.SuggestionItem) []articleSuggestionView {
	views := make([]articleSuggestionView, 0, len(items))
	for _, item := range items {
		var name string
		if json.Unmarshal(item.Value, &name) == nil {
			views = append(views, articleSuggestionView{ID: item.ID, Field: item.Field, Name: name, Value: name, Reason: item.Rationale, NewTerm: item.NewTerm, UsageCount: item.UsageCount, Accepted: item.Accepted, Ignored: item.Ignored, Status: item.Status()})
			continue
		}
		var values []string
		if json.Unmarshal(item.Value, &values) != nil {
			continue
		}
		views = append(views, articleSuggestionView{ID: item.ID, Field: item.Field, Name: strings.Join(values, "、"), Value: values, Reason: item.Rationale, NewTerm: item.NewTerm, UsageCount: item.UsageCount, Accepted: item.Accepted, Ignored: item.Ignored, Status: item.Status()})
	}
	return views
}

func pendingSuggestionItems(items []editorialapp.SuggestionItem) []editorialapp.SuggestionItem {
	pending := make([]editorialapp.SuggestionItem, 0, len(items))
	for _, item := range items {
		if item.Status() == "pending" {
			pending = append(pending, item)
		}
	}
	return pending
}
