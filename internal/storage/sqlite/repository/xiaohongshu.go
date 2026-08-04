package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	domain "github.com/gkmz/InkHub/internal/domain/xiaohongshu"
)

// XiaohongshuRepository 持久化小红书草稿、图片渲染版本和审计事件。
type XiaohongshuRepository struct{ db *sql.DB }

// NewXiaohongshuRepository 创建小红书 Repository。
func NewXiaohongshuRepository(db *sql.DB) *XiaohongshuRepository {
	return &XiaohongshuRepository{db: db}
}

// SaveDraft 新增一个草稿版本；同一个 ID 只允许幂等更新同一版本。
func (r *XiaohongshuRepository) SaveDraft(ctx context.Context, draft domain.Draft) error {
	if r == nil || r.db == nil || draft.ID == "" || draft.ArticleID == "" || draft.WorkspaceID == "" || draft.SourceContentHash == "" {
		return fmt.Errorf("小红书草稿身份字段不完整")
	}
	topics, err := json.Marshal(draft.Topics)
	if err != nil {
		return fmt.Errorf("序列化小红书话题: %w", err)
	}
	pages, err := json.Marshal(draft.Pages)
	if err != nil {
		return fmt.Errorf("序列化小红书页面: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = r.db.ExecContext(ctx, `INSERT INTO xiaohongshu_drafts(
id,article_id,workspace_id,source_content_hash,title,body_html,pages_json,topics_json,source_note,comment_copy,ai_model,prompt_version,state,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET title=excluded.title,body_html=excluded.body_html,pages_json=excluded.pages_json,topics_json=excluded.topics_json,
source_note=excluded.source_note,comment_copy=excluded.comment_copy,state=excluded.state,updated_at=excluded.updated_at
WHERE xiaohongshu_drafts.article_id=excluded.article_id AND xiaohongshu_drafts.workspace_id=excluded.workspace_id`,
		draft.ID, draft.ArticleID, draft.WorkspaceID, draft.SourceContentHash, draft.Title, draft.BodyHTML, string(pages), string(topics), draft.SourceNote, draft.CommentCopy, draft.AIModel, draft.PromptVersion, draft.State, now, now)
	if err != nil {
		return fmt.Errorf("保存小红书草稿: %w", err)
	}
	return nil
}

// FindDraft 在文章和工作区边界内读取一个草稿版本。
func (r *XiaohongshuRepository) FindDraft(ctx context.Context, workspaceID, articleID, id string) (domain.Draft, error) {
	return scanDraft(r.db.QueryRowContext(ctx, `SELECT id,article_id,workspace_id,source_content_hash,title,body_html,pages_json,topics_json,source_note,comment_copy,ai_model,prompt_version,state,created_at,updated_at FROM xiaohongshu_drafts WHERE workspace_id=? AND article_id=? AND id=?`, workspaceID, articleID, id))
}

// ListDrafts 返回文章草稿历史，按生成时间倒序排列。
func (r *XiaohongshuRepository) ListDrafts(ctx context.Context, workspaceID, articleID string, limit int) ([]domain.Draft, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,article_id,workspace_id,source_content_hash,title,body_html,pages_json,topics_json,source_note,comment_copy,ai_model,prompt_version,state,created_at,updated_at FROM xiaohongshu_drafts WHERE workspace_id=? AND article_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, workspaceID, articleID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询小红书草稿历史: %w", err)
	}
	defer rows.Close()
	items := make([]domain.Draft, 0, limit)
	for rows.Next() {
		item, scanErr := scanDraft(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// FindLatestDraft 返回文章最新草稿。
func (r *XiaohongshuRepository) FindLatestDraft(ctx context.Context, workspaceID, articleID string) (domain.Draft, bool, error) {
	item, err := scanDraft(r.db.QueryRowContext(ctx, `SELECT id,article_id,workspace_id,source_content_hash,title,body_html,pages_json,topics_json,source_note,comment_copy,ai_model,prompt_version,state,created_at,updated_at FROM xiaohongshu_drafts WHERE workspace_id=? AND article_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, workspaceID, articleID))
	if err == sql.ErrNoRows {
		return domain.Draft{}, false, nil
	}
	return item, err == nil, err
}

// SaveRender 保存一次手机模板渲染元数据。
func (r *XiaohongshuRepository) SaveRender(ctx context.Context, render domain.Render) error {
	if render.ID == "" || render.DraftID == "" || render.ArticleID == "" || render.TemplateID == "" || render.PageCount < 1 {
		return fmt.Errorf("小红书渲染参数不完整")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `INSERT INTO xiaohongshu_renders(id,draft_id,article_id,template_id,template_version,viewport_width,page_height,html_hash,page_count,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, render.ID, render.DraftID, render.ArticleID, render.TemplateID, render.TemplateVersion, render.ViewportWidth, render.PageHeight, render.HTMLHash, render.PageCount, render.State, now, now)
	if err != nil {
		return fmt.Errorf("保存小红书渲染: %w", err)
	}
	return nil
}

// SaveEvent 追加一条小红书审计事件。
func (r *XiaohongshuRepository) SaveEvent(ctx context.Context, event domain.Event) error {
	if event.ID == "" || event.DraftID == "" || event.EventType == "" {
		return fmt.Errorf("小红书审计事件参数不完整")
	}
	if event.Payload == "" {
		event.Payload = "{}"
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	var renderID any
	if event.RenderID != "" {
		renderID = event.RenderID
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO xiaohongshu_events(id,draft_id,render_id,event_type,payload_json,created_at) VALUES(?,?,?,?,?,?)`, event.ID, event.DraftID, renderID, event.EventType, event.Payload, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("保存小红书审计事件: %w", err)
	}
	return nil
}

func scanDraft(row rowScanner) (domain.Draft, error) {
	var item domain.Draft
	var pagesJSON string
	var topicsJSON string
	if err := row.Scan(&item.ID, &item.ArticleID, &item.WorkspaceID, &item.SourceContentHash, &item.Title, &item.BodyHTML, &pagesJSON, &topicsJSON, &item.SourceNote, &item.CommentCopy, &item.AIModel, &item.PromptVersion, &item.State, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return domain.Draft{}, fmt.Errorf("查询小红书草稿: %w", err)
	}
	if pagesJSON != "" && pagesJSON != "null" {
		if err := json.Unmarshal([]byte(pagesJSON), &item.Pages); err != nil {
			return domain.Draft{}, fmt.Errorf("解析小红书页面: %w", err)
		}
	}
	if item.Pages == nil {
		item.Pages = []domain.Page{}
	}
	if err := json.Unmarshal([]byte(topicsJSON), &item.Topics); err != nil {
		return domain.Draft{}, fmt.Errorf("解析小红书话题: %w", err)
	}
	if item.Topics == nil {
		item.Topics = []string{}
	}
	return item, nil
}
