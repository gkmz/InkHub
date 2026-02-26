package utils

import (
	"os"
	"path/filepath"
)

// FindProjectRoot 向上查找项目根目录
func FindProjectRoot(startPath string) string {
	curr := startPath
	for {
		markers := []string{".git", "hugo.yaml", "hugo.toml", "go.mod", "package.json"}
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(curr, m)); err == nil {
				return curr
			}
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}
