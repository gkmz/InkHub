package bootstrap

import (
	"context"
	"database/sql"
	"sort"

	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

const dashboardQuery = `SELECT articles.id,articles.title,articles.relative_path,articles.category,
COALESCE(articles.source_mtime,articles.updated_at),COALESCE(editorial_reviews.state,'pending_review'),
CASE WHEN hugo_publication.id IS NULL THEN 'never' WHEN hugo_publication.content_hash<>articles.content_hash THEN 'outdated' ELSE hugo_publication.state END,
CASE WHEN wechat_publication.id IS NULL THEN 'never' WHEN wechat_publication.content_hash<>articles.content_hash THEN 'outdated' ELSE wechat_publication.state END,
articles.content_hash,
CASE WHEN article_dispositions.cleared_at IS NULL
  AND (article_dispositions.kind='ignored' OR (article_dispositions.kind='published' AND article_dispositions.content_hash=articles.content_hash))
  THEN article_dispositions.kind ELSE '' END,
editorial_reviews.approved_content_hash,editorial_reviews.updated_at,
article_dispositions.kind,article_dispositions.content_hash,article_dispositions.cleared_at,article_dispositions.updated_at,
hugo_publication.state,hugo_publication.content_hash,wechat_publication.state,wechat_publication.content_hash
FROM articles
LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id
LEFT JOIN article_dispositions ON article_dispositions.article_id=articles.id AND article_dispositions.workspace_id=articles.workspace_id
LEFT JOIN provider_instances AS hugo_provider ON hugo_provider.workspace_id=articles.workspace_id AND hugo_provider.provider_type='hugo'
LEFT JOIN publications AS hugo_publication ON hugo_publication.article_id=articles.id AND hugo_publication.provider_instance_id=hugo_provider.id
LEFT JOIN provider_instances AS wechat_provider ON wechat_provider.workspace_id=articles.workspace_id AND wechat_provider.provider_type='wechat'
LEFT JOIN publications AS wechat_publication ON wechat_publication.article_id=articles.id AND wechat_publication.provider_instance_id=wechat_provider.id
WHERE articles.deleted_at IS NULL AND articles.workspace_id=` + currentWorkspaceSQL + `
ORDER BY COALESCE(articles.source_mtime,articles.updated_at) DESC,articles.id DESC`

type dashboardArticleRow struct {
	summary            httptransport.ArticleSummary
	reviewApprovedHash sql.NullString
	reviewUpdatedAt    sql.NullString
	dispositionKind    sql.NullString
	dispositionHash    sql.NullString
	dispositionCleared sql.NullString
	dispositionUpdated sql.NullString
	hugoState          sql.NullString
	hugoHash           sql.NullString
	wechatState        sql.NullString
	wechatHash         sql.NullString
}

type handledArticle struct {
	summary   httptransport.ArticleSummary
	handledAt string
}

// Dashboard 返回当前工作区按固定优先级分类的工作台视图。
func (api databaseAPI) Dashboard(ctx context.Context) (httptransport.DashboardView, error) {
	view := httptransport.DashboardView{
		Failed:          []httptransport.ArticleSummary{},
		Changed:         []httptransport.ArticleSummary{},
		NeedsReview:     []httptransport.ArticleSummary{},
		RecentlyHandled: []httptransport.ArticleSummary{},
	}
	rows, err := api.db.QueryContext(ctx, dashboardQuery)
	if err != nil {
		return view, err
	}
	defer rows.Close()

	recent := make([]handledArticle, 0, 10)
	for rows.Next() {
		row, scanErr := scanDashboardArticle(rows)
		if scanErr != nil {
			return view, scanErr
		}
		// 分类顺序是产品契约，确保失败或更新不会被“已发表”状态遮蔽。
		switch {
		case row.ignored():
			continue
		case row.currentFailure():
			view.Failed = append(view.Failed, row.summary)
		case row.contentChanged():
			view.Changed = append(view.Changed, row.summary)
		case row.summary.Disposition == "published":
			recent = append(recent, handledArticle{summary: row.summary, handledAt: row.dispositionUpdated.String})
		case needsReview(row.summary.State):
			view.NeedsReview = append(view.NeedsReview, row.summary)
		case row.currentApproved():
			recent = append(recent, handledArticle{summary: row.summary, handledAt: row.reviewUpdatedAt.String})
		}
	}
	if err := rows.Err(); err != nil {
		return view, err
	}

	sort.SliceStable(recent, func(left, right int) bool {
		if recent[left].handledAt == recent[right].handledAt {
			return recent[left].summary.ID > recent[right].summary.ID
		}
		return recent[left].handledAt > recent[right].handledAt
	})
	if len(recent) > 10 {
		recent = recent[:10]
	}
	for _, item := range recent {
		view.RecentlyHandled = append(view.RecentlyHandled, item.summary)
	}
	return view, nil
}

func scanDashboardArticle(rows interface{ Scan(...any) error }) (dashboardArticleRow, error) {
	var row dashboardArticleRow
	var relative, hugoDisplayState, wechatDisplayState string
	err := rows.Scan(
		&row.summary.ID, &row.summary.Title, &relative, &row.summary.Category,
		&row.summary.ModifiedAt, &row.summary.State, &hugoDisplayState, &wechatDisplayState,
		&row.summary.ContentVersion, &row.summary.Disposition,
		&row.reviewApprovedHash, &row.reviewUpdatedAt,
		&row.dispositionKind, &row.dispositionHash, &row.dispositionCleared, &row.dispositionUpdated,
		&row.hugoState, &row.hugoHash, &row.wechatState, &row.wechatHash,
	)
	if err != nil {
		return dashboardArticleRow{}, err
	}
	row.summary = finalizeArticleSummary(row.summary, relative, hugoDisplayState, wechatDisplayState)
	return row, nil
}

func (row dashboardArticleRow) ignored() bool {
	return row.dispositionKind.String == "ignored" && !row.dispositionCleared.Valid
}

func (row dashboardArticleRow) currentFailure() bool {
	return row.summary.State == "blocked" || publicationMatches(row.hugoState, row.hugoHash, "failed", row.summary.ContentVersion) || publicationMatches(row.wechatState, row.wechatHash, "failed", row.summary.ContentVersion)
}

func (row dashboardArticleRow) contentChanged() bool {
	stalePublished := row.dispositionKind.String == "published" && !row.dispositionCleared.Valid && row.dispositionHash.String != row.summary.ContentVersion
	staleReview := row.summary.State == "approved" && row.reviewApprovedHash.String != row.summary.ContentVersion
	return stalePublished || row.summary.State == "changed" || staleReview || publicationOutdated(row.hugoState, row.hugoHash, row.summary.ContentVersion) || publicationOutdated(row.wechatState, row.wechatHash, row.summary.ContentVersion)
}

func (row dashboardArticleRow) currentApproved() bool {
	return row.summary.State == "approved" && row.reviewApprovedHash.String == row.summary.ContentVersion
}

func publicationMatches(state, hash sql.NullString, wantState, currentHash string) bool {
	return state.Valid && state.String == wantState && hash.String == currentHash
}

func publicationOutdated(state, hash sql.NullString, currentHash string) bool {
	return state.Valid && hash.String != currentHash
}

func needsReview(state string) bool {
	switch state {
	case "draft", "incomplete", "pending_review":
		return true
	default:
		return false
	}
}
