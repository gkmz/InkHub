package hugo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	platformprocess "github.com/gkmz/InkHub/internal/platform/process"
)

// CLIBuilder 使用官方 Hugo CLI 构建站点，不经过 shell。
type CLIBuilder struct {
	Executable     string
	MaxOutputBytes int
}

// Build 执行 Hugo 构建并返回生成目录的确定性 revision。
func (b CLIBuilder) Build(ctx context.Context, root string) (string, error) {
	executable := b.Executable
	if executable == "" {
		executable = "hugo"
	}
	destination := filepath.Join(root, "public")
	result, err := (platformprocess.Runner{}).Run(ctx, platformprocess.Request{
		Executable: executable,
		Arguments: []string{
			"--source", root,
			"--destination", destination,
			"--cleanDestinationDir",
			"--noBuildLock",
		},
		WorkingDir: root, MaxOutputBytes: b.MaxOutputBytes,
	})
	if err != nil {
		return "", fmt.Errorf("Hugo 构建失败: %w; stderr=%s", err, result.Stderr)
	}
	revision, err := directoryRevision(destination)
	if err != nil {
		return "", fmt.Errorf("计算 Hugo 构建 revision: %w", err)
	}
	return revision, nil
}

func directoryRevision(root string) (string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Hugo 输出包含符号链接: %s", path)
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
