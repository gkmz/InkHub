// Package template 编排多目标模板的安全安装、激活和回滚。
package template

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
)

const (
	maxArchiveBytes     = 10 << 20
	maxExtractedBytes   = 25 << 20
	maxArchiveFileBytes = 8 << 20
	maxArchiveFiles     = 200
	maxCompressionRatio = 100
)

var (
	// ErrVersionConflict 表示相同模板版本已经绑定不同内容。
	ErrVersionConflict = errors.New("模板版本内容冲突")
)

// Activator 在文件安装成功后以短事务切换工作区活动模板。
type Activator interface {
	Activate(ctx context.Context, id, version, digest, path, target, format, renderer string) error
}

// Installed 描述一个已校验并写入不可变目录的模板版本。
type Installed struct {
	ID       string
	Version  string
	Digest   string
	Path     string
	Target   string
	Format   string
	Renderer string
}

// Install 流式解压、校验、原子安装并激活模板包。
func Install(ctx context.Context, archivePath, installRoot string, activator Activator) (Installed, error) {
	if activator == nil {
		return Installed{}, fmt.Errorf("模板 Activator 为空")
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		return Installed{}, fmt.Errorf("读取模板包: %w", err)
	}
	if info.Size() > maxArchiveBytes {
		return Installed{}, fmt.Errorf("模板包超过大小限制")
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return Installed{}, fmt.Errorf("打开模板包: %w", err)
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > maxArchiveFiles {
		return Installed{}, fmt.Errorf("模板包文件数量无效")
	}
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return Installed{}, fmt.Errorf("创建模板安装目录: %w", err)
	}
	staging, err := os.MkdirTemp(installRoot, ".install-*")
	if err != nil {
		return Installed{}, fmt.Errorf("创建模板安装 staging: %w", err)
	}
	defer os.RemoveAll(staging)

	rootName, err := extractArchive(ctx, reader.File, staging)
	if err != nil {
		return Installed{}, err
	}
	validated, err := domaintemplate.ValidateDirectory(filepath.Join(staging, rootName))
	if err != nil {
		return Installed{}, fmt.Errorf("校验模板包: %w", err)
	}
	destination := filepath.Join(installRoot, validated.Manifest.ID, validated.Manifest.Version)
	if existing, validateErr := domaintemplate.ValidateDirectory(destination); validateErr == nil {
		if existing.Digest != validated.Digest {
			return Installed{}, ErrVersionConflict
		}
		installed := installedFromValidated(existing, destination)
		if err := activator.Activate(ctx, installed.ID, installed.Version, installed.Digest, installed.Path, installed.Target, installed.Format, installed.Renderer); err != nil {
			return Installed{}, fmt.Errorf("激活模板: %w", err)
		}
		return installed, nil
	} else if !os.IsNotExist(unwrapPathError(validateErr)) {
		if _, statErr := os.Stat(destination); statErr == nil {
			return Installed{}, ErrVersionConflict
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return Installed{}, fmt.Errorf("创建模板版本目录: %w", err)
	}
	if err := os.Rename(validated.Root, destination); err != nil {
		return Installed{}, fmt.Errorf("原子安装模板版本: %w", err)
	}
	installed := installedFromValidated(validated, destination)
	if err := activator.Activate(ctx, installed.ID, installed.Version, installed.Digest, installed.Path, installed.Target, installed.Format, installed.Renderer); err != nil {
		return Installed{}, fmt.Errorf("激活模板: %w", err)
	}
	return installed, nil
}

func installedFromValidated(validated domaintemplate.Validated, path string) Installed {
	return Installed{ID: validated.Manifest.ID, Version: validated.Manifest.Version, Digest: validated.Digest, Path: path, Target: validated.Manifest.Target, Format: validated.Manifest.Format, Renderer: validated.Manifest.Renderer}
}

func extractArchive(ctx context.Context, files []*zip.File, staging string) (string, error) {
	var rootName string
	var total uint64
	seen := map[string]bool{}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		name := strings.TrimSuffix(file.Name, "/")
		clean := filepath.ToSlash(filepath.Clean(name))
		if name == "" || clean != name || filepath.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") {
			return "", fmt.Errorf("模板包路径越界: %s", file.Name)
		}
		parts := strings.Split(clean, "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("模板包文件必须位于唯一根目录内: %s", clean)
		}
		if rootName == "" {
			rootName = parts[0]
		} else if parts[0] != rootName {
			return "", fmt.Errorf("模板包必须只有一个根目录")
		}
		lower := strings.ToLower(clean)
		if seen[lower] {
			return "", fmt.Errorf("模板包路径大小写冲突: %s", clean)
		}
		seen[lower] = true
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return "", fmt.Errorf("模板包包含特殊文件: %s", clean)
		}
		if file.UncompressedSize64 > maxArchiveFileBytes {
			return "", fmt.Errorf("模板包单文件超过限制: %s", clean)
		}
		total += file.UncompressedSize64
		if total > maxExtractedBytes || (file.CompressedSize64 > 0 && file.UncompressedSize64/file.CompressedSize64 > maxCompressionRatio) {
			return "", fmt.Errorf("模板包解压大小或压缩比超过限制")
		}
		destination := filepath.Join(staging, filepath.FromSlash(clean))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := extractFile(file, destination); err != nil {
			return "", err
		}
	}
	return rootName, nil
}

func extractFile(file *zip.File, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := file.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, maxArchiveFileBytes+1))
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if written > maxArchiveFileBytes {
		return fmt.Errorf("模板解压文件超过限制: %s", file.Name)
	}
	return closeErr
}

func unwrapPathError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
