package markdown

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/source/folder"
	"gopkg.in/yaml.v3"
)

// Config 定义普通 Markdown Folder 的扫描范围。
type Config struct {
	SourceID         string
	Root             string
	ContentRoots     []string
	IgnoredFolders   []string
	IgnoredFileNames []string
}

// Provider 读取标准 Markdown 文件，不处理 Obsidian 方言。
type Provider struct {
	config Config
	folder *folder.Source
}

var _ contracts.SourceProvider = (*Provider)(nil)

// New 创建只读 Markdown Folder Provider。
func New(config Config) (*Provider, error) {
	shared, err := folder.New(folder.Config{Root: config.Root, SourceID: config.SourceID, ExcludedDirs: map[string]bool{".git": true}, ContentRoots: config.ContentRoots, IgnoredFolders: config.IgnoredFolders, IgnoredFileNames: config.IgnoredFileNames})
	if err != nil {
		return nil, err
	}
	config.Root = shared.Root()
	return &Provider{config: config, folder: shared}, nil
}

// Descriptor 返回普通 Markdown 只读能力。
func (*Provider) Descriptor() contracts.SourceDescriptor { return NewFactory().Descriptor() }

// Validate 检查目录仍然存在且可访问。
func (p *Provider) Validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(p.config.Root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("Markdown Folder 已不可用: %s", p.config.Root)
	}
	return nil
}

// Scan 返回当前目录中 Markdown 文档的稳定引用和 revision。
func (p *Provider) Scan(ctx context.Context, cursor contracts.ScanCursor) (contracts.ScanResult, error) {
	paths, err := p.folder.MarkdownPaths(ctx)
	if err != nil {
		return contracts.ScanResult{}, err
	}
	result := contracts.ScanResult{Complete: true, Documents: make([]contracts.SourceDocumentRef, 0, len(paths))}
	hash := sha256.New()
	for _, relative := range paths {
		document, readErr := p.Read(ctx, contracts.SourceRef{SourceID: p.config.SourceID, RelativePath: relative})
		item := contracts.SourceDocumentRef{Ref: document.Ref, Fingerprint: document.Fingerprint}
		if readErr != nil {
			item.Ref = contracts.SourceRef{SourceID: p.config.SourceID, RelativePath: relative}
			item.Diagnostics = []contracts.Diagnostic{{Code: "markdown.read_failed", Message: readErr.Error(), Blocking: true}}
		}
		result.Documents = append(result.Documents, item)
		_, _ = hash.Write([]byte(relative))
		_, _ = hash.Write([]byte(item.Fingerprint))
	}
	result.Next.Revision = hex.EncodeToString(hash.Sum(nil))
	if cursor.Revision != "" && cursor.Revision == result.Next.Revision {
		result.Documents = nil
	}
	return result, nil
}

// Read 读取一篇标准 Markdown 文档。
func (p *Provider) Read(ctx context.Context, ref contracts.SourceRef) (contracts.SourceDocument, error) {
	if err := ctx.Err(); err != nil {
		return contracts.SourceDocument{}, err
	}
	path, err := p.folder.Resolve(ref.RelativePath)
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return contracts.SourceDocument{}, fmt.Errorf("读取 Markdown: %w", err)
	}
	document, err := parseDocument(content)
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	if ref.StableID != "" && ref.StableID != string(document.Article.StableID) {
		return contracts.SourceDocument{}, &contracts.ProviderError{Code: "markdown.source_conflict", Category: contracts.ErrorConflict, Message: "Markdown 文件身份与引用不一致"}
	}
	document.Ref = contracts.SourceRef{SourceID: p.config.SourceID, RelativePath: filepath.ToSlash(ref.RelativePath), StableID: string(document.Article.StableID)}
	document.Article.SourceID = p.config.SourceID
	document.Article.RelativePath = document.Ref.RelativePath
	if document.Article.Title == "" {
		document.Article.Title = strings.TrimSuffix(filepath.Base(ref.RelativePath), filepath.Ext(ref.RelativePath))
	}
	return document, nil
}

// WriteMetadata 明确拒绝普通 Markdown Provider 的写回操作。
func (*Provider) WriteMetadata(context.Context, contracts.MetadataWriteCommand) (contracts.SourceDocument, error) {
	return contracts.SourceDocument{}, &contracts.ProviderError{Code: "markdown.write_unsupported", Category: contracts.ErrorPermanent, Message: "Markdown Folder 当前为只读内容源"}
}

// Watch 委托共享文件夹监听能力报告变化。
func (p *Provider) Watch(ctx context.Context, changes chan<- contracts.SourceChange) error {
	return p.folder.Watch(ctx, changes)
}

type frontmatter struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	URL         string   `yaml:"url"`
	PublishDate string   `yaml:"date"`
	Category    string   `yaml:"category"`
	Series      string   `yaml:"series"`
	Tags        []string `yaml:"tags"`
	Keywords    []string `yaml:"keywords"`
	Slug        string   `yaml:"slug"`
	Cover       string   `yaml:"cover"`
}

func parseDocument(content []byte) (contracts.SourceDocument, error) {
	raw, body, err := splitFrontmatter(string(content))
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	var metadata frontmatter
	if raw != "" {
		if err := yaml.Unmarshal([]byte(raw), &metadata); err != nil {
			return contracts.SourceDocument{}, fmt.Errorf("解析 Markdown frontmatter: %w", err)
		}
	}
	sum := sha256.Sum256(content)
	return contracts.SourceDocument{
		Article: article.Article{StableID: article.StableID(metadata.ID), Title: metadata.Title, Description: metadata.Description, URL: metadata.URL, PublishDate: metadata.PublishDate, Category: metadata.Category, Series: metadata.Series, Tags: append([]string{}, metadata.Tags...), Keywords: append([]string{}, metadata.Keywords...), Slug: metadata.Slug, Cover: metadata.Cover},
		Body:    body, RawFrontmatter: raw, Fingerprint: hex.EncodeToString(sum[:]),
	}, nil
}

func splitFrontmatter(content string) (string, string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	opening := "---\n"
	closing := "\n---"
	if strings.HasPrefix(content, "---\r\n") {
		opening = "---\r\n"
		closing = "\r\n---"
	} else if !strings.HasPrefix(content, opening) {
		return "", content, nil
	}
	remaining := content[len(opening):]
	end := strings.Index(remaining, closing)
	if end < 0 {
		return "", "", fmt.Errorf("Markdown frontmatter 未闭合")
	}
	body := remaining[end+len(closing):]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	return remaining[:end], body, nil
}
