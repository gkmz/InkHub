package publication

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestHugoPreviewQueuesDeterministicallyAndReturnsSafeView(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := &memoryPreviewJobStore{jobs: map[string]domainjob.Job{}}
	resolver := staticPreviewResolver{article: PreviewArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash", ContentStage: article.ContentStageReady}}
	service := NewHugoPreviewService(store, resolver, func() time.Time { return now })
	job, err := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"})
	if err != nil || second.ID != job.ID || len(store.jobs) != 1 {
		t.Fatalf("预览未幂等: first=%+v second=%+v jobs=%d err=%v", job, second, len(store.jobs), err)
	}

	expires := now.Add(time.Hour)
	result := HugoPreviewResult{PreviewID: job.ID, ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", Section: "posts", Artifact: contracts.PreparedArtifact{
		OperationID: job.ID, ContentHash: "hash", Location: "/secret/staging/bundle", TargetPath: "/secret/hugo/content/posts/demo", TargetRelativePath: "content/posts/demo", Change: "added", ExpiresAt: &expires,
		Files: []contracts.ArtifactFile{{RelativePath: "index.md", MediaType: "text/markdown", Size: 10, SHA256: "abc"}},
	}}
	encoded, _ := json.Marshal(result)
	completed := store.jobs[job.ID]
	completed.State, completed.ResultJSON = domainjob.StateSucceeded, string(encoded)
	store.jobs[job.ID] = completed
	view, err := service.Find(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	viewJSON, _ := json.Marshal(view)
	if view.State != "ready" || view.TargetPath != "content/posts/demo" || strings.Contains(string(viewJSON), "/secret/") {
		t.Fatalf("预览安全视图错误: %+v json=%s", view, viewJSON)
	}
}

func TestHugoPreviewFailedJobReturnsActionableFailure(t *testing.T) {
	store := &memoryPreviewJobStore{jobs: map[string]domainjob.Job{
		"preview_failed": {ID: "preview_failed", Kind: "hugo_preview", State: domainjob.StateFailed, ErrorCode: "source.image_unresolved", ErrorMessage: "图片引用无法解析: missing.png"},
	}}
	service := NewHugoPreviewService(store, staticPreviewResolver{}, time.Now)

	view, err := service.Find(context.Background(), "preview_failed")
	if err != nil {
		t.Fatal(err)
	}
	if view.Failure == nil || view.Failure.Stage != "preflight" || view.Failure.Code != "source.image_unresolved" || view.Failure.Message != "图片引用无法解析: missing.png" || view.Failure.Action != "修复文章中的图片引用后重新生成预览" || !view.Failure.Retryable {
		t.Fatalf("Hugo 失败视图不可执行: %+v", view.Failure)
	}
}

func TestHugoPreviewConfirmRejectsStaleAndReusesDelivery(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := &memoryPreviewJobStore{jobs: map[string]domainjob.Job{}}
	resolver := &mutablePreviewResolver{article: PreviewArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash", ContentStage: article.ContentStageReady}}
	service := NewHugoPreviewService(store, resolver, func() time.Time { return now })
	preview, _ := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"})
	expires := now.Add(time.Hour)
	result := HugoPreviewResult{PreviewID: preview.ID, ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", Section: "posts", Artifact: contracts.PreparedArtifact{OperationID: preview.ID, ContentHash: "hash", Location: "/staging", TargetPath: "/hugo/content/posts/demo", TargetRelativePath: "content/posts/demo", ExpiresAt: &expires}}
	encoded, _ := json.Marshal(result)
	job := store.jobs[preview.ID]
	job.State, job.ResultJSON = domainjob.StateSucceeded, string(encoded)
	store.jobs[preview.ID] = job

	resolver.article.ContentHash = "changed"
	if _, err := service.Confirm(context.Background(), ConfirmPreviewRequest{PreviewID: preview.ID}); !errors.Is(err, ErrPreviewStale) {
		t.Fatalf("内容变化未拒绝: %v", err)
	}
	resolver.article.ContentHash = "hash"
	delivery, err := service.Confirm(context.Background(), ConfirmPreviewRequest{PreviewID: preview.ID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Confirm(context.Background(), ConfirmPreviewRequest{PreviewID: preview.ID})
	if err != nil || second.ID != delivery.ID || len(store.jobs) != 2 {
		t.Fatalf("确认未幂等: %+v %+v jobs=%d err=%v", delivery, second, len(store.jobs), err)
	}
}

func TestHugoPreviewRejectsMismatchedResolvedArticle(t *testing.T) {
	store := &memoryPreviewJobStore{jobs: map[string]domainjob.Job{}}
	resolver := staticPreviewResolver{article: PreviewArticle{ArticleID: "other", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash", ContentStage: article.ContentStageReady}}
	service := NewHugoPreviewService(store, resolver, time.Now)

	if _, err := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"}); err == nil {
		t.Fatal("解析结果属于其他文章时仍创建了预览")
	}
}

func TestHugoPreviewRejectsMismatchedResultIdentity(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := &memoryPreviewJobStore{jobs: map[string]domainjob.Job{}}
	resolver := staticPreviewResolver{article: PreviewArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash", ContentStage: article.ContentStageReady}}
	service := NewHugoPreviewService(store, resolver, func() time.Time { return now })
	preview, _ := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"})
	expires := now.Add(time.Hour)
	result := HugoPreviewResult{PreviewID: "preview_other", ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", Section: "posts", Artifact: contracts.PreparedArtifact{OperationID: preview.ID, ContentHash: "hash", ExpiresAt: &expires}}
	encoded, _ := json.Marshal(result)
	job := store.jobs[preview.ID]
	job.State, job.ResultJSON = domainjob.StateSucceeded, string(encoded)
	store.jobs[preview.ID] = job

	if _, err := service.Find(context.Background(), preview.ID); err == nil {
		t.Fatal("result_json 的预览身份不匹配时仍返回安全视图")
	}
	if _, err := service.Confirm(context.Background(), ConfirmPreviewRequest{PreviewID: preview.ID}); err == nil {
		t.Fatal("result_json 的预览身份不匹配时仍允许确认")
	}
}

func TestHugoPreviewReusesCompletedDeterministicJobs(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := &completedConflictPreviewJobStore{memoryPreviewJobStore: memoryPreviewJobStore{jobs: map[string]domainjob.Job{}}}
	resolver := staticPreviewResolver{article: PreviewArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash", ContentStage: article.ContentStageReady}}
	service := NewHugoPreviewService(store, resolver, func() time.Time { return now })
	preview, err := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"})
	if err != nil {
		t.Fatal(err)
	}
	stored := store.jobs[preview.ID]
	stored.State = domainjob.StateSucceeded
	store.jobs[preview.ID] = stored
	if reused, err := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"}); err != nil || reused.ID != preview.ID {
		t.Fatalf("已完成的确定性预览未复用: job=%+v err=%v", reused, err)
	}
}

func TestHugoPreviewRequeuesFailedDeterministicJobs(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	store := &memoryPreviewJobStore{jobs: map[string]domainjob.Job{}}
	resolver := staticPreviewResolver{article: PreviewArticle{ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", ContentHash: "hash", ContentStage: article.ContentStageReady}}
	service := NewHugoPreviewService(store, resolver, func() time.Time { return now })
	preview, err := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"})
	if err != nil {
		t.Fatal(err)
	}
	failedPreview := store.jobs[preview.ID]
	failedPreview.State = domainjob.StateFailed
	store.jobs[preview.ID] = failedPreview
	requeuedPreview, err := service.Queue(context.Background(), PreviewRequest{ArticleID: "a1", ContentHash: "hash", Section: "posts"})
	if err != nil || requeuedPreview.State != domainjob.StateQueued || requeuedPreview.ID != preview.ID {
		t.Fatalf("失败 Preview 未重排: %+v err=%v", requeuedPreview, err)
	}

	expires := now.Add(time.Hour)
	result := HugoPreviewResult{PreviewID: preview.ID, ArticleID: "a1", WorkspaceID: "w1", ProviderID: "h1", Section: "posts", Artifact: contracts.PreparedArtifact{OperationID: preview.ID, ContentHash: "hash", ExpiresAt: &expires}}
	encoded, _ := json.Marshal(result)
	succeededPreview := store.jobs[preview.ID]
	succeededPreview.State, succeededPreview.ResultJSON = domainjob.StateSucceeded, string(encoded)
	store.jobs[preview.ID] = succeededPreview
	delivery, err := service.Confirm(context.Background(), ConfirmPreviewRequest{PreviewID: preview.ID})
	if err != nil {
		t.Fatal(err)
	}
	failedDelivery := store.jobs[delivery.ID]
	failedDelivery.State = domainjob.StateFailed
	store.jobs[delivery.ID] = failedDelivery
	requeuedDelivery, err := service.Confirm(context.Background(), ConfirmPreviewRequest{PreviewID: preview.ID})
	if err != nil || requeuedDelivery.State != domainjob.StateQueued || requeuedDelivery.ID != delivery.ID {
		t.Fatalf("失败 Deliver 未重排: %+v err=%v", requeuedDelivery, err)
	}
}

type memoryPreviewJobStore struct{ jobs map[string]domainjob.Job }

func (s *memoryPreviewJobStore) Enqueue(_ context.Context, value domainjob.Job) (domainjob.Job, bool, error) {
	if existing, ok := s.jobs[value.ID]; ok {
		return existing, false, nil
	}
	value.State = domainjob.StateQueued
	s.jobs[value.ID] = value
	return value, true, nil
}

type completedConflictPreviewJobStore struct{ memoryPreviewJobStore }

func (s *completedConflictPreviewJobStore) Enqueue(ctx context.Context, value domainjob.Job) (domainjob.Job, bool, error) {
	if existing, ok := s.jobs[value.ID]; ok && existing.State == domainjob.StateSucceeded {
		return domainjob.Job{}, false, errors.New("任务 ID 已存在")
	}
	return s.memoryPreviewJobStore.Enqueue(ctx, value)
}
func (s *memoryPreviewJobStore) FindByID(_ context.Context, id string) (domainjob.Job, error) {
	value, ok := s.jobs[id]
	if !ok {
		return domainjob.Job{}, errors.New("not found")
	}
	return value, nil
}
func (s *memoryPreviewJobStore) RequeueFailed(_ context.Context, id, workspaceID, kind string, _ time.Time) (domainjob.Job, error) {
	value, ok := s.jobs[id]
	if !ok || value.WorkspaceID != workspaceID || value.Kind != kind || value.State != domainjob.StateFailed {
		return domainjob.Job{}, errors.New("failed job mismatch")
	}
	value.State, value.Progress, value.ErrorCode, value.ErrorMessage = domainjob.StateQueued, 0, "", ""
	s.jobs[id] = value
	return value, nil
}

type staticPreviewResolver struct{ article PreviewArticle }

func (r staticPreviewResolver) ResolvePreviewArticle(context.Context, string) (PreviewArticle, error) {
	return r.article, nil
}
func (r staticPreviewResolver) ValidatePreviewArtifact(context.Context, HugoPreviewResult) error {
	return nil
}

type mutablePreviewResolver struct{ article PreviewArticle }

func (r *mutablePreviewResolver) ResolvePreviewArticle(context.Context, string) (PreviewArticle, error) {
	return r.article, nil
}
func (r *mutablePreviewResolver) ValidatePreviewArtifact(context.Context, HugoPreviewResult) error {
	return nil
}
