// Package sqlite 实现 InkHub 的 SQLite 持久化基础设施。
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Open 打开数据库、配置安全连接参数并执行 migration。
func Open(ctx context.Context, path string) (*sql.DB, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("解析数据库路径: %w", err)
	}
	_, statErr := os.Stat(absPath)
	existed := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("检查数据库路径: %w", statErr)
	}
	dsn := "file:" + filepath.ToSlash(absPath) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接数据库: %w", err)
	}
	if existed {
		needed, err := migrationNeeded(ctx, db)
		if err != nil {
			db.Close()
			return nil, err
		}
		if needed {
			name := "inkhub-before-migration-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".db"
			destination := filepath.Join(filepath.Dir(absPath), "backups", name)
			if err := Backup(ctx, db, destination); err != nil {
				db.Close()
				return nil, fmt.Errorf("迁移前备份: %w", err)
			}
		}
	}
	if err := Migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
