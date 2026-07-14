// Package folder 提供基于本地文件夹的 Source Provider 共享能力。
package folder

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Config 定义文件夹 Source 的共享配置。
type Config struct {
	Root           string
	SourceID       string
	PollInterval   time.Duration
	ExcludedDirs   map[string]bool
	ContentRoots   []string
	IgnoredFolders []string
}

// Source 提供路径授权、Markdown 遍历和变化监听。
type Source struct {
	config Config
	fs     *filesystem.AuthorizedFS
	scope  *Scope
}

// New 创建文件夹 Source 共享组件。
func New(config Config) (*Source, error) {
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fmt.Errorf("解析内容目录: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("内容目录无效: %s", root)
	}
	authorized, err := filesystem.NewAuthorizedFS([]string{root})
	if err != nil {
		return nil, err
	}
	var scope *Scope
	if config.ContentRoots != nil || config.IgnoredFolders != nil {
		value, err := NewScope(config.ContentRoots, config.IgnoredFolders)
		if err != nil {
			return nil, err
		}
		scope = &value
	}
	config.Root = root
	if config.PollInterval <= 0 {
		config.PollInterval = 2 * time.Second
	}
	return &Source{config: config, fs: authorized, scope: scope}, nil
}

// Root 返回规范化后的内容根目录。
func (s *Source) Root() string { return s.config.Root }

// Resolve 将相对路径解析为授权范围内的绝对路径。
func (s *Source) Resolve(relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("内容路径必须是相对路径: %s", relativePath)
	}
	return s.fs.Resolve(filepath.Join(s.config.Root, filepath.FromSlash(relativePath)))
}

// MarkdownPaths 返回稳定排序且未排除的 Markdown 相对路径。
func (s *Source) MarkdownPaths(ctx context.Context) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(s.config.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() && path != s.config.Root && s.excluded(entry.Name()) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			relative, err := filepath.Rel(s.config.Root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if s.scope == nil || s.scope.Includes(relative) {
				paths = append(paths, relative)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描内容目录: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

// Watch 轮询内容目录并持续报告 Markdown 文件变化。
func (s *Source) Watch(ctx context.Context, changes chan<- contracts.SourceChange) error {
	previous, err := s.snapshot(ctx)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current, err := s.snapshot(ctx)
			if err != nil {
				return err
			}
			if err := s.emitChanges(ctx, changes, previous, current); err != nil {
				return err
			}
			previous = current
		}
	}
}

type fileStamp struct {
	size     int64
	modified int64
	digest   [sha256.Size]byte
}

func (s *Source) snapshot(ctx context.Context) (map[string]fileStamp, error) {
	paths, err := s.MarkdownPaths(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]fileStamp, len(paths))
	for _, relative := range paths {
		path, err := s.Resolve(relative)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		// mtime 和大小只能优化诊断，内容摘要才是防止漏报的最终依据。
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		result[relative] = fileStamp{size: info.Size(), modified: info.ModTime().UnixNano(), digest: sha256.Sum256(content)}
	}
	return result, nil
}

func (s *Source) emitChanges(ctx context.Context, output chan<- contracts.SourceChange, previous, current map[string]fileStamp) error {
	// 先报告新增和修改，再报告删除，保证一次轮询内的事件顺序可预测。
	for path, stamp := range current {
		kind := contracts.SourceModified
		old, existed := previous[path]
		if !existed {
			kind = contracts.SourceCreated
		} else if old == stamp {
			continue
		}
		if err := emit(ctx, output, kind, s.config.SourceID, path); err != nil {
			return err
		}
	}
	for path := range previous {
		if _, exists := current[path]; !exists {
			if err := emit(ctx, output, contracts.SourceDeleted, s.config.SourceID, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Source) excluded(name string) bool {
	return strings.HasPrefix(name, ".inkhub-") || s.config.ExcludedDirs[name]
}

func emit(ctx context.Context, output chan<- contracts.SourceChange, kind contracts.SourceChangeKind, sourceID, path string) error {
	select {
	case output <- contracts.SourceChange{Kind: kind, Ref: contracts.SourceRef{SourceID: sourceID, RelativePath: path}}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
