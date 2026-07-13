package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate 按版本执行尚未应用的数据库 migration。
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("创建 migration 目录: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("读取 migration: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := applyMigration(ctx, db, entry.Name()); err != nil {
			return err
		}
	}
	return ensureSchemaComments(ctx, db)
}

func migrationNeeded(ctx context.Context, db *sql.DB) (bool, error) {
	latest, err := latestMigrationVersion()
	if err != nil {
		return false, err
	}
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name='schema_migrations'`).Scan(&exists); err != nil {
		return false, fmt.Errorf("检查 migration 目录: %w", err)
	}
	if exists == 0 {
		return true, nil
	}
	var current sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&current); err != nil {
		return false, fmt.Errorf("读取当前 schema 版本: %w", err)
	}
	return !current.Valid || int(current.Int64) < latest, nil
}

func latestMigrationVersion() (int, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return 0, fmt.Errorf("读取 migration: %w", err)
	}
	latest := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%04d_", &version); err != nil {
			return 0, fmt.Errorf("无效 migration 文件名 %q", entry.Name())
		}
		if version > latest {
			latest = version
		}
	}
	return latest, nil
}

func applyMigration(ctx context.Context, db *sql.DB, name string) error {
	var version int
	if _, err := fmt.Sscanf(name, "%04d_", &version); err != nil {
		return fmt.Errorf("无效 migration 文件名 %q", name)
	}
	content, err := migrationFiles.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("读取 migration %s: %w", name, err)
	}
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	var existing string
	err = db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, version).Scan(&existing)
	if err == nil {
		if existing != checksum {
			return fmt.Errorf("migration %s checksum 已变化", name)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("查询 migration %s: %w", name, err)
	}

	// 每个 migration 单独使用事务，DDL 和版本记录必须同时成功或同时回滚。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 migration %s: %w", name, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("执行 migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		version, name, checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("记录 migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 migration %s: %w", name, err)
	}
	return nil
}

var tableComments = map[string]string{
	"schema_migrations":  "记录已应用的数据库结构版本与校验摘要",
	"schema_comments":    "保存数据库表和字段的中文说明",
	"workspaces":         "保存 InkHub 工作区及最近使用状态",
	"sources":            "保存工作区内容源配置与扫描状态",
	"articles":           "保存可重建的文章索引和标准元数据快照",
	"editorial_reviews":  "保存文章审核状态和已确认内容版本",
	"taxonomy_terms":     "缓存权威 taxonomy 条目及使用统计",
	"article_taxonomies": "关联文章与 taxonomy 条目",
	"provider_instances": "保存工作区 Provider 实例的非敏感配置",
	"publications":       "保存文章在渠道中的当前处理投影",
	"publication_events": "追加保存渠道处理状态事件",
	"ai_suggestions":     "保存 AI 建议及用户采用结果",
	"templates":          "保存已安装微信模板的版本与来源",
	"jobs":               "保存可恢复后台任务及执行结果",
	"settings":           "保存工作区非敏感设置",
}

var columnComments = map[string]string{
	"id": "记录的稳定内部标识", "workspace_id": "所属工作区标识", "source_id": "所属内容源标识",
	"article_id": "关联文章标识", "provider_instance_id": "关联 Provider 实例标识", "taxonomy_term_id": "关联 taxonomy 条目标识",
	"name": "可读名称", "version": "版本号", "checksum": "内容 SHA-256 校验摘要", "applied_at": "应用时间（UTC）",
	"object_type": "注释对象类型", "object_name": "注释对象完整名称", "comment": "对象中文说明",
	"created_at": "创建时间（UTC）", "updated_at": "最后更新时间（UTC）", "deleted_at": "软删除时间（UTC）",
	"state": "当前状态", "content_hash": "实际使用的文章内容版本", "frontmatter_hash": "标准元数据版本",
	"config_json": "经过 schema 校验的非敏感配置", "capabilities_json": "Provider 能力快照", "payload_json": "结构化任务或事件输入",
	"result_json": "结构化任务结果", "error_code": "稳定错误码", "error_message": "脱敏错误摘要",
}

func ensureSchemaComments(ctx context.Context, db *sql.DB) error {
	// 每次启动按实际 schema 校验并补齐注释，避免 migration 新增字段却遗漏数据库说明。
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return fmt.Errorf("读取 schema 表: %w", err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, table)
	}
	rows.Close()

	for _, table := range tables {
		description := tableComments[table]
		if description == "" {
			description = "保存 " + table + " 领域数据"
		}
		if err := upsertComment(ctx, db, "table", table, description); err != nil {
			return err
		}
		columns, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			return fmt.Errorf("读取表 %s 字段: %w", table, err)
		}
		for columns.Next() {
			var column string
			if err := columns.Scan(&column); err != nil {
				columns.Close()
				return err
			}
			description := columnComments[column]
			if description == "" {
				description = humanizeColumn(column)
			}
			if err := upsertComment(ctx, db, "column", table+"."+column, description); err != nil {
				columns.Close()
				return err
			}
		}
		columns.Close()
	}
	return nil
}

func upsertComment(ctx context.Context, db *sql.DB, objectType, objectName, comment string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO schema_comments(object_type, object_name, comment) VALUES (?, ?, ?)
ON CONFLICT(object_type, object_name) DO UPDATE SET comment = excluded.comment`, objectType, objectName, comment)
	return err
}

func humanizeColumn(column string) string {
	return "保存" + strings.ReplaceAll(column, "_", " ") + "字段值"
}
