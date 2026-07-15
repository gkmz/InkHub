package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	appjob "github.com/gkmz/InkHub/internal/app/job"
	domainpublication "github.com/gkmz/InkHub/internal/domain/publication"
	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
	"github.com/gkmz/InkHub/internal/provider/publish/wechat"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
	"go.uber.org/zap"
)

func newPublicationRunner(db *sql.DB, logger *zap.Logger) *appjob.Runner {
	runner := appjob.NewRunner(repository.NewJobRepository(db), appjob.Config{Workers: 2, PollInterval: 200 * time.Millisecond, Logger: logger})
	handler := publicationJobHandler{db: db, publications: repository.NewPublicationRepository(db)}
	runner.Register("hugo_sync", appjob.HandlerOptions{Handle: handler.handle, MaxAttempts: 1})
	runner.Register("wechat_prepare", appjob.HandlerOptions{Handle: handler.handle, MaxAttempts: 3, RetrySafe: true})
	return runner
}

type publicationJobHandler struct {
	db           *sql.DB
	publications *repository.PublicationRepository
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
	var provider contracts.PublishProvider
	switch providerType {
	case "hugo":
		provider, err = h.buildHugo(ctx, payload.ProviderID, config)
	case "wechat":
		provider, err = h.buildWeChat(ctx, payload.ProviderID, config, &input)
	default:
		return "", fmt.Errorf("不支持的发布 Provider: %s", providerType)
	}
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
	if providerType == "hugo" {
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
	var root string
	if err := h.db.QueryRowContext(ctx, `SELECT root_path FROM sources WHERE id=?`, sourceID).Scan(&root); err != nil {
		return contracts.PublishInput{}, "", nil, err
	}
	source, err := obsidian.New(obsidian.Config{SourceID: sourceID, Root: root})
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
	return contracts.PublishInput{OperationID: operationID, Article: document.Article, Body: document.Body, ContentHash: contentHash}, providerType, []byte(config), nil
}

func (h publicationJobHandler) buildHugo(ctx context.Context, id string, config []byte) (contracts.PublishProvider, error) {
	var raw struct {
		Root    string `json:"root"`
		Staging string `json:"staging_root"`
	}
	if err := json.Unmarshal(config, &raw); err != nil {
		return nil, err
	}
	return hugo.NewFactory(hugo.CLIBuilder{}).Build(ctx, contracts.ProviderRef{ID: id, Type: contracts.ProviderHugo}, contracts.ConfigView{Data: config, AllowedRoots: []string{raw.Root, raw.Staging}}, nil)
}
func (h publicationJobHandler) buildWeChat(ctx context.Context, id string, config []byte, input *contracts.PublishInput) (contracts.PublishProvider, error) {
	var raw struct {
		Staging  string `json:"staging_root"`
		Template string `json:"template"`
	}
	if err := json.Unmarshal(config, &raw); err != nil {
		return nil, err
	}
	validated, err := domaintemplate.Builtin(templateID(raw.Template))
	if err != nil {
		return nil, err
	}
	input.TemplateRef = &contracts.TemplateRef{ID: validated.Manifest.ID, Version: validated.Manifest.Version, Digest: validated.Digest}
	return wechat.New(wechat.Config{StagingRoot: raw.Staging}, builtinTemplateLoader{}, nil, unusedClipboard{})
}
func templateID(value string) string {
	if value == "minimal" {
		return domaintemplate.BuiltinMinimalID
	}
	return domaintemplate.BuiltinDefaultID
}

type builtinTemplateLoader struct{}

func (builtinTemplateLoader) Load(_ context.Context, ref contracts.TemplateRef) (domaintemplate.Validated, error) {
	return domaintemplate.Builtin(ref.ID)
}

type unusedClipboard struct{}

func (unusedClipboard) CopyHTML(context.Context, string) error {
	return fmt.Errorf("后台任务不允许写入剪贴板")
}
