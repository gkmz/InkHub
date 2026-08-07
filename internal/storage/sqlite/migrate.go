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

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("获取 migration 数据库连接: %w", err)
	}
	defer conn.Close()

	var existing string
	err = conn.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, version).Scan(&existing)
	if err == nil {
		if existing != checksum {
			return fmt.Errorf("migration %s checksum 已变化", name)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("查询 migration %s: %w", name, err)
	}

	foreignKeysOff := strings.Contains(string(content), "-- inkhub: foreign_keys_off")
	if foreignKeysOff {
		// SQLite 重建被外键引用的表时，必须在同一连接、事务开始前暂停外键检查。
		if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("暂停 migration 外键检查: %w", err)
		}
		defer conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)
	}

	// 每个 migration 单独使用事务，DDL 和版本记录必须同时成功或同时回滚。
	tx, err := conn.BeginTx(ctx, nil)
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
	if foreignKeysOff {
		if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
			return fmt.Errorf("恢复 migration 外键检查: %w", err)
		}
		var table, rowID, parent string
		var foreignKeyID int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_key_check`).Scan(&table, &rowID, &parent, &foreignKeyID); err != sql.ErrNoRows {
			if err == nil {
				return fmt.Errorf("migration %s 产生无效外键: table=%s row=%s parent=%s key=%d", name, table, rowID, parent, foreignKeyID)
			}
			return fmt.Errorf("检查 migration 外键: %w", err)
		}
	}
	return nil
}

var tableComments = map[string]string{
	"schema_migrations":    "记录已应用的数据库结构版本与校验摘要",
	"schema_comments":      "保存数据库表和字段的中文说明",
	"workspaces":           "保存 InkHub 工作区及最近使用状态",
	"sources":              "保存工作区内容源配置与扫描状态",
	"articles":             "保存可重建的文章索引和标准元数据快照",
	"article_dispositions": "保存用户对文章当前版本的外部发表或长期忽略决定",
	"editorial_reviews":    "保存文章审核状态和已确认内容版本",
	"taxonomy_terms":       "缓存权威 taxonomy 条目及使用统计",
	"taxonomy_snapshots":   "保存 Provider taxonomy 最近成功 revision 与刷新状态",
	"article_taxonomies":   "关联文章与 taxonomy 条目",
	"provider_instances":   "保存工作区 Provider 实例的非敏感配置",
	"publications":         "保存文章在渠道中的当前处理投影",
	"publication_events":   "追加保存渠道处理状态事件",
	"ai_suggestions":       "保存 AI 建议及用户采用结果",
	"templates":            "保存按渲染目标隔离的已安装模板版本与来源",
	"jobs":                 "保存可恢复后台任务及执行结果",
	"settings":             "保存工作区非敏感设置",
	"xiaohongshu_drafts":   "保存小红书完整内容草稿及版本历史",
	"xiaohongshu_renders":  "保存小红书手机模板渲染版本",
	"xiaohongshu_events":   "追加保存小红书草稿、渲染和发布审计事件",
}

var columnComments = map[string]string{
	"id": "记录的稳定内部标识", "workspace_id": "所属工作区标识", "source_id": "所属内容源标识",
	"article_id": "关联文章标识", "provider_instance_id": "关联 Provider 实例标识", "taxonomy_term_id": "关联 taxonomy 条目标识",
	"name": "可读名称", "version": "版本号", "checksum": "内容 SHA-256 校验摘要", "applied_at": "应用时间（UTC）",
	"object_type": "注释对象类型", "object_name": "注释对象完整名称", "comment": "对象中文说明",
	"created_at": "创建时间（UTC）", "updated_at": "最后更新时间（UTC）", "deleted_at": "软删除时间（UTC）",
	"source_content_hash": "草稿生成时对应的文章内容版本", "mode": "小红书草稿内容模式", "body_html": "小红书正文 HTML 或分镜模式发布短文", "pages_json": "小红书长文分页卡片和内容块 JSON", "script_pages_json": "小红书逐页生图分镜提示词 JSON", "topics_json": "小红书话题 JSON 数组",
	"source_note": "小红书来源说明", "comment_copy": "小红书评论区文案", "ai_model": "生成草稿使用的模型标识",
	"prompt_version": "生成草稿使用的提示词版本", "draft_id": "关联小红书草稿标识", "render_id": "关联小红书渲染标识",
	"template_id": "手机页面模板标识", "template_version": "手机页面模板版本", "viewport_width": "渲染视口宽度（像素）",
	"page_height": "渲染页面高度（像素）", "html_hash": "渲染 HTML 内容摘要", "page_count": "渲染图片页数",
	"event_type": "审计事件类型",
	"cleared_at": "文章恢复管理的时间（UTC）", "kind": "记录的业务类型",
	"state": "当前状态", "content_hash": "实际使用的文章内容版本", "frontmatter_hash": "标准元数据版本",
	"config_json": "经过 schema 校验的非敏感配置", "capabilities_json": "Provider 能力快照", "payload_json": "结构化任务或事件输入",
	"result_json": "结构化任务结果", "error_code": "稳定错误码", "error_message": "脱敏错误摘要",
	"external_key": "Provider 内稳定 taxonomy term 标识", "metadata_json": "Provider term 元数据 JSON",
	"source_revision": "生成当前投影的权威来源 revision", "usage_count": "term 在文章中的使用次数",
	"revision": "最近成功发现的权威来源 revision", "complete": "快照是否为完整发现结果",
	"last_error_code": "最近刷新失败的稳定错误码", "last_error_message": "最近刷新失败的脱敏说明",
	"content_stage": "作者控制的文章内容阶段", "content_stage_issue": "文章内容阶段字段的修复提示",
	"last_attempt_at": "最近刷新尝试时间（UTC）", "last_success_at": "最近成功刷新时间（UTC）",
	"canonical_name": "term 的规范显示名称",
	"target":         "模板适用的稳定渲染目标", "format": "模板入口内容格式", "renderer": "模板要求的 Renderer 契约标识",
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
