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
provider_revision=excluded.provider_revision,last_processed_at=excluded.last_processed_at,updated_at=excluded.updated_at`,
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
