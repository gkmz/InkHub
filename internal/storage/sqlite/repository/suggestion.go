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
	return scanSuggestion(r.db.QueryRowContext(ctx, `SELECT id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state,created_at,updated_at FROM ai_suggestions WHERE id=?`, id))
}

// ListByArticle 查询指定工作区文章的建议历史，最新生成的版本排在最前面。
func (r *SuggestionRepository) ListByArticle(ctx context.Context, workspaceID, articleID string, limit int) ([]domaineditorial.SuggestionSet, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state,created_at,updated_at
FROM ai_suggestions
WHERE workspace_id=? AND article_id=?
ORDER BY created_at DESC,id DESC LIMIT ?`, workspaceID, articleID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询 AI 建议历史: %w", err)
	}
	defer rows.Close()
	items := make([]domaineditorial.SuggestionSet, 0, limit)
	for rows.Next() {
		item, scanErr := scanSuggestion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 AI 建议历史: %w", err)
	}
	return items, nil
}

// FindByArticleID 在工作区和文章边界内查询指定的建议版本。
func (r *SuggestionRepository) FindByArticleID(ctx context.Context, workspaceID, articleID, suggestionID string) (domaineditorial.SuggestionSet, error) {
	return scanSuggestion(r.db.QueryRowContext(ctx, `SELECT id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state,created_at,updated_at
FROM ai_suggestions WHERE workspace_id=? AND article_id=? AND id=?`, workspaceID, articleID, suggestionID))
}

// FindLatestByArticle 查询当前工作区文章最近更新的一组 AI 建议。
func (r *SuggestionRepository) FindLatestByArticle(ctx context.Context, workspaceID, articleID string) (domaineditorial.SuggestionSet, bool, error) {
	value, err := scanSuggestion(r.db.QueryRowContext(ctx, `SELECT id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state,created_at,updated_at FROM ai_suggestions WHERE workspace_id=? AND article_id=? ORDER BY updated_at DESC,id DESC LIMIT 1`, workspaceID, articleID))
	if err == sql.ErrNoRows {
		return domaineditorial.SuggestionSet{}, false, nil
	}
	return value, err == nil, err
}

// UpdateItemStates 在指定建议版本内原子更新一批建议项的采用或忽略状态。
func (r *SuggestionRepository) UpdateItemStates(ctx context.Context, workspaceID, articleID, suggestionID, action string, itemIDs []string) (domaineditorial.SuggestionSet, error) {
	if workspaceID == "" || articleID == "" || suggestionID == "" {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("AI 建议身份字段不完整")
	}
	if action != "accepted" && action != "ignored" {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("不支持的 AI 建议操作: %s", action)
	}
	if len(itemIDs) == 0 {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("至少需要一个 AI 建议项")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("开启 AI 建议状态事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	value, err := scanSuggestion(tx.QueryRowContext(ctx, `SELECT id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state,created_at,updated_at
FROM ai_suggestions WHERE workspace_id=? AND article_id=? AND id=?`, workspaceID, articleID, suggestionID))
	if err != nil {
		return domaineditorial.SuggestionSet{}, err
	}
	requested := make(map[string]struct{}, len(itemIDs))
	for _, id := range itemIDs {
		if id == "" {
			return domaineditorial.SuggestionSet{}, fmt.Errorf("AI 建议项 ID 不能为空")
		}
		if _, exists := requested[id]; exists {
			continue
		}
		requested[id] = struct{}{}
	}
	updated := 0
	for index := range value.Items {
		if _, exists := requested[value.Items[index].ID]; !exists {
			continue
		}
		if value.Items[index].Accepted || value.Items[index].Ignored {
			return domaineditorial.SuggestionSet{}, fmt.Errorf("AI 建议项已经处理: %s", value.Items[index].ID)
		}
		if action == "accepted" {
			value.Items[index].Accepted = true
		} else {
			value.Items[index].Ignored = true
		}
		updated++
	}
	if updated != len(requested) {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("找不到 AI 建议项")
	}
	value.State = domaineditorial.DeriveSuggestionState(value.Items)
	payload, err := json.Marshal(suggestionPayload{Model: value.Model, Items: value.Items})
	if err != nil {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("序列化 AI 建议状态: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE ai_suggestions SET suggestion_json=?,state=?,updated_at=? WHERE id=? AND article_id=? AND workspace_id=?`, string(payload), value.State, now, suggestionID, articleID, workspaceID)
	if err != nil {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("更新 AI 建议状态: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("更新 AI 建议状态失败")
	}
	if err := tx.Commit(); err != nil {
		return domaineditorial.SuggestionSet{}, fmt.Errorf("提交 AI 建议状态: %w", err)
	}
	value.UpdatedAt = now
	return value, nil
}

func scanSuggestion(row rowScanner) (domaineditorial.SuggestionSet, error) {
	var value domaineditorial.SuggestionSet
	var payloadJSON string
	err := row.Scan(
		&value.ID, &value.ArticleID, &value.InputContentHash, &value.ProviderInstanceID,
		&value.WorkspaceID, &payloadJSON, &value.State, &value.CreatedAt, &value.UpdatedAt,
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
