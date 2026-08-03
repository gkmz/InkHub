package publication

import (
	"context"
	"encoding/json"
	"fmt"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
)

// WorkflowArticle 是恢复发布流程所需的当前文章身份。
type WorkflowArticle struct {
	ArticleID   string
	WorkspaceID string
	ProviderID  string
	ContentHash string
}

// WorkflowResolver 按当前工作区解析文章和 Hugo Provider。
type WorkflowResolver interface {
	ResolveWorkflowArticle(ctx context.Context, articleID string) (WorkflowArticle, error)
}

// WorkflowJobStore 查询当前业务身份下最新的有限 Job 快照。
type WorkflowJobStore interface {
	FindLatestTargetJob(ctx context.Context, workspaceID, articleID, providerID, contentHash, kind string) (domainjob.Job, bool, error)
}

// WorkflowPreviewFinder 读取现有 Hugo Preview 的安全视图。
type WorkflowPreviewFinder interface {
	Find(ctx context.Context, previewID string) (PreviewView, error)
}

// DeliveryJobView 是页面可见的有限交付任务状态。
type DeliveryJobView struct {
	State    string
	Progress int
	Stage    string
	Error    string
	Failure  *PublicationFailure
}

// HugoWorkflowView 描述当前文章版本的 Hugo 发布流程。
type HugoWorkflowView struct {
	State    string
	Progress int
	Stage    string
	Error    string
	Failure  *PublicationFailure
	Preview  *PreviewView
	Delivery *DeliveryJobView
}

// WorkflowView 是文章级发布恢复视图。
type WorkflowView struct {
	ArticleID   string
	ContentHash string
	Hugo        *HugoWorkflowView
}

// PublicationWorkflowService 从持久化 Job 构造只读发布恢复视图。
type PublicationWorkflowService struct {
	resolver WorkflowResolver
	jobs     WorkflowJobStore
	previews WorkflowPreviewFinder
}

// NewPublicationWorkflowService 创建文章级发布恢复服务。
func NewPublicationWorkflowService(resolver WorkflowResolver, jobs WorkflowJobStore, previews WorkflowPreviewFinder) *PublicationWorkflowService {
	return &PublicationWorkflowService{resolver: resolver, jobs: jobs, previews: previews}
}

// Find 查询当前文章版本的 Hugo Preview/Deliver 状态。
func (s *PublicationWorkflowService) Find(ctx context.Context, articleID string) (WorkflowView, error) {
	if s == nil || s.resolver == nil || s.jobs == nil || s.previews == nil || articleID == "" {
		return WorkflowView{}, fmt.Errorf("发布工作流服务未正确配置")
	}
	article, err := s.resolver.ResolveWorkflowArticle(ctx, articleID)
	if err != nil {
		return WorkflowView{}, err
	}
	if article.ArticleID != articleID || article.WorkspaceID == "" || article.ProviderID == "" || article.ContentHash == "" {
		return WorkflowView{}, fmt.Errorf("发布工作流文章身份无效")
	}
	view := WorkflowView{ArticleID: article.ArticleID, ContentHash: article.ContentHash}
	delivery, found, err := s.jobs.FindLatestTargetJob(ctx, article.WorkspaceID, article.ArticleID, article.ProviderID, article.ContentHash, "hugo_deliver")
	if err != nil {
		return WorkflowView{}, err
	}
	if found {
		hugo, mapErr := s.deliveryView(ctx, delivery)
		if mapErr != nil {
			return WorkflowView{}, mapErr
		}
		view.Hugo = &hugo
		return view, nil
	}
	preview, found, err := s.jobs.FindLatestTargetJob(ctx, article.WorkspaceID, article.ArticleID, article.ProviderID, article.ContentHash, "hugo_preview")
	if err != nil || !found {
		return view, err
	}
	hugo, err := s.previewView(ctx, preview)
	if err != nil {
		return WorkflowView{}, err
	}
	view.Hugo = &hugo
	return view, nil
}

func (s *PublicationWorkflowService) deliveryView(ctx context.Context, job domainjob.Job) (HugoWorkflowView, error) {
	var payload struct {
		PreviewID string `json:"preview_id"`
	}
	if json.Unmarshal([]byte(job.PayloadJSON), &payload) != nil || payload.PreviewID == "" {
		return HugoWorkflowView{}, ErrPreviewInvalid
	}
	preview, err := s.previews.Find(ctx, payload.PreviewID)
	if err != nil {
		return HugoWorkflowView{}, err
	}
	delivery := DeliveryJobView{State: string(job.State), Progress: job.Progress, Stage: deliveryStage(job.Progress), Error: job.ErrorMessage}
	state := "delivering"
	if job.State == domainjob.StateFailed {
		state = "failed"
		delivery.Failure = NewPublicationFailure(job.Kind, job.ErrorCode, job.ErrorMessage)
	} else if job.State == domainjob.StateSucceeded {
		state = "published"
	}
	return HugoWorkflowView{State: state, Progress: job.Progress, Stage: delivery.Stage, Error: job.ErrorMessage, Failure: delivery.Failure, Preview: &preview, Delivery: &delivery}, nil
}

func (s *PublicationWorkflowService) previewView(ctx context.Context, job domainjob.Job) (HugoWorkflowView, error) {
	if job.State == domainjob.StateSucceeded {
		preview, err := s.previews.Find(ctx, job.ID)
		if err != nil {
			return HugoWorkflowView{}, err
		}
		return HugoWorkflowView{State: preview.State, Progress: job.Progress, Stage: previewStage(job.Progress), Error: preview.Error, Preview: &preview}, nil
	}
	state := "preparing"
	if job.State == domainjob.StateFailed || job.State == domainjob.StateCancelled {
		state = "failed"
	}
	return HugoWorkflowView{State: state, Progress: job.Progress, Stage: previewStage(job.Progress), Error: job.ErrorMessage, Failure: NewPublicationFailure(job.Kind, job.ErrorCode, job.ErrorMessage)}, nil
}

func previewStage(progress int) string {
	switch {
	case progress < 20:
		return "正在加载文章"
	case progress < 45:
		return "正在执行发布检查"
	case progress < 85:
		return "正在构建 Hugo 预览"
	default:
		return "正在保存预览结果"
	}
}

func deliveryStage(progress int) string {
	switch {
	case progress < 45:
		return "正在校验发布内容"
	case progress < 85:
		return "正在更新 Hugo 内容"
	default:
		return "正在记录发布结果"
	}
}
