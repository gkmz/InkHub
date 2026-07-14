package bootstrap

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	appjob "github.com/gkmz/InkHub/internal/app/job"
	"github.com/gkmz/InkHub/internal/app/publication"
	"github.com/gkmz/InkHub/internal/domain/article"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

type databaseAPI struct {
	db           *sql.DB
	publications *publication.Service
}

func newDatabaseAPI(db *sql.DB) databaseAPI {
	return databaseAPI{db: db, publications: publication.NewService(jobQueueAdapter{repository.NewJobRepository(db)}, publicationStoreAdapter{repository.NewPublicationRepository(db)})}
}

func (api databaseAPI) ListArticles(ctx context.Context, _ string, limit int) (httptransport.ArticlePage, error) {
	rows, err := api.db.QueryContext(ctx, `SELECT articles.id,articles.title,articles.relative_path,articles.category,COALESCE(articles.source_mtime,articles.updated_at),COALESCE(editorial_reviews.state,'pending_review'),COALESCE((SELECT CASE WHEN publications.content_hash<>articles.content_hash THEN 'outdated' ELSE publications.state END FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id WHERE publications.article_id=articles.id AND provider_instances.provider_type='hugo' LIMIT 1),'never'),COALESCE((SELECT CASE WHEN publications.content_hash<>articles.content_hash THEN 'outdated' ELSE publications.state END FROM publications JOIN provider_instances ON provider_instances.id=publications.provider_instance_id WHERE publications.article_id=articles.id AND provider_instances.provider_type='wechat' LIMIT 1),'never') FROM articles LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id WHERE articles.deleted_at IS NULL ORDER BY COALESCE(articles.source_mtime,articles.updated_at) DESC,articles.id LIMIT ?`, limit)
	if err != nil {
		return httptransport.ArticlePage{}, err
	}
	defer rows.Close()
	page := httptransport.ArticlePage{Items: []httptransport.ArticleSummary{}}
	for rows.Next() {
		var item httptransport.ArticleSummary
		var relative string
		var hugoState, wechatState string
		if err := rows.Scan(&item.ID, &item.Title, &relative, &item.Category, &item.ModifiedAt, &item.State, &hugoState, &wechatState); err != nil {
			return page, err
		}
		item.Directory = filepath.ToSlash(filepath.Dir(relative))
		if item.Directory == "." {
			item.Directory = ""
		}
		item.HugoState, item.WeChatState = publicationLabel(hugoState, "hugo"), publicationLabel(wechatState, "wechat")
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

func publicationLabel(state, channel string) string {
	switch state {
	case "published":
		return "已同步"
	case "prepared":
		return "已准备"
	case "copied":
		return "已复制"
	case "confirmed":
		return "已确认草稿"
	case "failed":
		return "处理失败"
	case "outdated":
		if channel == "hugo" {
			return "需要同步"
		}
		return "草稿可能过期"
	}
	if channel == "hugo" {
		return "尚未同步"
	}
	return "尚未准备"
}

func (api databaseAPI) QueuePublication(ctx context.Context, command httptransport.PublicationCommand) (string, error) {
	var value article.Article
	var approved sql.NullString
	err := api.db.QueryRowContext(ctx, `SELECT articles.id,articles.workspace_id,articles.content_hash,editorial_reviews.approved_content_hash FROM articles LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id WHERE articles.id=?`, command.ArticleID).Scan(&value.ID, &value.WorkspaceID, &value.ContentHash, &approved)
	if err != nil {
		return "", httptransport.ErrNotFound
	}
	if command.ContentHash != value.ContentHash {
		return "", httptransport.ErrStaleContent
	}
	channel := publication.Channel(command.Channel)
	jobID := stableAPIID("job", command.ArticleID, command.ProviderInstanceID, command.ContentHash, command.Channel)
	id, err := api.publications.Queue(ctx, publication.QueueRequest{JobID: jobID, ProviderInstanceID: command.ProviderInstanceID, Channel: channel, Article: value, ApprovedContentHash: approved.String})
	if errors.Is(err, publication.ErrContentChanged) {
		return "", httptransport.ErrStaleContent
	}
	return id, err
}

func (api databaseAPI) ConfirmWeChat(ctx context.Context, command httptransport.ConfirmCommand) error {
	err := api.publications.ConfirmWeChatDraft(ctx, publication.ConfirmRequest{ArticleID: command.ArticleID, ProviderInstanceID: command.ProviderInstanceID, CurrentContentHash: command.ContentHash})
	if errors.Is(err, publication.ErrContentChanged) {
		return httptransport.ErrStaleContent
	}
	return err
}

func (api databaseAPI) MarkWeChatCopied(ctx context.Context, command httptransport.ConfirmCommand) error {
	err := api.publications.MarkWeChatCopied(ctx, publication.ConfirmRequest{ArticleID: command.ArticleID, ProviderInstanceID: command.ProviderInstanceID, CurrentContentHash: command.ContentHash})
	if errors.Is(err, publication.ErrContentChanged) {
		return httptransport.ErrStaleContent
	}
	return err
}

type jobQueueAdapter struct{ repository *repository.JobRepository }

func (a jobQueueAdapter) Enqueue(ctx context.Context, intent publication.JobIntent) (string, error) {
	payload, _ := json.Marshal(map[string]string{"article_id": intent.ArticleID, "provider_instance_id": intent.ProviderInstanceID, "content_hash": intent.ContentHash})
	value, _, err := a.repository.Enqueue(ctx, domainjob.Job{ID: intent.ID, WorkspaceID: intent.WorkspaceID, Kind: intent.Kind, DedupeKey: appjob.BuildDedupeKey(intent.Kind, intent.ArticleID, intent.ProviderInstanceID, intent.ContentHash), PayloadJSON: string(payload), AvailableAt: time.Now().UTC()})
	return value.ID, err
}

type publicationStoreAdapter struct {
	repository *repository.PublicationRepository
}

func (a publicationStoreAdapter) Find(ctx context.Context, articleID, providerID string) (publication.Record, error) {
	value, err := a.repository.Find(ctx, articleID, providerID)
	return publication.Record{ID: value.ID, WorkspaceID: value.WorkspaceID, ArticleID: value.ArticleID, ProviderInstanceID: value.ProviderInstanceID, State: value.State, ContentHash: value.ContentHash}, err
}
func (a publicationStoreAdapter) SaveWithEvent(ctx context.Context, value publication.Record, eventType string) error {
	return a.repository.SaveWithEvent(ctx, repository.PublicationRecord{ID: value.ID, WorkspaceID: value.WorkspaceID, ArticleID: value.ArticleID, ProviderInstanceID: value.ProviderInstanceID, State: value.State, ContentHash: value.ContentHash}, repository.PublicationEvent{ID: stableAPIID("event", value.ID, eventType, value.ContentHash), Type: eventType, ContentHash: value.ContentHash, Payload: map[string]string{"source": "user"}})
}

func stableAPIID(kind string, values ...string) string {
	encoded, _ := json.Marshal(values)
	sum := sha256.Sum256(encoded)
	return kind + "_" + hex.EncodeToString(sum[:12])
}
