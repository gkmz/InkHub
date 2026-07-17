package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gkmz/InkHub/internal/app/publication"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// hugoPreviewAPI 将预览应用服务适配到当前工作区的数据库和 Provider Runtime。
type hugoPreviewAPI struct {
	db      *sql.DB
	runtime contracts.ProviderRuntime
	service *publication.HugoPreviewService
}

func newHugoPreviewAPI(db *sql.DB, runtime contracts.ProviderRuntime) *hugoPreviewAPI {
	api := &hugoPreviewAPI{db: db, runtime: runtime}
	api.service = publication.NewHugoPreviewService(repository.NewJobRepository(db), api, time.Now)
	return api
}

// DiscoverSections 返回当前文章对应 Hugo Provider 的受控一级目录。
func (api *hugoPreviewAPI) DiscoverSections(ctx context.Context, articleID string) (contracts.SectionDiscovery, error) {
	article, err := api.ResolvePreviewArticle(ctx, articleID)
	if err != nil {
		return contracts.SectionDiscovery{}, err
	}
	var stableID string
	if err := api.db.QueryRowContext(ctx, `SELECT stable_id FROM articles WHERE id=? AND workspace_id=? AND deleted_at IS NULL`, article.ArticleID, article.WorkspaceID).Scan(&stableID); err != nil {
		return contracts.SectionDiscovery{}, err
	}
	provider, err := api.buildHugoProvider(ctx, article.WorkspaceID, article.ProviderID)
	if err != nil {
		return contracts.SectionDiscovery{}, err
	}
	sectionProvider, ok := provider.(contracts.SectionAwarePublishProvider)
	if !ok {
		return contracts.SectionDiscovery{}, fmt.Errorf("Hugo Provider 不支持 Section 发现")
	}
	return sectionProvider.DiscoverSections(ctx, stableID)
}

// Queue 创建当前内容版本的 Hugo 预览任务。
func (api *hugoPreviewAPI) Queue(ctx context.Context, request publication.PreviewRequest) (domainjob.Job, error) {
	discovery, err := api.DiscoverSections(ctx, request.ArticleID)
	if err != nil {
		return domainjob.Job{}, err
	}
	if discovery.SelectionLocked {
		if request.Section != discovery.ExistingSection {
			return domainjob.Job{}, fmt.Errorf("文章必须继续发布到原 Hugo Section")
		}
	} else if !discoveryContainsSection(discovery, request.Section) {
		return domainjob.Job{}, fmt.Errorf("请选择扫描到的 Hugo Section")
	}
	return api.service.Queue(ctx, request)
}

// Find 返回不包含 Provider 私有路径的预览视图。
func (api *hugoPreviewAPI) Find(ctx context.Context, previewID string) (publication.PreviewView, error) {
	if err := api.requireCurrentWorkspacePreview(ctx, previewID); err != nil {
		return publication.PreviewView{}, err
	}
	return api.service.Find(ctx, previewID)
}

// Confirm 校验 Artifact 后创建确定性交付任务。
func (api *hugoPreviewAPI) Confirm(ctx context.Context, request publication.ConfirmPreviewRequest) (domainjob.Job, error) {
	if err := api.requireCurrentWorkspacePreview(ctx, request.PreviewID); err != nil {
		return domainjob.Job{}, err
	}
	return api.service.Confirm(ctx, request)
}

// ResolvePreviewArticle 解析当前工作区文章和唯一启用的 Hugo Provider。
func (api *hugoPreviewAPI) ResolvePreviewArticle(ctx context.Context, articleID string) (publication.PreviewArticle, error) {
	var value publication.PreviewArticle
	err := api.db.QueryRowContext(ctx, `SELECT articles.id,articles.workspace_id,provider_instances.id,articles.content_hash
FROM articles
JOIN workspaces ON workspaces.id=articles.workspace_id
JOIN provider_instances ON provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='hugo' AND provider_instances.enabled=1
WHERE articles.id=? AND articles.deleted_at IS NULL AND workspaces.id=(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)`, articleID).Scan(&value.ArticleID, &value.WorkspaceID, &value.ProviderID, &value.ContentHash)
	return value, err
}

// ValidatePreviewArtifact 委托同一 Hugo Provider 校验 staging manifest 和路径边界。
func (api *hugoPreviewAPI) ValidatePreviewArtifact(ctx context.Context, result publication.HugoPreviewResult) error {
	provider, err := api.buildHugoProvider(ctx, result.WorkspaceID, result.ProviderID)
	if err != nil {
		return err
	}
	validator, ok := provider.(contracts.PreparedArtifactValidator)
	if !ok {
		return fmt.Errorf("Hugo Provider 不支持 Artifact 校验")
	}
	return validator.ValidatePreparedArtifact(ctx, result.Artifact)
}

func (api *hugoPreviewAPI) buildHugoProvider(ctx context.Context, workspaceID, providerID string) (contracts.PublishProvider, error) {
	var providerType, configJSON string
	err := api.db.QueryRowContext(ctx, `SELECT provider_type,config_json FROM provider_instances WHERE id=? AND workspace_id=? AND provider_type='hugo' AND enabled=1`, providerID, workspaceID).Scan(&providerType, &configJSON)
	if err != nil {
		return nil, err
	}
	view, err := providerConfigView([]byte(configJSON))
	if err != nil {
		return nil, err
	}
	return api.runtime.BuildPublish(ctx, contracts.ProviderRef{ID: providerID, Type: contracts.ProviderType(providerType)}, view)
}

func (api *hugoPreviewAPI) requireCurrentWorkspacePreview(ctx context.Context, previewID string) error {
	var exists int
	return api.db.QueryRowContext(ctx, `SELECT 1 FROM jobs JOIN workspaces ON workspaces.id=jobs.workspace_id
WHERE jobs.id=? AND jobs.kind='hugo_preview' AND workspaces.id=(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)`, previewID).Scan(&exists)
}

func discoveryContainsSection(discovery contracts.SectionDiscovery, name string) bool {
	for _, section := range discovery.Sections {
		if section.Name == name {
			return true
		}
	}
	return false
}
