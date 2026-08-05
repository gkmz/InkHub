package publication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

var (
	// ErrPreviewStale 表示文章内容版本已经不再匹配预览。
	ErrPreviewStale = errors.New("Hugo 发布预览对应的文章版本已变化")
	// ErrPreviewExpired 表示准备产物已经超过有效期。
	ErrPreviewExpired = errors.New("Hugo 发布预览已过期")
	// ErrPreviewNotReady 表示准备任务尚未成功生成 Artifact。
	ErrPreviewNotReady = errors.New("Hugo 发布预览尚未准备完成")
	// ErrPreviewInvalid 表示持久化的预览身份或产物信息不可信。
	ErrPreviewInvalid = errors.New("Hugo 发布预览数据无效")
	// ErrReviewRequired 表示文章当前版本尚未审核通过。
	ErrReviewRequired = errors.New("文章需要重新审核")
)

// PreviewArticle 是预览服务执行身份和版本校验所需的最小文章视图。
type PreviewArticle struct {
	ArticleID      string
	WorkspaceID    string
	ProviderID     string
	StableID       article.StableID
	ContentHash    string
	ContentStage   article.ContentStage
	ReviewApproved bool
}

// PreviewRequest 描述一次 Hugo Artifact 准备请求。
type PreviewRequest struct {
	ArticleID   string
	ContentHash string
	Section     string
	Directory   string
	// RefreshKey 用于真实 Hugo 目录发生外部变化后绕过旧的确定性预览任务。
	RefreshKey string
}

// ConfirmPreviewRequest 只引用服务端已经准备的 Preview。
type ConfirmPreviewRequest struct{ PreviewID string }

// HugoPreviewResult 是 Job result_json 中保存的完整服务端结果。
type HugoPreviewResult struct {
	PreviewID   string                     `json:"preview_id"`
	ArticleID   string                     `json:"article_id"`
	WorkspaceID string                     `json:"workspace_id"`
	ProviderID  string                     `json:"provider_instance_id"`
	Section     string                     `json:"section"`
	Artifact    contracts.PreparedArtifact `json:"artifact"`
	Diagnostics []contracts.Diagnostic     `json:"diagnostics"`
}

// PreviewFile 是浏览器可见的脱敏文件摘要。
type PreviewFile struct {
	RelativePath, MediaType string
	Size                    int64
}

// PreviewDiagnostic 是浏览器可见的检查结果。
type PreviewDiagnostic struct{ Code, Level, Message string }

// PublicationFailure 是浏览器可见且不包含内部路径的发布失败摘要。
type PublicationFailure struct {
	Stage     string `json:"stage"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Action    string `json:"action"`
	Retryable bool   `json:"retryable"`
}

// PreviewView 是不包含 Provider 内部路径的 Hugo 预览视图。
type PreviewView struct {
	ID, ArticleID, ContentHash, Section, TargetPath, Change string
	Files                                                   []PreviewFile
	Diagnostics                                             []PreviewDiagnostic
	PreviewURL                                              string
	ExpiresAt                                               *time.Time
	State, JobID, Error                                     string
	Failure                                                 *PublicationFailure
}

// HugoPreviewJobStore 是预览服务所需的最小持久化任务接口。
type HugoPreviewJobStore interface {
	Enqueue(ctx context.Context, value domainjob.Job) (domainjob.Job, bool, error)
	FindByID(ctx context.Context, id string) (domainjob.Job, error)
	RequeueFailed(ctx context.Context, id, workspaceID, kind string, now time.Time) (domainjob.Job, error)
}

// PreviewDependencies 解析当前文章并验证服务端 Artifact 是否仍可交付。
type PreviewDependencies interface {
	ResolvePreviewArticle(ctx context.Context, articleID string) (PreviewArticle, error)
	ValidatePreviewArtifact(ctx context.Context, result HugoPreviewResult) error
}

// HugoPreviewService 编排 Hugo Artifact 的准备、查询和确认交付。
type HugoPreviewService struct {
	jobs         HugoPreviewJobStore
	dependencies PreviewDependencies
	now          func() time.Time
}

// NewHugoPreviewService 创建不依赖 HTTP 或具体 Provider 的预览服务。
func NewHugoPreviewService(jobs HugoPreviewJobStore, dependencies PreviewDependencies, now func() time.Time) *HugoPreviewService {
	if now == nil {
		now = time.Now
	}
	return &HugoPreviewService{jobs: jobs, dependencies: dependencies, now: now}
}

// Queue 为当前文章版本创建确定性 Hugo 预览任务。
func (s *HugoPreviewService) Queue(ctx context.Context, request PreviewRequest) (domainjob.Job, error) {
	if s == nil || s.jobs == nil || s.dependencies == nil || request.ArticleID == "" || request.ContentHash == "" || request.Section == "" {
		return domainjob.Job{}, fmt.Errorf("Hugo 预览请求不完整")
	}
	current, err := s.dependencies.ResolvePreviewArticle(ctx, request.ArticleID)
	if err != nil {
		return domainjob.Job{}, err
	}
	if current.ArticleID != request.ArticleID || current.WorkspaceID == "" || current.ProviderID == "" {
		return domainjob.Job{}, ErrPreviewInvalid
	}
	if current.StableID.Validate() != nil {
		return domainjob.Job{}, ErrPreviewInvalid
	}
	if current.ContentStage != article.ContentStageReady {
		return domainjob.Job{}, ErrArticleNotReady
	}
	if !current.ReviewApproved {
		return domainjob.Job{}, ErrReviewRequired
	}
	if current.ContentHash != request.ContentHash {
		return domainjob.Job{}, ErrPreviewStale
	}
	id := previewID(current.ArticleID, current.ProviderID, request.ContentHash, request.Section, request.Directory, request.RefreshKey)
	payload, _ := json.Marshal(map[string]string{"preview_id": id, "article_id": current.ArticleID, "provider_instance_id": current.ProviderID, "content_hash": request.ContentHash, "section": request.Section, "directory": request.Directory, "refresh_key": request.RefreshKey})
	if existing, found, err := s.findDeterministicJob(ctx, id, current.WorkspaceID, "hugo_preview", string(payload)); found || err != nil {
		if err == nil && existing.State == domainjob.StateFailed {
			return s.jobs.RequeueFailed(ctx, id, current.WorkspaceID, "hugo_preview", s.now().UTC())
		}
		return existing, err
	}
	value, _, err := s.jobs.Enqueue(ctx, domainjob.Job{ID: id, WorkspaceID: current.WorkspaceID, Kind: "hugo_preview", DedupeKey: "hugo-preview:" + id, PayloadJSON: string(payload), AvailableAt: s.now().UTC()})
	return value, err
}

// Find 读取任务并转换为不泄露绝对路径的安全视图。
func (s *HugoPreviewService) Find(ctx context.Context, id string) (PreviewView, error) {
	job, err := s.jobs.FindByID(ctx, id)
	if err != nil {
		return PreviewView{}, err
	}
	view := PreviewView{ID: id, JobID: job.ID, State: previewState(job), Error: job.ErrorMessage}
	if job.State == domainjob.StateFailed || job.State == domainjob.StateCancelled {
		view.Failure = NewPublicationFailure(job.Kind, job.ErrorCode, job.ErrorMessage)
	}
	if job.State != domainjob.StateSucceeded {
		return view, nil
	}
	result, err := decodePreviewResult(job.ResultJSON, job.ID)
	if err != nil {
		return PreviewView{}, err
	}
	view.ArticleID, view.ContentHash, view.Section = result.ArticleID, result.Artifact.ContentHash, result.Section
	view.TargetPath, view.Change, view.PreviewURL, view.ExpiresAt = result.Artifact.TargetRelativePath, result.Artifact.Change, result.Artifact.PreviewURL, result.Artifact.ExpiresAt
	for _, file := range result.Artifact.Files {
		view.Files = append(view.Files, PreviewFile{RelativePath: file.RelativePath, MediaType: file.MediaType, Size: file.Size})
	}
	for _, diagnostic := range result.Diagnostics {
		level := "passed"
		if diagnostic.Blocking {
			level = "blocking"
		}
		view.Diagnostics = append(view.Diagnostics, PreviewDiagnostic{Code: diagnostic.Code, Level: level, Message: diagnostic.Message})
	}
	if view.ExpiresAt != nil && !view.ExpiresAt.After(s.now()) {
		view.State = "expired"
	}
	return view, nil
}

// NewPublicationFailure 将持久化任务错误转换为可恢复、可执行的安全失败说明。
func NewPublicationFailure(kind, code, message string) *PublicationFailure {
	if code == "" && message == "" {
		return nil
	}
	stage := "prepare"
	if kind == "hugo_deliver" {
		stage = "deliver"
	} else if strings.HasPrefix(code, "source.") || code == "hugo.preflight_failed" || code == "hugo.article_invalid" || code == "hugo.content_version_missing" || code == "hugo.stable_id_missing" || code == "hugo.stable_id_invalid" || code == "hugo.title_missing" || code == "hugo.operation_invalid" {
		stage = "preflight"
	}
	action := "检查发布历史和 Hugo 配置后重新生成预览"
	switch {
	case code == "source.image_unresolved":
		action = "修复文章中的图片引用后重新生成预览"
	case code == "hugo.article_invalid":
		action = "补充标题、稳定 ID 和内容版本后重新审核"
	case code == "hugo.content_version_missing":
		action = "刷新文章内容版本后重新审核"
	case code == "hugo.stable_id_missing":
		action = "返回审核页，保存元数据以生成稳定 ID 后重新审核"
	case code == "hugo.stable_id_invalid":
		action = "修复源文件中的稳定 ID 后重新审核"
	case code == "hugo.title_missing":
		action = "返回审核页，补充标题并保存后重新审核"
	case code == "hugo.operation_invalid":
		action = "重新打开 Hugo 发布页并生成预览"
	case code == "hugo.section_invalid" || code == "hugo.section_locked":
		action = "重新选择文章原有或有效的 Hugo 发布目录"
	case code == "hugo.build_failed":
		action = "修复 Hugo 构建错误后重新生成预览"
	case code == "hugo.config_invalid" || code == "hugo.content_unavailable" || code == "hugo.path_unauthorized":
		action = "检查设置中的 Hugo 路径和配置后重试"
	case stage == "deliver":
		action = "确认 Hugo 站点可写后重新同步"
	}
	if message == "" {
		message = "Hugo 发布操作失败"
	}
	return &PublicationFailure{Stage: stage, Code: code, Message: message, Action: action, Retryable: true}
}

// Confirm 校验预览仍有效后创建确定性交付任务。
func (s *HugoPreviewService) Confirm(ctx context.Context, request ConfirmPreviewRequest) (domainjob.Job, error) {
	if s == nil || s.jobs == nil || s.dependencies == nil || request.PreviewID == "" {
		return domainjob.Job{}, fmt.Errorf("Hugo 预览确认请求不完整")
	}
	preview, err := s.jobs.FindByID(ctx, request.PreviewID)
	if err != nil {
		return domainjob.Job{}, err
	}
	if preview.State != domainjob.StateSucceeded {
		return domainjob.Job{}, ErrPreviewNotReady
	}
	result, err := decodePreviewResult(preview.ResultJSON, preview.ID)
	if err != nil {
		return domainjob.Job{}, err
	}
	current, err := s.dependencies.ResolvePreviewArticle(ctx, result.ArticleID)
	if err != nil {
		return domainjob.Job{}, err
	}
	if current.StableID.Validate() != nil {
		return domainjob.Job{}, ErrPreviewInvalid
	}
	if !current.ReviewApproved {
		return domainjob.Job{}, ErrReviewRequired
	}
	if current.WorkspaceID != result.WorkspaceID || current.ProviderID != result.ProviderID || current.ContentHash != result.Artifact.ContentHash {
		return domainjob.Job{}, ErrPreviewStale
	}
	if current.ContentStage != article.ContentStageReady {
		return domainjob.Job{}, ErrArticleNotReady
	}
	id := deliveryID(result.PreviewID)
	payload, _ := json.Marshal(map[string]string{"preview_id": result.PreviewID, "article_id": result.ArticleID, "provider_instance_id": result.ProviderID, "content_hash": result.Artifact.ContentHash})
	// 已完成或进行中的确定性交付直接复用，不再依赖可能已清理的临时 Artifact。
	// 失败任务仍需经过下面的 Artifact 校验，确认 staging 内容可安全重试。
	if existing, found, err := s.findDeterministicJob(ctx, id, result.WorkspaceID, "hugo_deliver", string(payload)); found || err != nil {
		if err != nil {
			return domainjob.Job{}, err
		}
		if existing.State != domainjob.StateFailed {
			return existing, nil
		}
	}
	if result.Artifact.ExpiresAt == nil || !result.Artifact.ExpiresAt.After(s.now()) {
		return domainjob.Job{}, ErrPreviewExpired
	}
	if err := s.dependencies.ValidatePreviewArtifact(ctx, result); err != nil {
		return domainjob.Job{}, err
	}
	if existing, found, err := s.findDeterministicJob(ctx, id, result.WorkspaceID, "hugo_deliver", string(payload)); found || err != nil {
		if err == nil && existing.State == domainjob.StateFailed {
			return s.jobs.RequeueFailed(ctx, id, result.WorkspaceID, "hugo_deliver", s.now().UTC())
		}
		return existing, err
	}
	value, _, err := s.jobs.Enqueue(ctx, domainjob.Job{ID: id, WorkspaceID: result.WorkspaceID, Kind: "hugo_deliver", DedupeKey: "hugo-deliver:" + result.PreviewID, PayloadJSON: string(payload), AvailableAt: s.now().UTC()})
	return value, err
}

func (s *HugoPreviewService) findDeterministicJob(ctx context.Context, id, workspaceID, kind, payload string) (domainjob.Job, bool, error) {
	existing, err := s.jobs.FindByID(ctx, id)
	if err != nil {
		return domainjob.Job{}, false, nil
	}
	if existing.WorkspaceID != workspaceID || existing.Kind != kind || existing.PayloadJSON != payload {
		return domainjob.Job{}, true, ErrPreviewInvalid
	}
	return existing, true, nil
}

func decodePreviewResult(raw, expectedPreviewID string) (HugoPreviewResult, error) {
	var result HugoPreviewResult
	if json.Unmarshal([]byte(raw), &result) != nil || result.PreviewID != expectedPreviewID || result.Artifact.OperationID != expectedPreviewID {
		return HugoPreviewResult{}, ErrPreviewInvalid
	}
	return result, nil
}

func previewState(job domainjob.Job) string {
	switch job.State {
	case domainjob.StateQueued, domainjob.StateRunning:
		return "preparing"
	case domainjob.StateSucceeded:
		return "ready"
	case domainjob.StateFailed, domainjob.StateCancelled:
		return "failed"
	}
	return string(job.State)
}

func previewID(articleID, providerID, hash, section, directory, refreshKey string) string {
	return stablePreviewID("preview", articleID, providerID, hash, section, directory, refreshKey)
}
func deliveryID(preview string) string { return stablePreviewID("delivery", preview) }
func stablePreviewID(prefix string, values ...string) string {
	encoded, _ := json.Marshal(values)
	sum := sha256.Sum256(encoded)
	return prefix + "_" + hex.EncodeToString(sum[:12])
}
