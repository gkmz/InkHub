package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
)

// ValidateContent 在原子替换前校验临时文件内容。
type ValidateContent func(path string) error

// AtomicWrite 在目标同目录校验并原子替换文件。
func AtomicWrite(path string, content []byte, validate ValidateContent) error {
	directory := filepath.Dir(path)
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取目标文件权限: %w", err)
	}

	// 临时文件必须与目标位于同一目录，确保 rename 不跨文件系统并保持原子性。
	temporary, err := os.CreateTemp(directory, ".inkhub-*")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("设置临时文件权限: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("同步临时文件: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if validate != nil {
		if err := validate(temporaryPath); err != nil {
			return fmt.Errorf("校验临时文件: %w", err)
		}
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("原子替换文件: %w", err)
	}
	return nil
}
