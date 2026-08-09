// Package editorial 提供发布前的编辑加工能力，包括 Obsidian WikiLink 的跨渠道解析。
package editorial

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
)

// LinkStatus 描述一个内部链接在目标渠道中的处理结果。
type LinkStatus string

const (
	LinkStatusConverted   LinkStatus = "converted"
	LinkStatusUnpublished LinkStatus = "unpublished"
	LinkStatusMissing     LinkStatus = "missing"
	LinkStatusAmbiguous   LinkStatus = "ambiguous"
	LinkStatusUnavailable LinkStatus = "unavailable"
)

// LinkResolution 保存 WikiLink 目标的索引解析结果。
type LinkResolution struct {
	Label        string
	RelativePath string
	StableID     string
	Slug         string
	Found        bool
}

// LinkOutcome 保存一个 WikiLink 的渠道转换结果，供测试和后续预览报告使用。
type LinkOutcome struct {
	Target   string
	Label    string
	Status   LinkStatus
	Blocking bool
}

// LinkResult 同时返回转换后的正文、链接结果和发布预检诊断。
type LinkResult struct {
	Body        string
	Links       []LinkOutcome
	Diagnostics []contracts.Diagnostic
}

// LinkResolver 将 Obsidian WikiLink 目标解析为文章索引信息。
type LinkResolver interface {
	Resolve(ctx context.Context, wikiTarget string) (LinkResolution, error)
}

// AmbiguousLinkError 表示不带路径的 WikiLink 同时匹配到多篇文章。
type AmbiguousLinkError struct{ Target string }

// Error 返回不包含本机路径的歧义说明。
func (e *AmbiguousLinkError) Error() string {
	return fmt.Sprintf("WikiLink 目标不唯一: %s", e.Target)
}

// ArticleLinkResolver 基于 article 表查询的 LinkResolver 实现。
type ArticleLinkResolver struct {
	db          *sql.DB
	workspaceID string
}

// NewArticleLinkResolver 创建按工作区限定查询范围的链接解析器。
func NewArticleLinkResolver(db *sql.DB, workspaceID string) *ArticleLinkResolver {
	return &ArticleLinkResolver{db: db, workspaceID: workspaceID}
}

// Resolve 按 Obsidian 目标路径或唯一文件名匹配文章，歧义目标不会静默任选一篇。
func (r *ArticleLinkResolver) Resolve(ctx context.Context, wikiTarget string) (LinkResolution, error) {
	target := linkDocumentTarget(wikiTarget)
	if target == "" || r == nil || r.db == nil || r.workspaceID == "" {
		return LinkResolution{Label: target}, nil
	}
	normalized := filepath.ToSlash(strings.TrimSuffix(target, ".md")) + ".md"
	query := `SELECT stable_id,slug,relative_path FROM articles WHERE workspace_id=? AND deleted_at IS NULL AND relative_path=?`
	args := []any{r.workspaceID, normalized}
	if !strings.Contains(normalized, "/") {
		query = `SELECT stable_id,slug,relative_path FROM articles WHERE workspace_id=? AND deleted_at IS NULL AND (relative_path=? OR relative_path LIKE ?)`
		args = append(args, "%/"+normalized)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return LinkResolution{}, err
	}
	defer rows.Close()
	matches := make([]LinkResolution, 0, 1)
	for rows.Next() {
		var resolution LinkResolution
		if err := rows.Scan(&resolution.StableID, &resolution.Slug, &resolution.RelativePath); err != nil {
			return LinkResolution{}, err
		}
		resolution.Label, resolution.Found = target, true
		matches = append(matches, resolution)
	}
	if err := rows.Err(); err != nil {
		return LinkResolution{}, err
	}
	if len(matches) == 0 {
		return LinkResolution{Label: target}, nil
	}
	if len(matches) > 1 {
		return LinkResolution{}, &AmbiguousLinkError{Target: target}
	}
	return matches[0], nil
}

// ProcessWikiLinks 只转换 Markdown 正文文本中的 WikiLink，跳过代码、已有链接和图片。
func ProcessWikiLinks(body string, resolve func(target, alias string) string) string {
	result := rewriteWikiLinks(body, func(target, alias string) linkReplacement {
		return linkReplacement{Text: resolve(target, alias), Status: LinkStatusConverted}
	})
	return result.Body
}

// DefaultLabel 返回 WikiLink 的默认显示文本。
func DefaultLabel(target, alias string) string {
	if alias != "" {
		return alias
	}
	return target
}

type hugoProviderConfig struct {
	Root    string `json:"root"`
	BaseURL string `json:"base_url,omitempty"`
}

type linkReplacement struct {
	Text     string
	Status   LinkStatus
	Blocking bool
}

// ProcessHugoWikiLinks 将全文 WikiLink 转为 Hugo relref，并返回可供预检展示的诊断。
func ProcessHugoWikiLinks(ctx context.Context, resolver LinkResolver, body, configJSON string) LinkResult {
	var cfg hugoProviderConfig
	hasConfig := json.Unmarshal([]byte(configJSON), &cfg) == nil && cfg.Root != ""
	if hasConfig {
		if _, err := os.Stat(filepath.Join(cfg.Root, "content")); err != nil {
			hasConfig = false
		}
	}
	return rewriteWikiLinks(body, func(target, alias string) linkReplacement {
		label := markdownLabel(DefaultLabel(target, alias))
		resolution, status := resolveIndexedLink(ctx, resolver, target)
		if status != LinkStatusConverted {
			return linkReplacement{Text: label, Status: status, Blocking: status == LinkStatusAmbiguous}
		}
		if !hasConfig {
			return linkReplacement{Text: label, Status: LinkStatusUnavailable}
		}
		bundlePath, _, published, err := hugo.FindPublishedBundleBySourceID(cfg.Root, resolution.StableID)
		if err != nil {
			return linkReplacement{Text: label, Status: LinkStatusUnavailable, Blocking: true}
		}
		if !published {
			return linkReplacement{Text: label, Status: LinkStatusUnpublished}
		}
		indexPath := filepath.Join(bundlePath, "index.md")
		relPath, err := filepath.Rel(filepath.Join(cfg.Root, "content"), indexPath)
		if err != nil || strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			return linkReplacement{Text: label, Status: LinkStatusUnavailable, Blocking: true}
		}
		return linkReplacement{Text: fmt.Sprintf(`[%s]({{< relref %q >}})`, label, filepath.ToSlash(relPath)), Status: LinkStatusConverted}
	})
}

// ProcessWebWikiLinks 将全文 WikiLink 转为博客绝对链接；未发布目标退化为纯文本。
func ProcessWebWikiLinks(ctx context.Context, resolver LinkResolver, body string, db *sql.DB, workspaceID string) LinkResult {
	cfg := loadHugoLinkConfig(ctx, db, workspaceID)
	return rewriteWikiLinks(body, func(target, alias string) linkReplacement {
		label := markdownLabel(DefaultLabel(target, alias))
		resolution, status := resolveIndexedLink(ctx, resolver, target)
		if status != LinkStatusConverted {
			return linkReplacement{Text: label, Status: status, Blocking: status == LinkStatusAmbiguous}
		}
		if cfg.BaseURL == "" || cfg.Root == "" {
			return linkReplacement{Text: label, Status: LinkStatusUnavailable}
		}
		bundlePath, _, published, err := hugo.FindPublishedBundleBySourceID(cfg.Root, resolution.StableID)
		if err != nil {
			return linkReplacement{Text: label, Status: LinkStatusUnavailable, Blocking: true}
		}
		if !published {
			return linkReplacement{Text: label, Status: LinkStatusUnpublished}
		}
		publicURL, err := publishedArticleURL(cfg, bundlePath)
		if err != nil {
			return linkReplacement{Text: label, Status: LinkStatusUnavailable, Blocking: true}
		}
		return linkReplacement{Text: fmt.Sprintf("[%s](%s)", label, publicURL), Status: LinkStatusConverted}
	})
}
