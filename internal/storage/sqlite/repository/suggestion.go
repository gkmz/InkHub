package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	domaineditorial "github.com/gkmz/InkHub/internal/domain/editorial"
)

// SuggestionRepository 保存和查询 AI 建议及其字段级处理状态。
type SuggestionRepository struct {
	db *sql.DB
}

// NewSuggestionRepository 创建 AI 建议 Repository。
func NewSuggestionRepository(db *sql.DB) *SuggestionRepository {
	return &SuggestionRepository{db: db}
}

type suggestionPayload struct {
	Model string                           `json:"model"`
	Items []domaineditorial.SuggestionItem `json:"items"`
}

// Save 新增建议或更新同一建议的处理状态。
func (r *SuggestionRepository) Save(ctx context.Context, value domaineditorial.SuggestionSet) error {
	if value.ID == "" || value.ArticleID == "" || value.WorkspaceID == "" || value.ProviderInstanceID == "" {
		return fmt.Errorf("AI 建议身份字段不完整")
	}
	payload, err := json.Marshal(suggestionPayload{Model: value.Model, Items: value.Items})
	if err != nil {
		return fmt.Errorf("序列化 AI 建议: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `INSERT INTO ai_suggestions(
id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET suggestion_json=excluded.suggestion_json,state=excluded.state,updated_at=excluded.updated_at
WHERE ai_suggestions.article_id=excluded.article_id
  AND ai_suggestions.provider_instance_id=excluded.provider_instance_id
  AND ai_suggestions.workspace_id=excluded.workspace_id
  AND ai_suggestions.input_content_hash=excluded.input_content_hash`,
		value.ID, value.ArticleID, value.InputContentHash, value.ProviderInstanceID, value.WorkspaceID,
		string(payload), value.State, now, now)
	if err != nil {
		return fmt.Errorf("保存 AI 建议: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("检查 AI 建议保存结果: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("AI 建议身份与已有记录冲突: %s", value.ID)
	}
	return nil
}

// FindByID 按建议 ID 查询完整建议集合。
func (r *SuggestionRepository) FindByID(ctx context.Context, id string) (domaineditorial.SuggestionSet, error) {
	var value domaineditorial.SuggestionSet
	var payloadJSON string
	err := r.db.QueryRowContext(ctx, `SELECT id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state
FROM ai_suggestions WHERE id=?`, id).Scan(
		&value.ID, &value.ArticleID, &value.InputContentHash, &value.ProviderInstanceID,
		&value.WorkspaceID, &payloadJSON, &value.State,
	)
	if err != nil {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("查询 AI 建议: %w", err)
	}
	var payload suggestionPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("解析 AI 建议: %w", err)
	}
	value.Model = payload.Model
	value.Items = payload.Items
	return value, nil
}
