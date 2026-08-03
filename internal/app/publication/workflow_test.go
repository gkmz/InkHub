package publication

import (
	"context"
	"errors"
	"testing"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
)

func TestPublicationWorkflowPrefersDeliveryAndUsesSafePreview(t *testing.T) {
	resolver := staticWorkflowResolver{article: WorkflowArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash"}}
	store := staticWorkflowJobStore{jobs: map[string]domainjob.Job{
		"hugo_preview": {ID: "preview_1", WorkspaceID: "w1", Kind: "hugo_preview", State: domainjob.StateSucceeded, Progress: 100},
		"hugo_deliver": {ID: "delivery_1", WorkspaceID: "w1", Kind: "hugo_deliver", State: domainjob.StateRunning, Progress: 52, PayloadJSON: `{"preview_id":"preview_1"}`},
	}}
	finder := staticWorkflowPreviewFinder{view: PreviewView{ID: "preview_1", ArticleID: "a1", TargetPath: "content/posts/demo", State: "ready"}}
	service := NewPublicationWorkflowService(resolver, store, finder)

	view, err := service.Find(context.Background(), "a1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Hugo == nil || view.Hugo.State != "delivering" || view.Hugo.Progress != 52 || view.Hugo.Stage != "正在更新 Hugo 内容" || view.Hugo.Preview == nil || view.Hugo.Preview.TargetPath != "content/posts/demo" {
		t.Fatalf("交付恢复视图错误: %+v", view)
	}
}

func TestPublicationWorkflowReturnsEmptyAndMapsPreviewStage(t *testing.T) {
	resolver := staticWorkflowResolver{article: WorkflowArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash"}}
	empty := NewPublicationWorkflowService(resolver, staticWorkflowJobStore{jobs: map[string]domainjob.Job{}}, staticWorkflowPreviewFinder{})
	view, err := empty.Find(context.Background(), "a1")
	if err != nil || view.Hugo != nil {
		t.Fatalf("无任务工作流错误: %+v err=%v", view, err)
	}
	preparing := NewPublicationWorkflowService(resolver, staticWorkflowJobStore{jobs: map[string]domainjob.Job{"hugo_preview": {ID: "preview_1", Kind: "hugo_preview", State: domainjob.StateRunning, Progress: 50}}}, staticWorkflowPreviewFinder{})
	view, err = preparing.Find(context.Background(), "a1")
	if err != nil || view.Hugo == nil || view.Hugo.State != "preparing" || view.Hugo.Stage != "正在构建 Hugo 预览" {
		t.Fatalf("Preview 阶段映射错误: %+v err=%v", view, err)
	}
}

func TestPublicationWorkflowRestoresFailedPreviewReason(t *testing.T) {
	resolver := staticWorkflowResolver{article: WorkflowArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash"}}
	store := staticWorkflowJobStore{jobs: map[string]domainjob.Job{
		"hugo_preview": {ID: "preview_failed", Kind: "hugo_preview", State: domainjob.StateFailed, ErrorCode: "source.image_unresolved", ErrorMessage: "图片引用无法解析: missing.png"},
	}}
	service := NewPublicationWorkflowService(resolver, store, staticWorkflowPreviewFinder{})

	view, err := service.Find(context.Background(), "a1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Hugo == nil || view.Hugo.State != "failed" || view.Hugo.Failure == nil || view.Hugo.Failure.Stage != "preflight" || view.Hugo.Failure.Action == "" {
		t.Fatalf("失败工作流未恢复诊断: %+v", view.Hugo)
	}
}

type staticWorkflowResolver struct{ article WorkflowArticle }

func (r staticWorkflowResolver) ResolveWorkflowArticle(context.Context, string) (WorkflowArticle, error) {
	return r.article, nil
}

type staticWorkflowJobStore struct{ jobs map[string]domainjob.Job }

func (s staticWorkflowJobStore) FindLatestTargetJob(_ context.Context, _, _, _, _, kind string) (domainjob.Job, bool, error) {
	job, found := s.jobs[kind]
	return job, found, nil
}

type staticWorkflowPreviewFinder struct {
	view PreviewView
	err  error
}

func (f staticWorkflowPreviewFinder) Find(context.Context, string) (PreviewView, error) {
	if f.err != nil {
		return PreviewView{}, f.err
	}
	if f.view.ID == "" {
		return PreviewView{}, errors.New("preview not found")
	}
	return f.view, nil
}
