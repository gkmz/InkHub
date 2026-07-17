package publication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
)

// PreviewArticle 是预览服务执行身份和版本校验所需的最小文章视图。
type PreviewArticle struct {
	ArticleID   string
	WorkspaceID string
	ProviderID  string
	ContentHash string
}

// PreviewRequest 描述一次 Hugo Artifact 准备请求。
type PreviewRequest struct {
	ArticleID   string
	ContentHash string
	Section     string
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

// PreviewView 是不包含 Provider 内部路径的 Hugo 预览视图。
type PreviewView struct {
	ID, ArticleID, ContentHash, Section, TargetPath, Change string
	Files                                                   []PreviewFile
	Diagnostics                                             []PreviewDiagnostic
	PreviewURL                                              string
	ExpiresAt                                               *time.Time
	State, JobID, Error                                     string
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
	if current.ContentHash != request.ContentHash {
		return domainjob.Job{}, ErrPreviewStale
	}
	id := previewID(current.ArticleID, current.ProviderID, request.ContentHash, request.Section)
	payload, _ := json.Marshal(map[string]string{"preview_id": id, "article_id": current.ArticleID, "provider_instance_id": current.ProviderID, "content_hash": request.ContentHash, "section": request.Section})
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
	if current.WorkspaceID != result.WorkspaceID || current.ProviderID != result.ProviderID || current.ContentHash != result.Artifact.ContentHash {
		return domainjob.Job{}, ErrPreviewStale
	}
	if result.Artifact.ExpiresAt == nil || !result.Artifact.ExpiresAt.After(s.now()) {
		return domainjob.Job{}, ErrPreviewExpired
	}
	if err := s.dependencies.ValidatePreviewArtifact(ctx, result); err != nil {
		return domainjob.Job{}, err
	}
	id := deliveryID(result.PreviewID)
	payload, _ := json.Marshal(map[string]string{"preview_id": result.PreviewID, "article_id": result.ArticleID, "provider_instance_id": result.ProviderID, "content_hash": result.Artifact.ContentHash})
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

func previewID(articleID, providerID, hash, section string) string {
	return stablePreviewID("preview", articleID, providerID, hash, section)
}
func deliveryID(preview string) string { return stablePreviewID("delivery", preview) }
func stablePreviewID(prefix string, values ...string) string {
	encoded, _ := json.Marshal(values)
	sum := sha256.Sum256(encoded)
	return prefix + "_" + hex.EncodeToString(sum[:12])
}
