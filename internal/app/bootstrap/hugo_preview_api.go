package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/app/publication"
	"github.com/gkmz/InkHub/internal/domain/article"
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
	if article.StableID.Validate() != nil {
		return contracts.SectionDiscovery{}, publication.ErrPreviewInvalid
	}
	if !article.ReviewApproved {
		return contracts.SectionDiscovery{}, publication.ErrReviewRequired
	}
	provider, err := api.buildHugoProvider(ctx, article.WorkspaceID, article.ProviderID)
	if err != nil {
		return contracts.SectionDiscovery{}, err
	}
	sectionProvider, ok := provider.(contracts.SectionAwarePublishProvider)
	if !ok {
		return contracts.SectionDiscovery{}, fmt.Errorf("Hugo Provider 不支持 Section 发现")
	}
	return sectionProvider.DiscoverSections(ctx, string(article.StableID))
}

// Queue 创建当前内容版本的 Hugo 预览任务。
func (api *hugoPreviewAPI) Queue(ctx context.Context, request publication.PreviewRequest) (domainjob.Job, error) {
	discovery, err := api.DiscoverSections(ctx, request.ArticleID)
	if err != nil {
		return domainjob.Job{}, err
	}
	if discovery.SelectionLocked {
		if request.Section != discovery.ExistingSection || request.Directory != discovery.ExistingDirectory {
			return domainjob.Job{}, fmt.Errorf("文章必须继续发布到原 Hugo Section")
		}
	} else if !discoveryContainsSection(discovery, request.Section) {
		return domainjob.Job{}, fmt.Errorf("请选择扫描到的 Hugo Section")
	} else if request.Directory != "" && !discoveryContainsDirectory(discovery, request.Section, request.Directory) {
		return domainjob.Job{}, fmt.Errorf("请选择扫描到的 Hugo 分类目录")
	}
	// 数据库中的“已同步”只能作为历史投影；若真实 Hugo Bundle 已被删除，强制生成新的预览任务。
	if !discovery.SelectionLocked && api.publishedRecordMissingBundle(ctx, request.ArticleID, request.ContentHash) {
		request.RefreshKey = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return api.service.Queue(ctx, request)
}

// publishedRecordMissingBundle 判断已记录的 Hugo 发布是否只剩数据库记录而没有真实 Bundle。
func (api *hugoPreviewAPI) publishedRecordMissingBundle(ctx context.Context, articleID, contentHash string) bool {
	article, err := api.ResolvePreviewArticle(ctx, articleID)
	if err != nil || article.ContentHash != contentHash {
		return false
	}
	var state, storedHash string
	err = api.db.QueryRowContext(ctx, `SELECT publications.state,publications.content_hash
FROM publications
WHERE publications.article_id=? AND publications.provider_instance_id=?`, articleID, article.ProviderID).Scan(&state, &storedHash)
	return err == nil && state == "published" && storedHash == contentHash
}

func discoveryContainsDirectory(discovery contracts.SectionDiscovery, sectionName, directoryPath string) bool {
	for _, section := range discovery.Sections {
		if section.Name != sectionName {
			continue
		}
		for _, directory := range section.Directories {
			if directory.Path == directoryPath {
				return true
			}
		}
	}
	return false
}

// Find 返回不包含 Provider 私有路径的预览视图。
func (api *hugoPreviewAPI) Find(ctx context.Context, previewID string) (publication.PreviewView, error) {
	if err := api.requireCurrentWorkspacePreview(ctx, previewID); err != nil {
		return publication.PreviewView{}, err
	}
	return api.service.Find(ctx, previewID)
}

// ResolveRenderFile 校验当前文章版本和 Artifact 后解析单篇预览文件。
func (api *hugoPreviewAPI) ResolveRenderFile(ctx context.Context, previewID, resourcePath string) (publication.PreviewRenderFile, error) {
	if err := api.requireCurrentWorkspacePreview(ctx, previewID); err != nil {
		return publication.PreviewRenderFile{}, err
	}
	result, err := api.service.FindResult(ctx, previewID)
	if err != nil {
		return publication.PreviewRenderFile{}, err
	}
	current, err := api.ResolvePreviewArticle(ctx, result.ArticleID)
	if err != nil {
		return publication.PreviewRenderFile{}, err
	}
	if current.WorkspaceID != result.WorkspaceID || current.ProviderID != result.ProviderID || current.ContentHash != result.Artifact.ContentHash {
		return publication.PreviewRenderFile{}, publication.ErrPreviewStale
	}
	if result.Artifact.PreviewPath == "" {
		return publication.PreviewRenderFile{}, publication.ErrPreviewRenderNotFound
	}
	if err := api.ValidatePreviewArtifact(ctx, result); err != nil {
		return publication.PreviewRenderFile{}, err
	}
	publicRoot, err := previewPublicRoot(result.Artifact)
	if err != nil {
		return publication.PreviewRenderFile{}, err
	}
	requested, err := normalizePreviewResourcePath(resourcePath, result.Artifact.PreviewPath)
	if err != nil {
		return publication.PreviewRenderFile{}, err
	}
	// 预览端点只暴露当前文章 HTML；样式、图片和字体可以从同一构建输出加载。
	if strings.EqualFold(path.Ext(requested), ".html") && requested != result.Artifact.PreviewPath {
		return publication.PreviewRenderFile{}, publication.ErrPreviewRenderNotFound
	}
	absolute := filepath.Join(publicRoot, filepath.FromSlash(requested))
	if !withinPreviewRoot(absolute, publicRoot) {
		return publication.PreviewRenderFile{}, publication.ErrPreviewRenderNotFound
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return publication.PreviewRenderFile{}, publication.ErrPreviewRenderNotFound
	}
	mediaType := mime.TypeByExtension(filepath.Ext(absolute))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return publication.PreviewRenderFile{AbsolutePath: absolute, MediaType: mediaType}, nil
}

func previewPublicRoot(artifact contracts.PreparedArtifact) (string, error) {
	relativeTarget := filepath.Clean(filepath.FromSlash(artifact.TargetRelativePath))
	if relativeTarget == "." || filepath.IsAbs(relativeTarget) || strings.HasPrefix(relativeTarget, "..") {
		return "", publication.ErrPreviewInvalid
	}
	stagedSite := filepath.Clean(artifact.Location)
	for range strings.Split(filepath.ToSlash(relativeTarget), "/") {
		stagedSite = filepath.Dir(stagedSite)
	}
	if filepath.Join(stagedSite, relativeTarget) != filepath.Clean(artifact.Location) {
		return "", publication.ErrPreviewInvalid
	}
	return filepath.Join(stagedSite, "public"), nil
}

func normalizePreviewResourcePath(resourcePath, previewPath string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(resourcePath, `\`, "/"))
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", publication.ErrPreviewRenderNotFound
		}
	}
	if value == "" {
		value = previewPath
	} else if strings.HasSuffix(value, "/") {
		value += "index.html"
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+value), "/")
	if cleaned == "" || cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", publication.ErrPreviewRenderNotFound
	}
	return cleaned, nil
}

func withinPreviewRoot(candidate, root string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
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
	var stage, reviewState, approvedHash string
	err := api.db.QueryRowContext(ctx, `SELECT articles.id,articles.workspace_id,provider_instances.id,articles.stable_id,articles.content_hash,articles.content_stage,COALESCE(editorial_reviews.state,''),COALESCE(editorial_reviews.approved_content_hash,'')
FROM articles
JOIN workspaces ON workspaces.id=articles.workspace_id
JOIN provider_instances ON provider_instances.workspace_id=articles.workspace_id AND provider_instances.provider_type='hugo' AND provider_instances.enabled=1
	LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id
	WHERE articles.id=? AND articles.deleted_at IS NULL AND workspaces.id=(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)`, articleID).Scan(&value.ArticleID, &value.WorkspaceID, &value.ProviderID, &value.StableID, &value.ContentHash, &stage, &reviewState, &approvedHash)
	value.ContentStage = article.ContentStage(stage)
	value.ReviewApproved = reviewState == "approved" && approvedHash == value.ContentHash
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
