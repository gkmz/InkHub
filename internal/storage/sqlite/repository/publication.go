package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gkmz/InkHub/internal/domain/publication"
)

// PublicationRecord 是渠道处理的当前投影。
type PublicationRecord struct {
	ID                 string
	ArticleID          string
	ProviderInstanceID string
	WorkspaceID        string
	State              publication.State
	ContentHash        string
	ProviderRevision   string
}

// PublicationEvent 是追加保存的渠道状态事件。
type PublicationEvent struct {
	ID          string
	Type        string
	ContentHash string
	Payload     any
}

// PublicationEventCursor 是发布事件 keyset 分页位置。
type PublicationEventCursor struct {
	CreatedAt time.Time
	ID        string
}

// PublicationEventRecord 是统一发布历史所需的持久化事件视图。
type PublicationEventRecord struct {
	ID           string
	ProviderType string
	Type         string
	PayloadJSON  string
	CreatedAt    time.Time
}

// PublicationEventPage 是按时间倒序的发布事件页。
type PublicationEventPage struct {
	Items   []PublicationEventRecord
	HasMore bool
}

// PublicationRepository 保存渠道投影和事件。
type PublicationRepository struct {
	db *sql.DB
}

// NewPublicationRepository 创建 Publication Repository。
func NewPublicationRepository(db *sql.DB) *PublicationRepository {
	return &PublicationRepository{db: db}
}

// Find 查询文章在指定 Provider 的当前渠道投影。
func (r *PublicationRepository) Find(ctx context.Context, articleID, providerInstanceID string) (PublicationRecord, error) {
	var value PublicationRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,article_id,provider_instance_id,workspace_id,state,content_hash,provider_revision FROM publications WHERE article_id=? AND provider_instance_id=?`, articleID, providerInstanceID).Scan(&value.ID, &value.ArticleID, &value.ProviderInstanceID, &value.WorkspaceID, &value.State, &value.ContentHash, &value.ProviderRevision)
	if err != nil {
		return PublicationRecord{}, fmt.Errorf("查询发布投影: %w", err)
	}
	return value, nil
}

// ListEvents 按工作区和文章稳定分页查询跨渠道发布事件。
func (r *PublicationRepository) ListEvents(ctx context.Context, workspaceID, articleID string, cursor PublicationEventCursor, limit int) (PublicationEventPage, error) {
	if limit <= 0 {
		return PublicationEventPage{}, fmt.Errorf("发布历史分页大小无效")
	}
	query := `SELECT publication_events.id,provider_instances.provider_type,publication_events.event_type,
publication_events.payload_json,publication_events.created_at
FROM publication_events
JOIN publications ON publications.id=publication_events.publication_id
JOIN provider_instances ON provider_instances.id=publications.provider_instance_id AND provider_instances.workspace_id=publications.workspace_id
WHERE publications.workspace_id=? AND publications.article_id=?`
	args := []any{workspaceID, articleID}
	if !cursor.CreatedAt.IsZero() || cursor.ID != "" {
		if cursor.CreatedAt.IsZero() || cursor.ID == "" {
			return PublicationEventPage{}, fmt.Errorf("发布历史 cursor 不完整")
		}
		query += ` AND (publication_events.created_at<? OR (publication_events.created_at=? AND publication_events.id<?))`
		encodedTime := cursor.CreatedAt.UTC().Format(time.RFC3339Nano)
		args = append(args, encodedTime, encodedTime, cursor.ID)
	}
	query += ` ORDER BY publication_events.created_at DESC,publication_events.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return PublicationEventPage{}, fmt.Errorf("查询发布历史: %w", err)
	}
	defer rows.Close()
	page := PublicationEventPage{Items: []PublicationEventRecord{}}
	for rows.Next() {
		var item PublicationEventRecord
		var createdAt string
		if err := rows.Scan(&item.ID, &item.ProviderType, &item.Type, &item.PayloadJSON, &createdAt); err != nil {
			return PublicationEventPage{}, fmt.Errorf("解析发布历史: %w", err)
		}
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return PublicationEventPage{}, fmt.Errorf("解析发布历史时间: %w", err)
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return PublicationEventPage{}, fmt.Errorf("遍历发布历史: %w", err)
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.HasMore = true
	}
	return page, nil
}

// SaveWithEvent 在同一事务保存渠道投影和对应事件。
func (r *PublicationRepository) SaveWithEvent(ctx context.Context, record PublicationRecord, event PublicationEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("序列化发布事件: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// 当前投影和追加事件属于同一个业务事实，禁止出现只写入其中一项的状态。
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始发布事务: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO publications(
id,article_id,provider_instance_id,workspace_id,state,content_hash,provider_revision,last_processed_at,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(article_id,provider_instance_id) DO UPDATE SET state=excluded.state,content_hash=excluded.content_hash,
provider_revision=CASE WHEN excluded.provider_revision<>'' THEN excluded.provider_revision ELSE publications.provider_revision END,
last_processed_at=excluded.last_processed_at,updated_at=excluded.updated_at`,
		record.ID, record.ArticleID, record.ProviderInstanceID, record.WorkspaceID, record.State, record.ContentHash,
		record.ProviderRevision, now, now, now)
	if err != nil {
		return fmt.Errorf("保存发布投影: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO publication_events(id,publication_id,event_type,content_hash,payload_json,created_at)
VALUES (?,?,?,?,?,?)`, event.ID, record.ID, event.Type, event.ContentHash, string(payload), now)
	if err != nil {
		return fmt.Errorf("保存发布事件: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交发布事务: %w", err)
	}
	return nil
}
