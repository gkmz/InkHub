package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
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
	Reason     string `json:"reason"`
	NewTerm    bool   `json:"new_term"`
	UsageCount int    `json:"usage_count"`
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
	writeJSON(response, http.StatusOK, map[string]any{"suggestions": suggestionViews(result.Items), "suggestions_stale": false})
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
		if json.Unmarshal(item.Value, &name) != nil {
			continue
		}
		views = append(views, articleSuggestionView{ID: item.ID, Field: item.Field, Name: name, Reason: item.Rationale, NewTerm: item.NewTerm, UsageCount: item.UsageCount})
	}
	return views
}
