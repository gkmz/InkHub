package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// TaxonomyRefreshStatus 描述最近一次 taxonomy 刷新的状态。
type TaxonomyRefreshStatus struct {
	State            string
	LastErrorCode    string
	LastErrorMessage string
	LastAttemptAt    time.Time
	LastSuccessAt    *time.Time
}

// TaxonomyRepository 持久化可重建的 Provider taxonomy 快照。
type TaxonomyRepository struct {
	db *sql.DB
}

// NewTaxonomyRepository 创建 taxonomy 快照仓库。
func NewTaxonomyRepository(db *sql.DB) *TaxonomyRepository { return &TaxonomyRepository{db: db} }

// ReplaceSnapshot 在单个事务中替换 Provider 的完整成功快照。
func (r *TaxonomyRepository) ReplaceSnapshot(ctx context.Context, workspaceID string, snapshot contracts.TaxonomySnapshot, now time.Time) error {
	if r == nil || r.db == nil || workspaceID == "" || snapshot.ProviderRef.ID == "" || snapshot.Revision == "" || !snapshot.Complete {
		return fmt.Errorf("taxonomy snapshot 参数不完整")
	}
	for _, term := range snapshot.Terms {
		if term.Kind == "" || term.Key == "" || term.Name == "" || term.CanonicalName == "" || term.UsageCount < 0 {
			return fmt.Errorf("taxonomy term 参数不完整")
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var providerType string
	if err := tx.QueryRowContext(ctx, `SELECT provider_type FROM provider_instances WHERE id=? AND workspace_id=? AND enabled=1`, snapshot.ProviderRef.ID, workspaceID).Scan(&providerType); err != nil {
		return fmt.Errorf("校验 Taxonomy Provider 实例: %w", err)
	}
	if providerType != string(snapshot.ProviderRef.Type) {
		return fmt.Errorf("Taxonomy Provider 类型不匹配")
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_taxonomies WHERE workspace_id=? AND taxonomy_term_id IN (SELECT id FROM taxonomy_terms WHERE provider_instance_id=?)`, workspaceID, snapshot.ProviderRef.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM taxonomy_terms WHERE workspace_id=? AND provider_instance_id=?`, workspaceID, snapshot.ProviderRef.ID); err != nil {
		return err
	}
	for _, term := range snapshot.Terms {
		metadata, marshalErr := json.Marshal(term.Metadata)
		if marshalErr != nil {
			return fmt.Errorf("编码 taxonomy term 元数据: %w", marshalErr)
		}
		id := taxonomyTermID(snapshot.ProviderRef.ID, term.Kind, term.Key)
		if _, err := tx.ExecContext(ctx, `INSERT INTO taxonomy_terms(id,workspace_id,provider_instance_id,kind,external_key,name,canonical_name,metadata_json,usage_count,source_revision,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, workspaceID, snapshot.ProviderRef.ID, term.Kind, term.Key, term.Name, term.CanonicalName, string(metadata), term.UsageCount, snapshot.Revision, stamp); err != nil {
			return fmt.Errorf("保存 taxonomy term: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO taxonomy_snapshots(provider_instance_id,workspace_id,revision,state,complete,last_error_code,last_error_message,last_attempt_at,last_success_at,updated_at) VALUES(?,?,?,'ready',1,NULL,NULL,?,?,?) ON CONFLICT(provider_instance_id) DO UPDATE SET revision=excluded.revision,state='ready',complete=1,last_error_code=NULL,last_error_message=NULL,last_attempt_at=excluded.last_attempt_at,last_success_at=excluded.last_success_at,updated_at=excluded.updated_at`, snapshot.ProviderRef.ID, workspaceID, snapshot.Revision, stamp, stamp, stamp); err != nil {
		return fmt.Errorf("保存 taxonomy snapshot 状态: %w", err)
	}
	return tx.Commit()
}

// MarkRefreshFailed 记录失败状态，同时保留最近成功的 revision 和 terms。
func (r *TaxonomyRepository) MarkRefreshFailed(ctx context.Context, workspaceID, providerID, code, message string, now time.Time) error {
	if r == nil || r.db == nil || workspaceID == "" || providerID == "" {
		return fmt.Errorf("taxonomy 刷新失败参数不完整")
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `INSERT INTO taxonomy_snapshots(provider_instance_id,workspace_id,revision,state,complete,last_error_code,last_error_message,last_attempt_at,updated_at) SELECT id,workspace_id,'','failed',0,?,?,?,? FROM provider_instances WHERE id=? AND workspace_id=? ON CONFLICT(provider_instance_id) DO UPDATE SET state='failed',last_error_code=excluded.last_error_code,last_error_message=excluded.last_error_message,last_attempt_at=excluded.last_attempt_at,updated_at=excluded.updated_at`, code, message, stamp, stamp, providerID, workspaceID)
	if err != nil {
		return fmt.Errorf("记录 taxonomy 刷新失败: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return fmt.Errorf("Taxonomy Provider 实例不存在")
	}
	return nil
}

// MarkRefreshSucceeded 记录一次未变化的成功刷新，不重写 term 集合。
func (r *TaxonomyRepository) MarkRefreshSucceeded(ctx context.Context, workspaceID, providerID string, now time.Time) error {
	stamp := now.UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `UPDATE taxonomy_snapshots SET state='ready',last_error_code=NULL,last_error_message=NULL,last_attempt_at=?,last_success_at=?,updated_at=? WHERE provider_instance_id=? AND workspace_id=?`, stamp, stamp, stamp, providerID, workspaceID)
	if err != nil {
		return fmt.Errorf("更新 taxonomy 刷新时间: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return fmt.Errorf("taxonomy snapshot 不存在")
	}
	return nil
}

// FindSnapshot 查找缓存快照，不存在时返回 found=false。
func (r *TaxonomyRepository) FindSnapshot(ctx context.Context, workspaceID, providerID string) (contracts.TaxonomySnapshot, bool, error) {
	snapshot, _, err := r.GetSnapshot(ctx, workspaceID, providerID)
	if err == sql.ErrNoRows {
		return contracts.TaxonomySnapshot{}, false, nil
	}
	return snapshot, err == nil, err
}

// GetSnapshot 读取最近成功 term 投影和当前刷新状态。
func (r *TaxonomyRepository) GetSnapshot(ctx context.Context, workspaceID, providerID string) (contracts.TaxonomySnapshot, TaxonomyRefreshStatus, error) {
	var providerType, revision, state, code, message, attempt string
	var complete int
	var success sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT provider_instances.provider_type,taxonomy_snapshots.revision,taxonomy_snapshots.state,taxonomy_snapshots.complete,COALESCE(taxonomy_snapshots.last_error_code,''),COALESCE(taxonomy_snapshots.last_error_message,''),taxonomy_snapshots.last_attempt_at,taxonomy_snapshots.last_success_at FROM taxonomy_snapshots JOIN provider_instances ON provider_instances.id=taxonomy_snapshots.provider_instance_id AND provider_instances.workspace_id=taxonomy_snapshots.workspace_id WHERE taxonomy_snapshots.provider_instance_id=? AND taxonomy_snapshots.workspace_id=?`, providerID, workspaceID).Scan(&providerType, &revision, &state, &complete, &code, &message, &attempt, &success)
	if err != nil {
		return contracts.TaxonomySnapshot{}, TaxonomyRefreshStatus{}, err
	}
	status := TaxonomyRefreshStatus{State: state, LastErrorCode: code, LastErrorMessage: message}
	status.LastAttemptAt, _ = time.Parse(time.RFC3339Nano, attempt)
	if success.Valid {
		parsed, parseErr := time.Parse(time.RFC3339Nano, success.String)
		if parseErr == nil {
			status.LastSuccessAt = &parsed
		}
	}
	rows, err := r.db.QueryContext(ctx, `SELECT kind,external_key,name,canonical_name,usage_count,metadata_json FROM taxonomy_terms WHERE workspace_id=? AND provider_instance_id=? ORDER BY kind,external_key`, workspaceID, providerID)
	if err != nil {
		return contracts.TaxonomySnapshot{}, TaxonomyRefreshStatus{}, err
	}
	defer rows.Close()
	var terms []contracts.TaxonomyTerm
	for rows.Next() {
		var term contracts.TaxonomyTerm
		var metadata string
		if err := rows.Scan(&term.Kind, &term.Key, &term.Name, &term.CanonicalName, &term.UsageCount, &metadata); err != nil {
			return contracts.TaxonomySnapshot{}, TaxonomyRefreshStatus{}, err
		}
		if err := json.Unmarshal([]byte(metadata), &term.Metadata); err != nil {
			return contracts.TaxonomySnapshot{}, TaxonomyRefreshStatus{}, fmt.Errorf("解析 taxonomy term 元数据: %w", err)
		}
		terms = append(terms, term)
	}
	snapshot := contracts.TaxonomySnapshot{ProviderRef: contracts.ProviderRef{ID: providerID, Type: contracts.ProviderType(providerType)}, Revision: revision, Terms: terms, Complete: complete == 1}
	return snapshot, status, rows.Err()
}

func taxonomyTermID(providerID, kind, key string) string {
	sum := sha256.Sum256([]byte(providerID + "\x00" + kind + "\x00" + key))
	return "taxonomy_" + hex.EncodeToString(sum[:12])
}
