package bootstrap

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	appdisposition "github.com/gkmz/InkHub/internal/app/disposition"
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
	dispositions *appdisposition.Service
}

func newDatabaseAPI(db *sql.DB) databaseAPI {
	return databaseAPI{
		db:           db,
		publications: publication.NewService(jobQueueAdapter{repository.NewJobRepository(db)}, publicationStoreAdapter{repository.NewPublicationRepository(db)}),
		dispositions: newDispositionService(db),
	}
}

// BatchDisposition 将 HTTP DTO 转换为 Application 命令并映射稳定错误。
func (api databaseAPI) BatchDisposition(ctx context.Context, command httptransport.BatchDispositionCommand) (httptransport.BatchDispositionResult, error) {
	articles := make([]appdisposition.ArticleVersion, 0, len(command.Articles))
	for _, article := range command.Articles {
		articles = append(articles, appdisposition.ArticleVersion{ID: article.ID, ContentVersion: article.ContentVersion})
	}
	result, err := api.dispositions.Apply(ctx, appdisposition.Command{Operation: appdisposition.Operation(command.Operation), Articles: articles, Channels: command.Channels})
	if err != nil {
		switch {
		case errors.Is(err, appdisposition.ErrContentChanged):
			return httptransport.BatchDispositionResult{}, httptransport.ErrDispositionContentChanged
		case errors.Is(err, appdisposition.ErrArticleNotFound):
			return httptransport.BatchDispositionResult{}, httptransport.ErrNotFound
		case errors.Is(err, appdisposition.ErrChannelUnavailable):
			return httptransport.BatchDispositionResult{}, httptransport.ErrDispositionChannelUnavailable
		case errors.Is(err, appdisposition.ErrInvalidCommand):
			return httptransport.BatchDispositionResult{}, httptransport.ErrDispositionInvalid
		default:
			return httptransport.BatchDispositionResult{}, err
		}
	}
	return httptransport.BatchDispositionResult{Processed: result.Processed, Changed: result.Changed, Unchanged: result.Unchanged}, nil
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
	var stage, reviewState string
	// Provider 必须启用且属于文章工作区，不能信任客户端提交的渠道或实例组合。
	err := api.db.QueryRowContext(ctx, `SELECT articles.id,articles.workspace_id,articles.stable_id,articles.content_hash,articles.content_stage,COALESCE(editorial_reviews.state,''),editorial_reviews.approved_content_hash FROM articles JOIN provider_instances ON provider_instances.id=? AND provider_instances.workspace_id=articles.workspace_id AND provider_instances.enabled=1 LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id WHERE articles.id=?`, command.ProviderInstanceID, command.ArticleID).Scan(&value.ID, &value.WorkspaceID, &value.StableID, &value.ContentHash, &stage, &reviewState, &approved)
	if err != nil {
		return "", httptransport.ErrNotFound
	}
	if command.ContentHash != value.ContentHash {
		return "", httptransport.ErrStaleContent
	}
	value.ContentStage = article.ContentStage(stage)
	if value.ContentStage != article.ContentStageReady {
		return "", httptransport.ErrArticleNotReady
	}
	if value.StableID.Validate() != nil || reviewState != "approved" || approved.String != value.ContentHash {
		return "", httptransport.ErrReviewRequired
	}
	jobID := stableAPIID("job", command.ArticleID, command.ProviderInstanceID, command.ContentHash)
	id, err := api.publications.Queue(ctx, publication.QueueRequest{JobID: jobID, ProviderInstanceID: command.ProviderInstanceID, Article: value, ApprovedContentHash: approved.String})
	if errors.Is(err, publication.ErrContentChanged) {
		return "", httptransport.ErrStaleContent
	}
	if errors.Is(err, publication.ErrArticleNotReady) {
		return "", httptransport.ErrArticleNotReady
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
	payload, _ := json.Marshal(map[string]string{"article_id": intent.ArticleID, "provider_instance_id": intent.ProviderInstanceID, "content_hash": intent.ContentHash, "mermaid_theme": intent.MermaidTheme})
	dedupeContent := intent.ContentHash
	// Mermaid 样式属于微信准备结果的一部分，但不能改变其他渠道的既有去重键。
	if intent.MermaidTheme != "" {
		dedupeContent += "\x00" + intent.MermaidTheme
	}
	// 微信准备任务使用确定性 ID：重复点击时复用活动任务，失败任务则原子重排。
	if intent.Kind == "wechat_prepare" {
		if existing, findErr := a.repository.FindByID(ctx, intent.ID); findErr == nil && existing.WorkspaceID == intent.WorkspaceID && existing.Kind == intent.Kind {
			switch existing.State {
			case domainjob.StateQueued, domainjob.StateRunning, domainjob.StateSucceeded:
				return existing.ID, nil
			case domainjob.StateFailed:
				requeued, requeueErr := a.repository.RequeueFailed(ctx, existing.ID, intent.WorkspaceID, intent.Kind, time.Now().UTC())
				if requeueErr == nil {
					return requeued.ID, nil
				}
				return "", requeueErr
			}
		}
	}
	value, _, err := a.repository.Enqueue(ctx, domainjob.Job{ID: intent.ID, WorkspaceID: intent.WorkspaceID, Kind: intent.Kind, DedupeKey: appjob.BuildDedupeKey(intent.Kind, intent.ArticleID, intent.ProviderInstanceID, dedupeContent), PayloadJSON: string(payload), AvailableAt: time.Now().UTC()})
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
