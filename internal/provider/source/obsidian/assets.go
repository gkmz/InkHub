package obsidian

import (
	"context"
	"encoding/json"
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
	if err != nil || decoded == "" || filepath.IsAbs(decoded) {
		return ResolvedAsset{}, fmt.Errorf("本地图片路径无效")
	}
	var candidate string
	if kind == AssetMarkdownImage {
		candidate = filepath.Join(p.config.Root, filepath.Dir(filepath.FromSlash(ref.RelativePath)), filepath.FromSlash(decoded))
	} else {
		candidate, err = p.resolveWikiAsset(decoded, ref.RelativePath)
		if err != nil {
			return ResolvedAsset{}, err
		}
	}
	return p.authorizeAsset(candidate)
}

func (p *Provider) resolveWikiAsset(reference, articleRelative string) (string, error) {
	if strings.Contains(filepath.ToSlash(reference), "/") {
		return filepath.Join(p.config.Root, filepath.FromSlash(reference)), nil
	}
	attachmentFolder := p.attachmentFolder()
	if attachmentFolder != "" && attachmentFolder != "." {
		candidate := filepath.Join(p.config.Root, filepath.FromSlash(attachmentFolder), reference)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	local := filepath.Join(p.config.Root, filepath.Dir(filepath.FromSlash(articleRelative)), reference)
	if info, err := os.Stat(local); err == nil && !info.IsDir() {
		return local, nil
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

func (p *Provider) attachmentFolder() string {
	content, err := os.ReadFile(filepath.Join(p.config.Root, ".obsidian", "app.json"))
	if err != nil {
		return ""
	}
	var config struct {
		AttachmentFolderPath string `json:"attachmentFolderPath"`
	}
	if json.Unmarshal(content, &config) != nil {
		return ""
	}
	return strings.TrimSpace(config.AttachmentFolderPath)
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
