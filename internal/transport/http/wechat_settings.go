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
)

type wechatSettingsRequest struct {
	Enabled          bool   `json:"enabled"`
	Template         string `json:"template"`
	GitHubOwner      string `json:"github_owner"`
	GitHubRepository string `json:"github_repository"`
	GitHubBranch     string `json:"github_branch"`
	GitHubPrefix     string `json:"github_prefix"`
	GitHubToken      string `json:"github_token"`
}

type storedWeChatConfig struct {
	StagingRoot      string `json:"staging_root"`
	Template         string `json:"template"`
	GitHubOwner      string `json:"github_owner,omitempty"`
	GitHubRepository string `json:"github_repository,omitempty"`
	GitHubBranch     string `json:"github_branch,omitempty"`
	GitHubPrefix     string `json:"github_prefix,omitempty"`
	GitHubSecretRef  string `json:"github_secret_ref,omitempty"`
}

// saveWeChatSettings 保存微信非敏感配置，并把 GitHub Token 交给系统 Secret Store。
func (h *runtimeHandler) saveWeChatSettings(response http.ResponseWriter, request *http.Request) {
	var input wechatSettingsRequest
	if decodeJSON(request, &input) != nil {
		writeError(response, http.StatusBadRequest, "wechat.config_invalid", "微信发布配置无效")
		return
	}
	// 微信当前只有墨绿色模板，忽略旧客户端提交的历史模板值。
	input.Template = "default"
	input.GitHubOwner = strings.TrimSpace(input.GitHubOwner)
	input.GitHubRepository = strings.TrimSpace(input.GitHubRepository)
	input.GitHubBranch = strings.TrimSpace(input.GitHubBranch)
	input.GitHubPrefix = strings.Trim(strings.TrimSpace(input.GitHubPrefix), "/")
	if input.GitHubBranch == "" {
		input.GitHubBranch = "main"
	}
	if input.GitHubPrefix == "" {
		input.GitHubPrefix = "inkhub"
	}
	if (input.GitHubOwner == "") != (input.GitHubRepository == "") {
		writeError(response, http.StatusBadRequest, "wechat.config_invalid", "GitHub Owner 和仓库必须同时填写")
		return
	}
	var workspaceID string
	if err := h.db.QueryRowContext(request.Context(), `SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1`).Scan(&workspaceID); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	providerID := stableRuntimeID("wechat", workspaceID)
	secretRef := providerID + "-github-token"
	if input.GitHubOwner != "" && input.GitHubToken == "" && !h.hasSecret(request.Context(), secretRef) {
		writeError(response, http.StatusBadRequest, "wechat.secret_required", "请填写 GitHub Token")
		return
	}
	if input.GitHubToken != "" {
		var secretErr error
		if h.aiSecrets == nil {
			secretErr = errors.New("Secret Store 不可用")
		} else {
			secretErr = h.aiSecrets.Set(request.Context(), secretRef, input.GitHubToken)
		}
		if secretErr != nil {
			logHTTPError(response, secretErr, http.StatusInternalServerError, "wechat.secret_save_failed")
			writeError(response, http.StatusInternalServerError, "wechat.secret_save_failed", "GitHub Token 保存失败")
			return
		}
	}
	config := storedWeChatConfig{
		StagingRoot: filepath.Join(h.dataDir, "wechat"), Template: input.Template,
		GitHubOwner: input.GitHubOwner, GitHubRepository: input.GitHubRepository,
		GitHubBranch: input.GitHubBranch, GitHubPrefix: input.GitHubPrefix,
	}
	if input.GitHubOwner != "" {
		config.GitHubSecretRef = secretRef
	}
	encoded, _ := json.Marshal(config)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := h.db.ExecContext(request.Context(), `INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at) VALUES(?,?,'wechat','微信公众号',?,?,?,?) ON CONFLICT(workspace_id,provider_type) DO UPDATE SET enabled=excluded.enabled,config_json=excluded.config_json,updated_at=excluded.updated_at`, providerID, workspaceID, input.Enabled, string(encoded), now, now)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"wechat_enabled": input.Enabled, "default_template": input.Template,
		"github_owner": input.GitHubOwner, "github_repository": input.GitHubRepository,
		"github_branch": input.GitHubBranch, "github_prefix": input.GitHubPrefix,
		"github_token_saved": input.GitHubOwner != "" && h.hasSecret(request.Context(), secretRef),
	})
}

func (h *runtimeHandler) hasSecret(ctx context.Context, ref string) bool {
	if h.aiSecrets == nil || ref == "" {
		return false
	}
	value, err := h.aiSecrets.Get(ctx, ref)
	return err == nil && value != ""
}

func loadStoredWeChatConfig(ctx context.Context, db *sql.DB, workspaceID string) (storedWeChatConfig, bool, bool) {
	var raw string
	var enabled bool
	if err := db.QueryRowContext(ctx, `SELECT config_json,enabled FROM provider_instances WHERE workspace_id=? AND provider_type='wechat'`, workspaceID).Scan(&raw, &enabled); err != nil {
		return storedWeChatConfig{}, false, false
	}
	var config storedWeChatConfig
	if json.Unmarshal([]byte(raw), &config) != nil {
		return storedWeChatConfig{}, false, false
	}
	return config, enabled, true
}
