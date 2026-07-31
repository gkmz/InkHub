package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gkmz/InkHub/internal/app/publication"
	"github.com/gkmz/InkHub/internal/domain/article"
	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// wechatPlanAPI 装配当前工作区解析、只读图片规划和确定性入队。
type wechatPlanAPI struct {
	db      *sql.DB
	runtime contracts.ProviderRuntime
	service *publication.WeChatPlanService
}

func newWeChatPlanAPI(db *sql.DB, runtime contracts.ProviderRuntime, key []byte) (*wechatPlanAPI, error) {
	api := &wechatPlanAPI{db: db, runtime: runtime}
	service, err := publication.NewWeChatPlanService(api, jobQueueAdapter{repository: repository.NewJobRepository(db)}, key, nil)
	if err != nil {
		return nil, err
	}
	api.service = service
	return api, nil
}

// Plan 返回当前文章的只读微信准备计划。
func (api *wechatPlanAPI) Plan(ctx context.Context, articleID, templateID string) (publication.WeChatPlanView, error) {
	return api.service.Plan(ctx, articleID, templateID)
}

// Confirm 校验计划后创建微信准备任务。
func (api *wechatPlanAPI) Confirm(ctx context.Context, articleID, token string) (string, error) {
	return api.service.Confirm(ctx, articleID, token)
}

// ResolveWeChatPlan 解析最近工作区中已审核的当前文章和微信 Provider。
func (api *wechatPlanAPI) ResolveWeChatPlan(ctx context.Context, articleID, templateID string) (publication.WeChatPlanArticle, error) {
	var workspaceID, providerID, contentHash, approvedHash, stage string
	err := api.db.QueryRowContext(ctx, `SELECT articles.workspace_id,provider_instances.id,articles.content_hash,COALESCE(editorial_reviews.approved_content_hash,''),articles.content_stage
FROM articles
JOIN workspaces ON workspaces.id=articles.workspace_id
JOIN provider_instances ON provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='wechat' AND provider_instances.enabled=1
LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id
WHERE articles.id=? AND articles.deleted_at IS NULL
	  AND workspaces.id=(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)`, articleID).Scan(&workspaceID, &providerID, &contentHash, &approvedHash, &stage)
	if article.ContentStage(stage) != article.ContentStageReady {
		return publication.WeChatPlanArticle{}, publication.ErrArticleNotReady
	}
	if err != nil || approvedHash == "" || approvedHash != contentHash {
		return publication.WeChatPlanArticle{}, fmt.Errorf("微信文章未找到或尚未审核")
	}
	validated, err := domaintemplate.Builtin(templateIDValue(templateID))
	if err != nil {
		return publication.WeChatPlanArticle{}, fmt.Errorf("微信模板无效")
	}
	handler := publicationJobHandler{db: api.db, runtime: api.runtime}
	input, providerType, config, err := handler.loadInput(ctx, stableAPIID("wechat_plan", articleID, contentHash, validated.Digest), publicationPayload{ArticleID: articleID, ProviderID: providerID, ContentHash: contentHash})
	if err != nil || providerType != string(contracts.ProviderWeChat) {
		return publication.WeChatPlanArticle{}, fmt.Errorf("读取微信文章失败")
	}
	input.TemplateRef = &contracts.TemplateRef{ID: validated.Manifest.ID, Version: validated.Manifest.Version, Digest: validated.Digest, Target: validated.Manifest.Target}
	view, err := providerConfigView(config)
	if err != nil {
		return publication.WeChatPlanArticle{}, err
	}
	provider, err := api.runtime.BuildPublish(ctx, contracts.ProviderRef{ID: providerID, Type: contracts.ProviderWeChat}, view)
	if err != nil {
		return publication.WeChatPlanArticle{}, err
	}
	return publication.WeChatPlanArticle{
		WorkspaceID: workspaceID, ArticleID: articleID, ProviderID: providerID, ContentHash: contentHash,
		ContentStage: article.ContentStage(stage),
		TemplateID:   validated.Manifest.ID, TemplateRevision: validated.Digest, Input: input, Provider: provider,
	}, nil
}

func templateIDValue(value string) string {
	if value == "" || value == "default" || value == string(domaintemplate.BuiltinDefaultID) {
		return domaintemplate.BuiltinDefaultID
	}
	if value == "minimal" || value == string(domaintemplate.BuiltinMinimalID) {
		return domaintemplate.BuiltinMinimalID
	}
	return value
}
