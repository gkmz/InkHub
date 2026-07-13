// Package filesystem 提供受授权根目录约束的文件操作。
package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AuthorizedFS 校验文件路径是否位于授权根目录内。
type AuthorizedFS struct {
	roots []string
}

// NewAuthorizedFS 创建授权文件系统边界。
func NewAuthorizedFS(roots []string) (*AuthorizedFS, error) {
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		value, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("解析授权根目录: %w", err)
		}
		value, err = filepath.EvalSymlinks(value)
		if err != nil {
			return nil, fmt.Errorf("解析授权根目录符号链接: %w", err)
		}
		resolved = append(resolved, filepath.Clean(value))
	}
	return &AuthorizedFS{roots: resolved}, nil
}

// Resolve 返回规范化且位于授权根目录内的绝对路径。
func (fs *AuthorizedFS) Resolve(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析文件路径: %w", err)
	}
	resolved, err := resolveExistingParent(absPath)
	if err != nil {
		return "", err
	}
	for _, root := range fs.roots {
		relative, err := filepath.Rel(root, resolved)
		if err != nil {
			continue
		}
		if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("路径不在授权目录内: %s", path)
}

func resolveExistingParent(path string) (string, error) {
	current := filepath.Clean(path)
	var suffix []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("解析路径符号链接: %w", err)
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("检查文件路径: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("找不到有效路径父目录: %s", path)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
