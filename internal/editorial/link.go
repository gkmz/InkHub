// Package editorial 提供发布前的编辑加工能力，包括 Obsidian wiki 链接的跨渠道解析。
package editorial

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
)

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// LinkResolution 保存 wiki 链接目标的解析结果。
type LinkResolution struct {
	Label        string
	RelativePath string
	StableID     string
	Slug         string
	Found        bool
}

// LinkResolver 将 Obsidian wiki 链接目标解析为文章索引信息。
type LinkResolver interface {
	Resolve(ctx context.Context, wikiTarget string) (LinkResolution, error)
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
	target := strings.TrimSpace(wikiTarget)
	label := target
	if target == "" {
		return LinkResolution{Label: label}, nil
	}
	normalized := filepath.ToSlash(strings.TrimSuffix(target, ".md")) + ".md"
	query := `SELECT stable_id,slug,relative_path FROM articles WHERE workspace_id=? AND deleted_at IS NULL AND stable_id<>'' AND relative_path=?`
	args := []any{r.workspaceID, normalized}
	if !strings.Contains(normalized, "/") {
		query = `SELECT stable_id,slug,relative_path FROM articles WHERE workspace_id=? AND deleted_at IS NULL AND stable_id<>'' AND (relative_path=? OR relative_path LIKE ?)`
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
		resolution.Label, resolution.Found = label, true
		matches = append(matches, resolution)
	}
	if err := rows.Err(); err != nil {
		return LinkResolution{}, err
	}
	if len(matches) == 0 {
		return LinkResolution{Label: label}, nil
	}
	if len(matches) > 1 {
		return LinkResolution{}, fmt.Errorf("WikiLink 目标不唯一: %s", target)
	}
	return matches[0], nil
}

// ProcessWikiLinks 处理 body 中的 [[...]] wiki 链接（跳过 ![[...]] 图片嵌入）。
// resolve 回调接收 (target, alias)，返回最终的 markdown 片段。
func ProcessWikiLinks(body string, resolve func(target, alias string) string) string {
	matches := wikiLinkPattern.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return body
	}
	var result strings.Builder
	position := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		// 跳过 ![[...]] 图片嵌入
		if start > 0 && body[start-1] == '!' {
			continue
		}
		result.WriteString(body[position:start])
		target := strings.TrimSpace(body[match[2]:match[3]])
		var alias string
		if match[4] >= 0 {
			alias = strings.TrimSpace(body[match[4]:match[5]])
		}
		result.WriteString(resolve(target, alias))
		position = end
	}
	result.WriteString(body[position:])
	return result.String()
}

// DefaultLabel 返回 wiki 链接的默认显示文本。
func DefaultLabel(target, alias string) string {
	if alias != "" {
		return alias
	}
	return target
}

// hugoProviderConfig 是 Hugo Provider 配置 JSON 的子集，用于读取 root 和 base_url。
type hugoProviderConfig struct {
	Root    string `json:"root"`
	BaseURL string `json:"base_url,omitempty"`
}

// ProcessHugoWikiLinks 将 wiki 链接转为 Hugo relref 内部链接。
// 未发布或 Hugo 站点未找到目标时，保留纯文本 label。
// 当 Hugo 配置不可用时，退化为纯文本 label，避免 Hugo 因无法识别 wiki 语法而构建失败。
func ProcessHugoWikiLinks(ctx context.Context, resolver LinkResolver, body, configJSON string) string {
	var cfg hugoProviderConfig
	hasConfig := json.Unmarshal([]byte(configJSON), &cfg) == nil && cfg.Root != ""
	if hasConfig {
		if _, err := os.Stat(filepath.Join(cfg.Root, "content")); err != nil {
			hasConfig = false
		}
	}
	if !hasConfig {
		return ProcessWikiLinks(body, func(target, alias string) string {
			return DefaultLabel(target, alias)
		})
	}
	contentRoot := filepath.Join(cfg.Root, "content")
	return ProcessWikiLinks(body, func(target, alias string) string {
		label := DefaultLabel(target, alias)
		resolution, err := resolver.Resolve(ctx, target)
		if err != nil || !resolution.Found {
			return label
		}
		bundlePath, _, found, err := hugo.FindBundleBySourceID(cfg.Root, resolution.StableID)
		if err != nil || !found {
			return label
		}
		indexPath := filepath.Join(bundlePath, "index.md")
		if _, statErr := os.Stat(indexPath); statErr != nil {
			return label
		}
		relPath, err := filepath.Rel(contentRoot, indexPath)
		if err != nil {
			return label
		}
		if strings.HasPrefix(relPath, "..") || strings.HasPrefix(relPath, "/") {
			return label
		}
		return fmt.Sprintf(`[%s]({{< relref %q >}})`, label, filepath.ToSlash(relPath))
	})
}

// ProcessWebWikiLinks 将 wiki 链接转为指向 Hugo 博客的外部 markdown 链接。
// 微信公众号和小红书使用相同的处理逻辑。未发布时保留纯文本 label。
func ProcessWebWikiLinks(ctx context.Context, resolver LinkResolver, body string, db *sql.DB, workspaceID string) string {
	config := loadHugoLinkConfig(ctx, db, workspaceID)
	if config.BaseURL == "" || config.Root == "" {
		return body
	}
	return ProcessWikiLinks(body, func(target, alias string) string {
		label := DefaultLabel(target, alias)
		resolution, err := resolver.Resolve(ctx, target)
		if err != nil || !resolution.Found {
			return label
		}
		bundlePath, _, found, err := hugo.FindBundleBySourceID(config.Root, resolution.StableID)
		if err != nil || !found {
			return label
		}
		relative, err := filepath.Rel(filepath.Join(config.Root, "content"), bundlePath)
		if err != nil || strings.HasPrefix(relative, "..") {
			return label
		}
		return fmt.Sprintf(`[%s](%s/%s/)`, label, strings.TrimRight(config.BaseURL, "/"), strings.Trim(filepath.ToSlash(relative), "/"))
	})
}

// loadHugoLinkConfig 从已启用 Hugo Provider 读取公开链接所需配置。
func loadHugoLinkConfig(ctx context.Context, db *sql.DB, workspaceID string) hugoProviderConfig {
	var configJSON string
	err := db.QueryRowContext(ctx,
		`SELECT config_json FROM provider_instances WHERE workspace_id=? AND provider_type='hugo' AND enabled=1`,
		workspaceID,
	).Scan(&configJSON)
	if err != nil {
		return hugoProviderConfig{}
	}
	var cfg hugoProviderConfig
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return hugoProviderConfig{}
	}
	return cfg
}
