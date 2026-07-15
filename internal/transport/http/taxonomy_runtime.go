package httptransport

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"

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
		writeError(response, http.StatusUnprocessableEntity, "taxonomy.refresh_failed", "类目刷新失败，请检查 Hugo 配置")
		return
	}
	h.taxonomyOverview(response, request)
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
