package bootstrap

import (
	"context"
	"path/filepath"
	"strings"

	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

const currentWorkspaceSQL = `(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)`

// ListArticles 查询当前工作区中符合条件的内容库文章。
func (api databaseAPI) ListArticles(ctx context.Context, input httptransport.ArticleListQuery) (httptransport.ArticlePage, error) {
	page := httptransport.ArticlePage{
		Items:             []httptransport.ArticleSummary{},
		AvailableChannels: []string{},
	}
	channels, err := api.listAvailableChannels(ctx)
	if err != nil {
		return page, err
	}
	page.AvailableChannels = channels

	query, arguments, err := buildArticleListQuery(input)
	if err != nil {
		return httptransport.ArticlePage{}, err
	}
	rows, err := api.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return httptransport.ArticlePage{}, err
	}
	defer rows.Close()

	for rows.Next() {
		item, scanErr := scanArticleSummary(rows)
		if scanErr != nil {
			return page, scanErr
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > input.Limit {
		page.Items = page.Items[:input.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor, err = encodeArticleCursor(articleCursor{ModifiedAt: last.ModifiedAt, ID: last.ID})
		if err != nil {
			return httptransport.ArticlePage{}, err
		}
	}
	return page, nil
}

func buildArticleListQuery(input httptransport.ArticleListQuery) (string, []any, error) {
	query := `SELECT articles.id,articles.title,articles.relative_path,articles.category,
COALESCE(articles.source_mtime,articles.updated_at),COALESCE(editorial_reviews.state,'pending_review'),
COALESCE((SELECT CASE WHEN publications.content_hash<>articles.content_hash THEN 'outdated' ELSE publications.state END
  FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id
  WHERE publications.article_id=articles.id AND provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='hugo' LIMIT 1),'never'),
COALESCE((SELECT CASE WHEN publications.content_hash<>articles.content_hash THEN 'outdated' ELSE publications.state END
  FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id
  WHERE publications.article_id=articles.id AND provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='wechat' LIMIT 1),'never'),
articles.content_hash,
CASE WHEN article_dispositions.cleared_at IS NULL
  AND (article_dispositions.kind='ignored' OR (article_dispositions.kind='published' AND article_dispositions.content_hash=articles.content_hash))
  THEN article_dispositions.kind ELSE '' END
FROM articles
LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id
LEFT JOIN article_dispositions ON article_dispositions.article_id=articles.id AND article_dispositions.workspace_id=articles.workspace_id
WHERE articles.deleted_at IS NULL AND articles.workspace_id=` + currentWorkspaceSQL
	arguments := make([]any, 0, 8)

	if input.Search != "" {
		query += ` AND LOWER(articles.title) LIKE ? ESCAPE '\'`
		arguments = append(arguments, "%"+escapeLike(strings.ToLower(input.Search))+"%")
	}
	if input.State != "" {
		query += ` AND COALESCE(editorial_reviews.state,'pending_review')=?`
		arguments = append(arguments, input.State)
	}

	// 处置枚举只映射到固定 SQL 片段，客户端输入永远不会成为 SQL 结构。
	switch input.Disposition {
	case "ignored":
		query += ` AND article_dispositions.kind='ignored' AND article_dispositions.cleared_at IS NULL`
	case "published":
		query += ` AND article_dispositions.kind='published' AND article_dispositions.cleared_at IS NULL AND article_dispositions.content_hash=articles.content_hash`
	case "unresolved":
		query += ` AND NOT COALESCE(article_dispositions.kind='ignored' AND article_dispositions.cleared_at IS NULL,0)
AND NOT COALESCE(article_dispositions.kind='published' AND article_dispositions.cleared_at IS NULL AND article_dispositions.content_hash=articles.content_hash,0)`
	default:
		query += ` AND NOT COALESCE(article_dispositions.kind='ignored' AND article_dispositions.cleared_at IS NULL,0)`
	}

	if input.Cursor != "" {
		cursor, err := decodeArticleCursor(input.Cursor)
		if err != nil {
			return "", nil, httptransport.ErrInvalidCursor
		}
		// 时间和 ID 共同构成稳定 keyset，避免翻页期间新增文章造成 OFFSET 漂移。
		query += ` AND (COALESCE(articles.source_mtime,articles.updated_at) < ? OR (COALESCE(articles.source_mtime,articles.updated_at) = ? AND articles.id < ?))`
		arguments = append(arguments, cursor.ModifiedAt, cursor.ModifiedAt, cursor.ID)
	}
	query += ` ORDER BY COALESCE(articles.source_mtime,articles.updated_at) DESC,articles.id DESC LIMIT ?`
	arguments = append(arguments, input.Limit+1)
	return query, arguments, nil
}

func scanArticleSummary(rows interface{ Scan(...any) error }) (httptransport.ArticleSummary, error) {
	var item httptransport.ArticleSummary
	var relative, hugoState, wechatState string
	if err := rows.Scan(&item.ID, &item.Title, &relative, &item.Category, &item.ModifiedAt, &item.State, &hugoState, &wechatState, &item.ContentVersion, &item.Disposition); err != nil {
		return httptransport.ArticleSummary{}, err
	}
	return finalizeArticleSummary(item, relative, hugoState, wechatState), nil
}

func finalizeArticleSummary(item httptransport.ArticleSummary, relative, hugoState, wechatState string) httptransport.ArticleSummary {
	item.Directory = filepath.ToSlash(filepath.Dir(relative))
	if item.Directory == "." {
		item.Directory = ""
	}
	item.HugoState = publicationLabel(hugoState, "hugo")
	item.WeChatState = publicationLabel(wechatState, "wechat")
	return item
}

func (api databaseAPI) listAvailableChannels(ctx context.Context) ([]string, error) {
	rows, err := api.db.QueryContext(ctx, `SELECT provider_type FROM provider_instances
WHERE workspace_id=`+currentWorkspaceSQL+` AND enabled=1 AND provider_type IN ('hugo','wechat')
ORDER BY CASE provider_type WHEN 'hugo' THEN 1 ELSE 2 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []string{}
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
