package bootstrap

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/domain/article"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

const currentWorkspaceSQL = `(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)`

// articleStageRankSQL 将作者明确标记的已就绪文章排在草稿前面。
const articleStageRankSQL = `CASE WHEN articles.content_stage='ready' THEN 0 ELSE 1 END`

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
		item, scanErr := scanArticleSummary(rows, channels)
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
		page.NextCursor, err = encodeArticleCursor(articleCursor{ContentStage: last.ContentStage, ModifiedAt: last.ModifiedAt, ID: last.ID})
		if err != nil {
			return httptransport.ArticlePage{}, err
		}
	}
	return page, nil
}

func buildArticleListQuery(input httptransport.ArticleListQuery) (string, []any, error) {
	query := `SELECT articles.id,articles.title,articles.relative_path,articles.category,
COALESCE(articles.source_mtime,articles.updated_at),articles.content_stage,articles.content_stage_issue,
COALESCE(editorial_reviews.state,'pending_review'),COALESCE(editorial_reviews.approved_content_hash,''),
COALESCE((SELECT publications.state
  FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id
  WHERE publications.article_id=articles.id AND provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='hugo' LIMIT 1),'never'),
COALESCE((SELECT publications.content_hash
  FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id
  WHERE publications.article_id=articles.id AND provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='hugo' LIMIT 1),''),
COALESCE((SELECT publications.state
  FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id
  WHERE publications.article_id=articles.id AND provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='wechat' LIMIT 1),'never'),
COALESCE((SELECT publications.content_hash
  FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id
  WHERE publications.article_id=articles.id AND provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='wechat' LIMIT 1),''),
COALESCE((SELECT xiaohongshu_drafts.state FROM xiaohongshu_drafts
  WHERE xiaohongshu_drafts.article_id=articles.id ORDER BY xiaohongshu_drafts.created_at DESC,xiaohongshu_drafts.id DESC LIMIT 1),'never'),
COALESCE((SELECT xiaohongshu_drafts.source_content_hash FROM xiaohongshu_drafts
  WHERE xiaohongshu_drafts.article_id=articles.id ORDER BY xiaohongshu_drafts.created_at DESC,xiaohongshu_drafts.id DESC LIMIT 1),''),
articles.content_hash,
COALESCE(article_dispositions.kind,''),COALESCE(article_dispositions.content_hash,''),
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
		if input.State == "draft" {
			query += ` AND articles.content_stage='draft'`
		} else {
			query += ` AND articles.content_stage='ready' AND COALESCE(editorial_reviews.state,'pending_review')=?`
			arguments = append(arguments, input.State)
		}
	}
	if input.ContentStage != "" {
		query += ` AND articles.content_stage=?`
		arguments = append(arguments, input.ContentStage)
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
		// 阶段、时间和 ID 共同构成稳定 keyset，避免跨阶段分页时漏掉草稿或重复文章。
		query += ` AND (` + articleStageRankSQL + ` > ? OR (` + articleStageRankSQL + ` = ? AND COALESCE(articles.source_mtime,articles.updated_at) < ?) OR (` + articleStageRankSQL + ` = ? AND COALESCE(articles.source_mtime,articles.updated_at) = ? AND articles.id < ?))`
		stageRank := 1
		if cursor.ContentStage == "ready" {
			stageRank = 0
		}
		arguments = append(arguments, stageRank, stageRank, cursor.ModifiedAt, stageRank, cursor.ModifiedAt, cursor.ID)
	}
	query += ` ORDER BY ` + articleStageRankSQL + ` ASC,COALESCE(articles.source_mtime,articles.updated_at) DESC,articles.id DESC LIMIT ?`
	arguments = append(arguments, input.Limit+1)
	return query, arguments, nil
}

func scanArticleSummary(rows interface{ Scan(...any) error }, channels []string) (httptransport.ArticleSummary, error) {
	var item httptransport.ArticleSummary
	var relative, stage, issue, reviewState, approvedHash, hugoState, hugoHash, wechatState, wechatHash, xiaohongshuState, xiaohongshuHash, dispositionKind, dispositionHash string
	if err := rows.Scan(&item.ID, &item.Title, &relative, &item.Category, &item.ModifiedAt, &stage, &issue, &reviewState, &approvedHash, &hugoState, &hugoHash, &wechatState, &wechatHash, &xiaohongshuState, &xiaohongshuHash, &item.ContentVersion, &dispositionKind, &dispositionHash, &item.Disposition); err != nil {
		return httptransport.ArticleSummary{}, err
	}
	item.ContentStage = stage
	item.ContentStageIssue = issue
	item.XiaohongshuState = xiaohongshuLabel(xiaohongshuState, xiaohongshuHash, item.ContentVersion)
	workflow := deriveArticleWorkflow(articleWorkflowInput{ContentStage: article.ContentStage(stage), ContentIssue: issue, ReviewState: reviewState, ApprovedHash: approvedHash, ContentHash: item.ContentVersion, Disposition: dispositionKind, DispositionHash: dispositionHash, HugoState: hugoState, HugoHash: hugoHash, WeChatState: wechatState, WeChatHash: wechatHash, AvailableChannels: channels})
	return finalizeArticleSummary(item, relative, workflow), nil
}

func finalizeArticleSummary(item httptransport.ArticleSummary, relative string, workflow articleWorkflowResult) httptransport.ArticleSummary {
	item.Directory = filepath.ToSlash(filepath.Dir(relative))
	if item.Directory == "." {
		item.Directory = ""
	}
	item.State = workflow.State
	item.HugoState = workflow.HugoLabel
	item.WeChatState = workflow.WeChatLabel
	item.NextAction = workflow.NextAction
	return item
}

// xiaohongshuLabel 将小红书草稿版本与文章当前版本转换为用户可读状态。
func xiaohongshuLabel(state, processedHash, currentHash string) string {
	if state == "published" && processedHash == currentHash {
		return "已发布"
	}
	if state != "" && state != "never" && processedHash != currentHash {
		return "内容已更新"
	}
	if state == "draft" {
		return "草稿"
	}
	return "尚未准备"
}

func (api databaseAPI) listAvailableChannels(ctx context.Context) ([]string, error) {
	rows, err := api.db.QueryContext(ctx, `SELECT provider_type FROM provider_instances
WHERE workspace_id=`+currentWorkspaceSQL+` AND enabled=1 AND provider_type IN ('hugo','wechat')
		ORDER BY CASE provider_type WHEN 'hugo' THEN 1 WHEN 'wechat' THEN 2 ELSE 3 END`)
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
