package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// TemplateRepository 以短事务切换工作区活动模板版本。
type TemplateRepository struct {
	db          dbBeginner
	workspaceID string
}

type dbBeginner interface {
	BeginTx(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error)
}

// NewTemplateRepository 创建指定工作区的模板激活 Repository。
func NewTemplateRepository(db *sql.DB, workspaceID string) *TemplateRepository {
	return &TemplateRepository{db: db, workspaceID: workspaceID}
}

// Activate 原子停用旧模板并启用已校验的不可变版本。
func (r *TemplateRepository) Activate(ctx context.Context, id, version, digest, path, target, format, renderer string) error {
	if r.workspaceID == "" || id == "" || version == "" || digest == "" || path == "" || target == "" || format == "" || renderer == "" {
		return fmt.Errorf("模板激活参数不完整")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始模板激活事务: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE templates SET enabled=0,updated_at=? WHERE workspace_id=? AND target=? AND enabled=1`, nowText(), r.workspaceID, target); err != nil {
		return fmt.Errorf("停用旧模板: %w", err)
	}
	manifest, _ := json.Marshal(map[string]string{"digest": digest, "target": target, "format": format, "renderer": renderer})
	stableID := templateRecordID(r.workspaceID, id, version)
	now := nowText()
	result, err := tx.ExecContext(ctx, `INSERT INTO templates(
id,workspace_id,template_id,version,source,manifest_json,install_path,enabled,created_at,updated_at,target,format,renderer)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(workspace_id,template_id,version) DO UPDATE SET enabled=1,updated_at=excluded.updated_at
WHERE templates.manifest_json=excluded.manifest_json AND templates.install_path=excluded.install_path`,
		stableID, r.workspaceID, id, version, "local", string(manifest), path, 1, now, now, target, format, renderer)
	if err != nil {
		return fmt.Errorf("启用模板版本: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("模板版本内容与已安装记录冲突")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交模板激活事务: %w", err)
	}
	return nil
}

func templateRecordID(workspaceID, id, version string) string {
	sum := sha256.Sum256([]byte(workspaceID + "\x00" + id + "\x00" + version))
	return "tpl_" + hex.EncodeToString(sum[:16])
}

func nowText() string { return time.Now().UTC().Format(time.RFC3339Nano) }
