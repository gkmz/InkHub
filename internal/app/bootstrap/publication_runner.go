package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	appjob "github.com/gkmz/InkHub/internal/app/job"
	domainpublication "github.com/gkmz/InkHub/internal/domain/publication"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
	"go.uber.org/zap"
)

func newPublicationRunner(db *sql.DB, logger *zap.Logger, runtime contracts.ProviderRuntime) *appjob.Runner {
	runner := appjob.NewRunner(repository.NewJobRepository(db), appjob.Config{Workers: 2, PollInterval: 200 * time.Millisecond, Logger: logger})
	handler := publicationJobHandler{db: db, publications: repository.NewPublicationRepository(db), runtime: runtime}
	runner.Register("publication", appjob.HandlerOptions{Handle: handler.handle, MaxAttempts: 3, RetrySafe: true})
	// 保留旧任务类型，仅用于升级后恢复已经持久化的任务。
	runner.Register("hugo_sync", appjob.HandlerOptions{Handle: handler.handle, MaxAttempts: 1})
	runner.Register("wechat_prepare", appjob.HandlerOptions{Handle: handler.handle, MaxAttempts: 3, RetrySafe: true})
	return runner
}

type publicationJobHandler struct {
	db           *sql.DB
	publications *repository.PublicationRepository
	runtime      contracts.ProviderRuntime
}
type publicationPayload struct {
	ArticleID   string `json:"article_id"`
	ProviderID  string `json:"provider_instance_id"`
	ContentHash string `json:"content_hash"`
}

func (h publicationJobHandler) handle(ctx context.Context, execution *appjob.Execution) (string, error) {
	var payload publicationPayload
	if err := json.Unmarshal([]byte(execution.Job.PayloadJSON), &payload); err != nil {
		return "", fmt.Errorf("解析发布任务: %w", err)
	}
	input, providerType, config, err := h.loadInput(ctx, execution.Job.ID, payload)
	if err != nil {
		return "", err
	}
	view, err := providerConfigView(config)
	if err != nil {
		return "", err
	}
	provider, err := h.runtime.BuildPublish(ctx, contracts.ProviderRef{ID: payload.ProviderID, Type: contracts.ProviderType(providerType)}, view)
	if err != nil {
		return "", err
	}
	input.TemplateRef, err = configuredTemplate(config)
	if err != nil {
		return "", err
	}
	preflight, err := provider.Preflight(ctx, input)
	if err != nil {
		return "", err
	}
	if !preflight.Ready {
		return "", fmt.Errorf("发布前检查未通过")
	}
	if err := execution.ReportProgress(ctx, 35); err != nil {
		return "", err
	}
	artifact, err := provider.Prepare(ctx, input)
	if err != nil {
		return "", err
	}
	state := domainpublication.StatePrepared
	revision := artifact.ProviderRevision
	if provider.Descriptor().DeliveryMode == contracts.DeliveryAutomatic {
		if err := execution.ReportProgress(ctx, 70); err != nil {
			return "", err
		}
		result, deliverErr := provider.Deliver(ctx, artifact)
		if deliverErr != nil {
			return "", deliverErr
		}
		state = domainpublication.StatePublished
		revision = result.ProviderRevision
	}
	recordID := stableAPIID("publication", payload.ArticleID, payload.ProviderID)
	eventType := string(state)
	err = h.publications.SaveWithEvent(ctx, repository.PublicationRecord{ID: recordID, ArticleID: payload.ArticleID, ProviderInstanceID: payload.ProviderID, WorkspaceID: execution.Job.WorkspaceID, State: state, ContentHash: payload.ContentHash, ProviderRevision: revision}, repository.PublicationEvent{ID: stableAPIID("event", recordID, eventType, payload.ContentHash), Type: eventType, ContentHash: payload.ContentHash, Payload: map[string]string{"job": execution.Job.ID}})
	if err != nil {
		return "", err
	}
	result, _ := json.Marshal(map[string]string{"state": eventType, "location": artifact.Location})
	return string(result), nil
}

func (h publicationJobHandler) loadInput(ctx context.Context, operationID string, payload publicationPayload) (contracts.PublishInput, string, []byte, error) {
	var workspaceID, sourceID, stableID, relative, contentHash, providerType, config string
	err := h.db.QueryRowContext(ctx, `SELECT articles.workspace_id,articles.source_id,articles.stable_id,articles.relative_path,articles.content_hash,provider_instances.provider_type,provider_instances.config_json FROM articles JOIN provider_instances ON provider_instances.id=? AND provider_instances.workspace_id=articles.workspace_id WHERE articles.id=?`, payload.ProviderID, payload.ArticleID).Scan(&workspaceID, &sourceID, &stableID, &relative, &contentHash, &providerType, &config)
	if err != nil {
		return contracts.PublishInput{}, "", nil, err
	}
	if contentHash != payload.ContentHash {
		return contracts.PublishInput{}, "", nil, fmt.Errorf("文章内容版本已变化")
	}
	var sourceType, root, sourceConfig string
	if err := h.db.QueryRowContext(ctx, `SELECT provider_type,root_path,config_json FROM sources WHERE id=?`, sourceID).Scan(&sourceType, &root, &sourceConfig); err != nil {
		return contracts.PublishInput{}, "", nil, err
	}
	view, err := sourceConfigView(root, []byte(sourceConfig))
	if err != nil {
		return contracts.PublishInput{}, "", nil, err
	}
	source, err := h.runtime.BuildSource(ctx, contracts.ProviderRef{ID: sourceID, Type: contracts.ProviderType(sourceType)}, view)
	if err != nil {
		return contracts.PublishInput{}, "", nil, err
	}
	document, err := source.Read(ctx, contracts.SourceRef{SourceID: sourceID, RelativePath: relative, StableID: stableID})
	if err != nil {
		return contracts.PublishInput{}, "", nil, err
	}
	document.Article.WorkspaceID = workspaceID
	document.Article.ID = payload.ArticleID
	document.Article.ContentHash = contentHash
	return contracts.PublishInput{OperationID: operationID, Article: document.Article, Body: document.Body, ResourceRefs: document.ResourceRefs, ContentHash: contentHash}, providerType, []byte(config), nil
}
