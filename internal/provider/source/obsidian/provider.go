// Package obsidian 实现 Obsidian Vault Source Provider。
package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/source/folder"
)

// ErrSourceChanged 表示源文件已在外部修改。
var ErrSourceChanged = errors.New("源文件已变化")

// ErrSourceConflict 表示路径和稳定 ID 指向的文章身份不一致。
var ErrSourceConflict = errors.New("源文章身份冲突")

// Config 定义 Obsidian Source Provider 配置。
type Config struct {
	SourceID         string
	Root             string
	PollInterval     time.Duration
	ContentRoots     []string
	IgnoredFolders   []string
	IgnoredFileNames []string
}

// Provider 读取和受控写回 Obsidian Vault。
type Provider struct {
	config Config
	folder *folder.Source
}

var _ contracts.SourceProvider = (*Provider)(nil)

// Descriptor 返回当前 Obsidian Provider 的稳定能力描述。
func (p *Provider) Descriptor() contracts.SourceDescriptor { return NewFactory().Descriptor() }

// Validate 检查 Vault 和共享文件夹能力仍然可用。
func (p *Provider) Validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(p.config.Root, ".obsidian"))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("Obsidian Vault 已不可用: %s", p.config.Root)
	}
	return nil
}

// New 创建并校验 Obsidian Provider。
func New(config Config) (*Provider, error) {
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fmt.Errorf("解析 Vault 路径: %w", err)
	}
	if info, err := os.Stat(filepath.Join(root, ".obsidian")); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("目录不是有效 Obsidian Vault: %s", root)
	}
	folderSource, err := folder.New(folder.Config{
		Root: root, SourceID: config.SourceID, PollInterval: config.PollInterval,
		ExcludedDirs: map[string]bool{".obsidian": true, ".git": true},
		ContentRoots: config.ContentRoots, IgnoredFolders: config.IgnoredFolders,
		IgnoredFileNames: config.IgnoredFileNames,
	})
	if err != nil {
		return nil, err
	}
	config.Root = root
	if config.PollInterval <= 0 {
		config.PollInterval = 2 * time.Second
	}
	return &Provider{config: config, folder: folderSource}, nil
}

// Read 读取并解析一篇 Markdown 源文章。
func (p *Provider) Read(ctx context.Context, ref contracts.SourceRef) (contracts.SourceDocument, error) {
	if err := ctx.Err(); err != nil {
		return contracts.SourceDocument{}, err
	}
	path, err := p.resolve(ref.RelativePath)
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return contracts.SourceDocument{}, fmt.Errorf("读取文章: %w", err)
	}
	document, err := parseDocument(content)
	if err != nil {
		return contracts.SourceDocument{}, fmt.Errorf("解析文章 %s: %w", ref.RelativePath, err)
	}
	if ref.StableID != "" && ref.StableID != string(document.Article.StableID) {
		return contracts.SourceDocument{}, fmt.Errorf("%w: path=%s stable_id=%s", ErrSourceConflict, ref.RelativePath, ref.StableID)
	}
	document.Ref = contracts.SourceRef{
		SourceID:     p.config.SourceID,
		RelativePath: filepath.ToSlash(ref.RelativePath),
		StableID:     string(document.Article.StableID),
	}
	document.Article.SourceID = p.config.SourceID
	document.Article.RelativePath = filepath.ToSlash(ref.RelativePath)
	if strings.TrimSpace(document.Article.Title) == "" {
		// 普通 Obsidian 笔记通常没有 title 属性，展示时使用文件名但不写回源文件。
		base := filepath.Base(ref.RelativePath)
		document.Article.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return document, nil
}

// WriteMetadata 以乐观并发方式原子写回标准 frontmatter。
func (p *Provider) WriteMetadata(ctx context.Context, command contracts.MetadataWriteCommand) (contracts.SourceDocument, error) {
	current, err := p.Read(ctx, command.Ref)
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	if current.Fingerprint != command.ExpectedFingerprint {
		return contracts.SourceDocument{}, ErrSourceChanged
	}
	content, err := applyMetadataPatch(current.RawFrontmatter, current.Body, command.Patch)
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	path, err := p.resolve(command.Ref.RelativePath)
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	if err := filesystem.AtomicWrite(path, content, func(temp string) error {
		candidate, err := os.ReadFile(temp)
		if err != nil {
			return err
		}
		_, err = parseDocument(candidate)
		return err
	}); err != nil {
		return contracts.SourceDocument{}, err
	}
	return p.Read(ctx, command.Ref)
}

func (p *Provider) resolve(relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("文章路径必须相对 Vault: %s", relativePath)
	}
	return p.folder.Resolve(filepath.ToSlash(relativePath))
}
