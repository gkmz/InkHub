package obsidian

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// AssetReferenceKind 区分 Markdown 相对图片和 Obsidian Wiki 嵌入。
type AssetReferenceKind string

const (
	// AssetMarkdownImage 表示标准 Markdown 图片引用。
	AssetMarkdownImage AssetReferenceKind = "markdown"
	// AssetWikiEmbed 表示 Obsidian `![[...]]` 图片嵌入。
	AssetWikiEmbed AssetReferenceKind = "wiki"
)

// ResolvedAsset 是经过 Vault 边界校验的图片资源。
type ResolvedAsset struct {
	RelativePath string
	AbsolutePath string
	RemoteURL    string
}

// ResolveResource 将通用资源类型适配到 Obsidian 图片解析规则。
func (p *Provider) ResolveResource(ctx context.Context, ref contracts.SourceRef, raw string, kind contracts.ResourceKind) (contracts.ResolvedResource, error) {
	if kind != contracts.ResourceMarkdownImage && kind != contracts.ResourceWikiEmbed {
		return contracts.ResolvedResource{}, fmt.Errorf("不支持的资源引用类型: %s", kind)
	}
	asset, err := p.ResolveAsset(ctx, ref, raw, AssetReferenceKind(kind))
	if err != nil {
		return contracts.ResolvedResource{}, err
	}
	return contracts.ResolvedResource{RelativePath: asset.RelativePath, AbsolutePath: asset.AbsolutePath, RemoteURL: asset.RemoteURL}, nil
}

// ResolveAsset 按 Obsidian 规则解析文章图片，并拒绝 Vault 外本地路径。
func (p *Provider) ResolveAsset(ctx context.Context, ref contracts.SourceRef, raw string, kind AssetReferenceKind) (ResolvedAsset, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedAsset{}, err
	}
	raw = strings.TrimSpace(strings.SplitN(raw, "|", 2)[0])
	parsed, err := url.Parse(raw)
	if err != nil {
		return ResolvedAsset{}, fmt.Errorf("解析图片引用: %w", err)
	}
	if parsed.IsAbs() {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return ResolvedAsset{RemoteURL: parsed.String()}, nil
		}
		return ResolvedAsset{}, fmt.Errorf("不支持的图片协议: %s", parsed.Scheme)
	}
	decoded, err := url.PathUnescape(parsed.Path)
	if err != nil || decoded == "" {
		return ResolvedAsset{}, fmt.Errorf("本地图片路径无效")
	}
	var candidate string
	if kind == AssetMarkdownImage {
		candidate = p.resolveMarkdownAsset(decoded, ref.RelativePath)
	} else {
		candidate, err = p.resolveWikiAsset(decoded, ref.RelativePath)
		if err != nil {
			return ResolvedAsset{}, err
		}
	}
	return p.authorizeAsset(candidate)
}

func (p *Provider) resolveMarkdownAsset(reference, articleRelative string) string {
	// Markdown 使用 / 开头表示 Vault 根路径，其他路径相对当前笔记目录。
	if strings.HasPrefix(filepath.ToSlash(reference), "/") {
		return filepath.Join(p.config.Root, filepath.FromSlash(strings.TrimPrefix(filepath.ToSlash(reference), "/")))
	}
	return filepath.Join(p.config.Root, filepath.Dir(filepath.FromSlash(articleRelative)), filepath.FromSlash(reference))
}

func (p *Provider) resolveWikiAsset(reference, articleRelative string) (string, error) {
	// 以 ./ 或 ../ 开头的 Wiki 图片引用按文章目录解析，兼容 Obsidian 中的相对资源路径。
	if reference == "." || reference == ".." || strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "../") {
		return filepath.Join(p.config.Root, filepath.Dir(filepath.FromSlash(articleRelative)), filepath.FromSlash(reference)), nil
	}
	// WikiLink 以 / 开头时表示 Vault 根路径，而不是本机文件系统绝对路径。
	if strings.HasPrefix(filepath.ToSlash(reference), "/") {
		return filepath.Join(p.config.Root, filepath.FromSlash(strings.TrimPrefix(filepath.ToSlash(reference), "/"))), nil
	}
	if strings.Contains(filepath.ToSlash(reference), "/") {
		return filepath.Join(p.config.Root, filepath.FromSlash(reference)), nil
	}
	settings, _ := readObsidianSettings(p.config.Root)
	candidates := p.shortAssetCandidates(reference, articleRelative, settings.AttachmentFolder)
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	var matches []string
	err := filepath.WalkDir(p.config.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".obsidian" || entry.Name() == ".git" || entry.Name() == ".trash") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == reference {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("图片文件名无法唯一解析: %s", reference)
	}
	return matches[0], nil
}

func (p *Provider) shortAssetCandidates(reference, articleRelative string, location AttachmentLocation) []string {
	articleDir := filepath.Dir(filepath.FromSlash(articleRelative))
	root := p.config.Root
	configured := func() string {
		if location.Path == "" {
			return filepath.Join(root, reference)
		}
		return filepath.Join(root, filepath.FromSlash(location.Path), reference)
	}
	current := filepath.Join(root, articleDir, reference)
	currentSubfolder := current
	if location.Path != "" {
		currentSubfolder = filepath.Join(root, articleDir, filepath.FromSlash(location.Path), reference)
	}
	ordered := make([]string, 0, 4)
	appendUnique := func(value string) {
		for _, existing := range ordered {
			if filepath.Clean(existing) == filepath.Clean(value) {
				return
			}
		}
		ordered = append(ordered, value)
	}
	switch location.Kind {
	case AttachmentAtCurrentFolder:
		appendUnique(current)
	case AttachmentAtCurrentSubfolder:
		appendUnique(currentSubfolder)
	case AttachmentAtConfiguredFolder:
		appendUnique(configured())
	case AttachmentAtVaultRoot:
		appendUnique(filepath.Join(root, reference))
	}
	// 兼容切换 Obsidian 附件设置前已经存在的文章引用。
	appendUnique(current)
	appendUnique(filepath.Join(root, reference))
	return ordered
}

func (p *Provider) authorizeAsset(candidate string) (ResolvedAsset, error) {
	root, err := filepath.EvalSymlinks(p.config.Root)
	if err != nil {
		return ResolvedAsset{}, err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(candidate))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResolvedAsset{}, fmt.Errorf("图片不存在")
		}
		return ResolvedAsset{}, err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ResolvedAsset{}, fmt.Errorf("图片路径超出 Vault")
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return ResolvedAsset{}, fmt.Errorf("图片文件无效")
	}
	return ResolvedAsset{RelativePath: filepath.ToSlash(relative), AbsolutePath: resolved}, nil
}
