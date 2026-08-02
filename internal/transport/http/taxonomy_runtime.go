package httptransport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	appTaxonomy "github.com/gkmz/InkHub/internal/app/taxonomy"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

type taxonomyTermView struct {
	Kind       string            `json:"kind"`
	Key        string            `json:"key"`
	Name       string            `json:"name"`
	UsageCount int               `json:"usage_count"`
	Metadata   map[string]string `json:"metadata"`
}

type taxonomyTermCommandRequest struct {
	ProviderID       string   `json:"provider_id"`
	Kind             string   `json:"kind"`
	Key              string   `json:"key"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Aliases          []string `json:"aliases"`
	ExpectedRevision string   `json:"expected_revision"`
}

type taxonomyFileChangeView struct {
	RelativePath string `json:"relative_path"`
	Before       string `json:"before"`
	After        string `json:"after"`
}

type taxonomyOverviewView struct {
	Source       string             `json:"source"`
	ProviderID   string             `json:"provider_id,omitempty"`
	ProviderType string             `json:"provider_type,omitempty"`
	State        string             `json:"state"`
	Revision     string             `json:"revision,omitempty"`
	LoadedAt     string             `json:"loaded_at"`
	AttemptedAt  string             `json:"attempted_at,omitempty"`
	Readonly     bool               `json:"readonly"`
	Error        string             `json:"error,omitempty"`
	ErrorCode    string             `json:"error_code,omitempty"`
	Terms        []taxonomyTermView `json:"terms"`
	Issues       []any              `json:"issues"`
}

func (h *runtimeHandler) taxonomyOverview(response http.ResponseWriter, request *http.Request) {
	view, err := h.loadTaxonomyOverview(request)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, view)
}

func (h *runtimeHandler) refreshTaxonomyOverview(response http.ResponseWriter, request *http.Request) {
	if h.refreshTaxonomy == nil {
		writeError(response, http.StatusConflict, "taxonomy.not_configured", "当前没有可刷新的类目来源")
		return
	}
	if err := h.refreshTaxonomy(request.Context()); err != nil {
		logHTTPError(response, err, http.StatusUnprocessableEntity, "taxonomy.refresh_failed")
		writeError(response, http.StatusUnprocessableEntity, "taxonomy.refresh_failed", "类目刷新失败，请检查 Hugo 配置")
		return
	}
	h.taxonomyOverview(response, request)
}

func (h *runtimeHandler) previewTaxonomyTerm(response http.ResponseWriter, request *http.Request) {
	input, command, ok := decodeTaxonomyTermCommand(response, request)
	if !ok {
		return
	}
	_, ref, provider, err := h.configuredTaxonomyProvider(request.Context(), input.ProviderID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			mapError(response, ErrNotFound)
		} else {
			writeTaxonomyError(response, err)
		}
		return
	}
	change, err := appTaxonomy.NewService(repository.NewTaxonomyRepository(h.db), time.Now).PlanChange(request.Context(), ref, provider, command)
	if err != nil {
		writeTaxonomyError(response, err)
		return
	}
	files := make([]taxonomyFileChangeView, 0, len(change.Files))
	for _, file := range change.Files {
		files = append(files, taxonomyFileChangeView{RelativePath: file.RelativePath, Before: file.Before, After: file.After})
	}
	writeJSON(response, http.StatusOK, map[string]any{"provider_id": ref.ID, "expected_revision": change.ExpectedRevision, "files": files})
}

func (h *runtimeHandler) applyTaxonomyTerm(response http.ResponseWriter, request *http.Request) {
	input, command, ok := decodeTaxonomyTermCommand(response, request)
	if !ok {
		return
	}
	workspaceID, ref, provider, err := h.configuredTaxonomyProvider(request.Context(), input.ProviderID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			mapError(response, ErrNotFound)
		} else {
			writeTaxonomyError(response, err)
		}
		return
	}
	if _, err := appTaxonomy.NewService(repository.NewTaxonomyRepository(h.db), time.Now).ApplyChange(request.Context(), workspaceID, ref, provider, command); err != nil {
		writeTaxonomyError(response, err)
		return
	}
	h.taxonomyOverview(response, request)
}

func decodeTaxonomyTermCommand(response http.ResponseWriter, request *http.Request) (taxonomyTermCommandRequest, contracts.TaxonomyCommand, bool) {
	var input taxonomyTermCommandRequest
	if decodeJSON(request, &input) != nil || input.ProviderID == "" || input.Kind == "" || input.Name == "" || input.ExpectedRevision == "" || len(input.Name) > 160 || len(input.Key) > 160 || len(input.Description) > 1000 || len(input.Aliases) > 20 {
		writeError(response, http.StatusBadRequest, "taxonomy.command_invalid", "类目变更内容无效")
		return input, contracts.TaxonomyCommand{}, false
	}
	for index, alias := range input.Aliases {
		input.Aliases[index] = strings.TrimSpace(alias)
		if input.Aliases[index] == "" || len(input.Aliases[index]) > 160 || strings.ContainsAny(alias, "\r\n") {
			writeError(response, http.StatusBadRequest, "taxonomy.command_invalid", "类目别名无效")
			return input, contracts.TaxonomyCommand{}, false
		}
	}
	input.Key = strings.TrimSpace(input.Key)
	if input.Key == "" {
		input.Key = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(input.Name), " ", "-"))
	}
	metadata := map[string]string{}
	if description := strings.TrimSpace(input.Description); description != "" {
		metadata["description"] = description
	}
	if len(input.Aliases) > 0 {
		metadata["aliases"] = strings.Join(input.Aliases, "\n")
	}
	command := contracts.TaxonomyCommand{Kind: contracts.TaxonomyCreateTerm, ExpectedRevision: input.ExpectedRevision, Term: contracts.TaxonomyTerm{Kind: input.Kind, Key: input.Key, Name: strings.TrimSpace(input.Name), CanonicalName: strings.TrimSpace(input.Name), Metadata: metadata}}
	return input, command, true
}

func (h *runtimeHandler) configuredTaxonomyProvider(ctx context.Context, providerID string) (string, contracts.ProviderRef, contracts.TaxonomyProvider, error) {
	if h.providerRuntime == nil {
		return "", contracts.ProviderRef{}, nil, ErrNotFound
	}
	var workspaceID, providerType, configJSON string
	err := h.db.QueryRowContext(ctx, `SELECT provider_instances.workspace_id,provider_instances.provider_type,provider_instances.config_json FROM provider_instances JOIN workspaces ON workspaces.id=provider_instances.workspace_id WHERE provider_instances.id=? AND provider_instances.enabled=1 AND workspaces.id=(SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1)`, providerID).Scan(&workspaceID, &providerType, &configJSON)
	if err != nil || !h.providerRuntime.SupportsTaxonomy(contracts.ProviderType(providerType)) {
		return "", contracts.ProviderRef{}, nil, ErrNotFound
	}
	view, err := taxonomyProviderConfigView([]byte(configJSON))
	if err != nil {
		return "", contracts.ProviderRef{}, nil, err
	}
	ref := contracts.ProviderRef{ID: providerID, Type: contracts.ProviderType(providerType)}
	provider, err := h.providerRuntime.BuildTaxonomy(ctx, ref, view)
	return workspaceID, ref, provider, err
}

func writeTaxonomyError(response http.ResponseWriter, err error) {
	var providerErr *contracts.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Category == contracts.ErrorConflict {
			logHTTPError(response, err, http.StatusConflict, providerErr.Code)
			writeError(response, http.StatusConflict, providerErr.Code, providerErr.Message)
			return
		}
		logHTTPError(response, err, http.StatusUnprocessableEntity, providerErr.Code)
		writeError(response, http.StatusUnprocessableEntity, providerErr.Code, providerErr.Message)
		return
	}
	logHTTPError(response, err, http.StatusUnprocessableEntity, "taxonomy.change_failed")
	writeError(response, http.StatusUnprocessableEntity, "taxonomy.change_failed", "类目变更失败，请检查 Hugo 配置")
}

func (h *runtimeHandler) loadTaxonomyOverview(request *http.Request) (taxonomyOverviewView, error) {
	empty := taxonomyOverviewView{Source: "尚未配置", State: "not_enabled", LoadedAt: "-", Readonly: true, Terms: []taxonomyTermView{}, Issues: []any{}}
	if h.providerRuntime == nil {
		return empty, nil
	}
	var workspaceID string
	if err := h.db.QueryRowContext(request.Context(), `SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1`).Scan(&workspaceID); err == sql.ErrNoRows {
		return empty, nil
	} else if err != nil {
		return empty, err
	}
	rows, err := h.db.QueryContext(request.Context(), `SELECT id,provider_type,name,config_json FROM provider_instances WHERE workspace_id=? AND enabled=1 ORDER BY id`, workspaceID)
	if err != nil {
		return empty, err
	}
	defer rows.Close()
	var providerID, providerType, sourceName, configJSON string
	for rows.Next() {
		var id, typeName, name, config string
		if err := rows.Scan(&id, &typeName, &name, &config); err != nil {
			return empty, err
		}
		if h.providerRuntime.SupportsTaxonomy(contracts.ProviderType(typeName)) {
			providerID, providerType, sourceName, configJSON = id, typeName, name, config
			break
		}
	}
	if err := rows.Err(); err != nil {
		return empty, err
	}
	if providerID == "" {
		return empty, nil
	}
	empty.Source, empty.ProviderID, empty.ProviderType, empty.State = sourceName, providerID, providerType, "not_loaded"
	configView, configErr := taxonomyProviderConfigView([]byte(configJSON))
	if configErr == nil {
		if provider, buildErr := h.providerRuntime.BuildTaxonomy(request.Context(), contracts.ProviderRef{ID: providerID, Type: contracts.ProviderType(providerType)}, configView); buildErr == nil {
			empty.Readonly = !provider.Descriptor().Writable
		}
	}
	snapshot, status, err := repository.NewTaxonomyRepository(h.db).GetSnapshot(request.Context(), workspaceID, providerID)
	if err == sql.ErrNoRows {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	empty.State = status.State
	empty.Revision = snapshot.Revision
	empty.AttemptedAt = formatTaxonomyTime(status.LastAttemptAt)
	if status.LastSuccessAt != nil {
		empty.LoadedAt = formatTaxonomyTime(*status.LastSuccessAt)
	}
	empty.Error = status.LastErrorMessage
	empty.ErrorCode = status.LastErrorCode
	for _, term := range snapshot.Terms {
		if term.Metadata == nil {
			term.Metadata = map[string]string{}
		}
		empty.Terms = append(empty.Terms, taxonomyTermView{Kind: term.Kind, Key: term.Key, Name: term.Name, UsageCount: term.UsageCount, Metadata: term.Metadata})
	}
	return empty, nil
}

func taxonomyProviderConfigView(data []byte) (contracts.ConfigView, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return contracts.ConfigView{}, err
	}
	var roots []string
	for key, value := range raw {
		if key != "root" && !strings.HasSuffix(key, "_root") {
			continue
		}
		if path, ok := value.(string); ok && path != "" {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return contracts.ConfigView{}, err
			}
			roots = append(roots, absolute)
		}
	}
	return contracts.ConfigView{Data: data, AllowedRoots: roots}, nil
}

func formatTaxonomyTime(value time.Time) string {
	return value.Format("2006-01-02 15:04")
}
