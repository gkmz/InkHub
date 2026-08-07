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

const (
	xiaohongshuSettingsKey       = "xiaohongshu"
	xiaohongshuDefaultTemplateID = "mobile-clean"
)

type storedXiaohongshuSettings struct {
	Enabled    bool   `json:"enabled"`
	TemplateID string `json:"template_id"`
}

type xiaohongshuSettingsRequest struct {
	Enabled  bool   `json:"enabled"`
	Template string `json:"template"`
}

// xiaohongshuTemplateSummaries 返回前后端共同支持的小红书模板清单。
func xiaohongshuTemplateSummaries() []map[string]any {
	return []map[string]any{{"id": xiaohongshuDefaultTemplateID, "name": "中文悦读", "version": "1", "compatible": true}}
}

// defaultXiaohongshuSettings 保持升级前默认可用，并指定当前唯一模板。
func defaultXiaohongshuSettings() storedXiaohongshuSettings {
	return storedXiaohongshuSettings{Enabled: true, TemplateID: xiaohongshuDefaultTemplateID}
}

// loadStoredXiaohongshuSettings 读取工作区配置；旧工作区没有记录时沿用原有启用行为。
func loadStoredXiaohongshuSettings(ctx context.Context, db *sql.DB, workspaceID string) (storedXiaohongshuSettings, error) {
	settings := defaultXiaohongshuSettings()
	var valueJSON string
	err := db.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE workspace_id=? AND key=?`, workspaceID, xiaohongshuSettingsKey).Scan(&valueJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return storedXiaohongshuSettings{}, err
	}
	if err := json.Unmarshal([]byte(valueJSON), &settings); err != nil {
		return storedXiaohongshuSettings{}, err
	}
	if strings.TrimSpace(settings.TemplateID) == "" {
		settings.TemplateID = xiaohongshuDefaultTemplateID
	}
	return settings, nil
}

// validXiaohongshuTemplate 判断模板是否属于当前可用注册表。
func validXiaohongshuTemplate(templateID string) bool {
	return templateID == xiaohongshuDefaultTemplateID
}

// saveXiaohongshuSettings 保存当前工作区的小红书启用状态和默认模板。
func (h *runtimeHandler) saveXiaohongshuSettings(response http.ResponseWriter, request *http.Request) {
	var input xiaohongshuSettingsRequest
	if decodeJSON(request, &input) != nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书设置请求无效")
		return
	}
	input.Template = strings.TrimSpace(input.Template)
	if !validXiaohongshuTemplate(input.Template) {
		writeError(response, http.StatusBadRequest, "xiaohongshu.template_invalid", "请选择可用的小红书模板")
		return
	}
	workspaceID, err := h.currentWorkspaceID(request.Context())
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	encoded, _ := json.Marshal(storedXiaohongshuSettings{Enabled: input.Enabled, TemplateID: input.Template})
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := h.db.ExecContext(request.Context(), `INSERT INTO settings(workspace_id,key,value_json,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(workspace_id,key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, workspaceID, xiaohongshuSettingsKey, string(encoded), now, now); err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"xiaohongshu_enabled": input.Enabled, "xiaohongshu_template": input.Template})
}
