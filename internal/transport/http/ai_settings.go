package httptransport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// AISecretStore 是 HTTP 设置层写入系统安全存储所需的最小接口。
type AISecretStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

type aiSettingsRequest struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

type storedAIConfig struct {
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	Timeout   int64  `json:"timeout"`
	SecretRef string `json:"secret_ref"`
}

// saveAISettings 保存非敏感 Provider 配置，并把 API Key 交给系统 Secret Store。
func (h *runtimeHandler) saveAISettings(response http.ResponseWriter, request *http.Request) {
	var input aiSettingsRequest
	if decodeJSON(request, &input) != nil || (input.Enabled && (strings.TrimSpace(input.BaseURL) == "" || strings.TrimSpace(input.Model) == "")) {
		writeError(response, http.StatusBadRequest, "ai.config_invalid", "AI 配置不完整")
		return
	}
	var workspaceID string
	if err := h.db.QueryRowContext(request.Context(), `SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1`).Scan(&workspaceID); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	providerID := stableRuntimeID("ai", workspaceID)
	secretRef := providerID + "-api-key"
	if input.Enabled && input.APIKey == "" {
		if !h.hasAISecret(request.Context(), secretRef) {
			writeError(response, http.StatusBadRequest, "ai.secret_required", "请填写 AI API Key")
			return
		}
	}
	if input.APIKey != "" {
		var secretErr error
		if h.aiSecrets == nil {
			secretErr = errors.New("AI Secret Store 不可用")
		} else {
			secretErr = h.aiSecrets.Set(request.Context(), secretRef, input.APIKey)
		}
		if secretErr != nil {
			logHTTPError(response, secretErr, http.StatusInternalServerError, "ai.secret_save_failed")
			writeError(response, http.StatusInternalServerError, "ai.secret_save_failed", "AI API Key 保存失败")
			return
		}
	}
	config, _ := json.Marshal(storedAIConfig{BaseURL: strings.TrimSpace(input.BaseURL), Model: strings.TrimSpace(input.Model), Timeout: int64(30 * time.Second), SecretRef: secretRef})
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := h.db.ExecContext(request.Context(), `INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at) VALUES(?,?,'openai-compatible','AI 建议',?,?,?,?) ON CONFLICT(workspace_id,provider_type) DO UPDATE SET enabled=excluded.enabled,config_json=excluded.config_json,updated_at=excluded.updated_at`, providerID, workspaceID, input.Enabled, string(config), now, now)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ai_enabled": input.Enabled, "ai_secret_saved": input.APIKey != "" || h.hasAISecret(request.Context(), secretRef)})
}

func (h *runtimeHandler) hasAISecret(ctx context.Context, ref string) bool {
	if h.aiSecrets == nil || ref == "" {
		return false
	}
	value, err := h.aiSecrets.Get(ctx, ref)
	return err == nil && value != ""
}

func loadStoredAIConfig(ctx context.Context, db *sql.DB, workspaceID string) (storedAIConfig, bool, bool) {
	var raw string
	var enabled bool
	if err := db.QueryRowContext(ctx, `SELECT config_json,enabled FROM provider_instances WHERE workspace_id=? AND provider_type='openai-compatible'`, workspaceID).Scan(&raw, &enabled); err != nil {
		return storedAIConfig{}, false, false
	}
	var config storedAIConfig
	if json.Unmarshal([]byte(raw), &config) != nil {
		return storedAIConfig{}, false, false
	}
	return config, enabled, true
}
