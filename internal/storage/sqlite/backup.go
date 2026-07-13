package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// Backup 创建数据库一致性快照并验证其完整性。
func Backup(ctx context.Context, db *sql.DB, destination string) error {
	absPath, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("解析备份路径: %w", err)
	}
	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("备份文件已存在: %s", absPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查备份路径: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		return fmt.Errorf("创建备份目录: %w", err)
	}

	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, absPath); err != nil {
		return fmt.Errorf("创建数据库备份: %w", err)
	}
	if err := verifyDatabase(ctx, absPath); err != nil {
		_ = os.Remove(absPath)
		return err
	}
	return nil
}

func verifyDatabase(ctx context.Context, path string) error {
	backup, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return fmt.Errorf("打开备份校验: %w", err)
	}
	defer backup.Close()
	var result string
	if err := backup.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("校验数据库备份: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("数据库备份完整性异常: %s", result)
	}
	return nil
}
