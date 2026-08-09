package editorial

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const publicationSettingsKey = "publication_content"

// PublicationSettings 保存同一工作区所有发布渠道共享的内容转换规则。
type PublicationSettings struct {
	ExcludedSections []string `json:"excluded_sections"`
}

// LoadPublicationSettings 读取工作区发布内容规则；旧工作区默认不排除任何章节。
func LoadPublicationSettings(ctx context.Context, db *sql.DB, workspaceID string) (PublicationSettings, error) {
	settings := PublicationSettings{ExcludedSections: []string{}}
	var valueJSON string
	err := db.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE workspace_id=? AND key=?`, workspaceID, publicationSettingsKey).Scan(&valueJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return PublicationSettings{}, err
	}
	if err := json.Unmarshal([]byte(valueJSON), &settings); err != nil {
		return PublicationSettings{}, fmt.Errorf("解析发布内容设置: %w", err)
	}
	sections, err := NormalizeExcludedSectionTitles(settings.ExcludedSections)
	if err != nil {
		return PublicationSettings{}, err
	}
	settings.ExcludedSections = sections
	return settings, nil
}

// SavePublicationSettings 保存经过规范化的工作区发布内容规则。
func SavePublicationSettings(ctx context.Context, db *sql.DB, workspaceID string, settings PublicationSettings) (PublicationSettings, error) {
	sections, err := NormalizeExcludedSectionTitles(settings.ExcludedSections)
	if err != nil {
		return PublicationSettings{}, err
	}
	settings.ExcludedSections = sections
	encoded, err := json.Marshal(settings)
	if err != nil {
		return PublicationSettings{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `INSERT INTO settings(workspace_id,key,value_json,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(workspace_id,key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, workspaceID, publicationSettingsKey, string(encoded), now, now)
	return settings, err
}

// NormalizeExcludedSectionTitles 清理空标题和重复项，并限制设置规模。
func NormalizeExcludedSectionTitles(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if len([]rune(value)) > 100 {
			return nil, fmt.Errorf("排除章节标题不能超过 100 个字符")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) > 50 {
			return nil, fmt.Errorf("最多配置 50 个排除章节")
		}
	}
	return result, nil
}
