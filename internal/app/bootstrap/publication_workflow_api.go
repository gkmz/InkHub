package bootstrap

import (
	"context"
	"database/sql"

	"github.com/gkmz/InkHub/internal/app/publication"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// publicationWorkflowAPI 装配当前工作区解析、Job 查询和安全 Preview 视图。
type publicationWorkflowAPI struct {
	db       *sql.DB
	hugo     *hugoPreviewAPI
	jobs     *repository.JobRepository
	events   *repository.PublicationRepository
	workflow *publication.PublicationWorkflowService
	history  *publication.PublicationHistoryService
}

func newPublicationWorkflowAPI(db *sql.DB, hugo *hugoPreviewAPI) *publicationWorkflowAPI {
	api := &publicationWorkflowAPI{db: db, hugo: hugo, jobs: repository.NewJobRepository(db), events: repository.NewPublicationRepository(db)}
	api.workflow = publication.NewPublicationWorkflowService(api, api, hugo.service)
	api.history = publication.NewPublicationHistoryService(api, api)
	return api
}

// History 返回当前文章的跨渠道发布历史。
func (api *publicationWorkflowAPI) History(ctx context.Context, articleID, cursor string, limit int) (publication.HistoryPage, error) {
	return api.history.History(ctx, articleID, cursor, limit)
}

// Find 返回当前文章版本的安全发布恢复视图。
func (api *publicationWorkflowAPI) Find(ctx context.Context, articleID string) (publication.WorkflowView, error) {
	return api.workflow.Find(ctx, articleID)
}

// ResolveWorkflowArticle 按最近工作区解析文章与启用的 Hugo Provider。
func (api *publicationWorkflowAPI) ResolveWorkflowArticle(ctx context.Context, articleID string) (publication.WorkflowArticle, error) {
	resolved, err := api.hugo.ResolvePreviewArticle(ctx, articleID)
	if err != nil {
		return publication.WorkflowArticle{}, err
	}
	return publication.WorkflowArticle{ArticleID: resolved.ArticleID, WorkspaceID: resolved.WorkspaceID, ProviderID: resolved.ProviderID, ContentHash: resolved.ContentHash}, nil
}

// ResolveHistoryArticle 按最近工作区解析渠道无关的文章身份。
func (api *publicationWorkflowAPI) ResolveHistoryArticle(ctx context.Context, articleID string) (publication.HistoryArticle, error) {
	var article publication.HistoryArticle
	err := api.db.QueryRowContext(ctx, `SELECT articles.id,articles.workspace_id FROM articles
JOIN workspaces ON workspaces.id=articles.workspace_id
WHERE articles.id=? AND articles.deleted_at IS NULL
  AND workspaces.id=(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)`, articleID).Scan(&article.ArticleID, &article.WorkspaceID)
	return article, err
}

// FindLatestTargetJob 查询当前业务身份下最新的有限 Job 快照。
func (api *publicationWorkflowAPI) FindLatestTargetJob(ctx context.Context, workspaceID, articleID, providerID, contentHash, kind string) (domainjob.Job, bool, error) {
	return api.jobs.FindLatestTargetJob(ctx, workspaceID, articleID, providerID, contentHash, kind)
}

// ListPublicationEvents 将 SQLite 事件页转换为 Application 读模型。
func (api *publicationWorkflowAPI) ListPublicationEvents(ctx context.Context, workspaceID, articleID string, cursor publication.HistoryEventCursor, limit int) (publication.HistoryEventPage, error) {
	stored, err := api.events.ListEvents(ctx, workspaceID, articleID, repository.PublicationEventCursor{CreatedAt: cursor.CreatedAt, ID: cursor.ID}, limit)
	if err != nil {
		return publication.HistoryEventPage{}, err
	}
	page := publication.HistoryEventPage{Items: make([]publication.HistoryEvent, 0, len(stored.Items)), HasMore: stored.HasMore}
	for _, item := range stored.Items {
		page.Items = append(page.Items, publication.HistoryEvent{ID: item.ID, ProviderType: item.ProviderType, Type: item.Type, PayloadJSON: item.PayloadJSON, CreatedAt: item.CreatedAt})
	}
	return page, nil
}
