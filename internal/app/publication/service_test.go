package publication

import (
	"context"
	"errors"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	domainpublication "github.com/gkmz/InkHub/internal/domain/publication"
)

func TestQueueProvidersBindCurrentApprovedContentHash(t *testing.T) {
	t.Parallel()

	queue := &capturingQueue{}
	service := NewService(queue, &capturingPublicationStore{})
	value := article.Article{ID: "article_1", WorkspaceID: "workspace_1", ContentHash: "hash-v1", ContentStage: article.ContentStageReady}
	for _, providerID := range []string{"provider_hugo", "provider_wechat"} {
		jobID, err := service.Queue(context.Background(), QueueRequest{
			JobID: "job_" + providerID, ProviderInstanceID: providerID,
			Article: value, ApprovedContentHash: "hash-v1",
		})
		if err != nil {
			t.Fatalf("%s 入队: %v", providerID, err)
		}
		if jobID == "" || queue.last.ContentHash != "hash-v1" || queue.last.ArticleID != "article_1" {
			t.Fatalf("任务未绑定当前版本: %+v", queue.last)
		}
	}
}

func TestQueueRejectsChangedArticle(t *testing.T) {
	t.Parallel()

	service := NewService(&capturingQueue{}, &capturingPublicationStore{})
	_, err := service.Queue(context.Background(), QueueRequest{
		JobID: "job", ProviderInstanceID: "provider",
		Article:             article.Article{ID: "article_1", WorkspaceID: "workspace_1", ContentHash: "new-hash", ContentStage: article.ContentStageReady},
		ApprovedContentHash: "old-hash",
	})
	if !errors.Is(err, ErrContentChanged) {
		t.Fatalf("文章变化后仍进入发布队列: %T %v", err, err)
	}
}

func TestQueueUsesGenericPublicationJobForAnyProvider(t *testing.T) {
	t.Parallel()
	queue := &capturingQueue{}
	service := NewService(queue, nil)
	value := article.Article{ID: "article", WorkspaceID: "workspace", ContentHash: "hash", ContentStage: article.ContentStageReady}
	if _, err := service.Queue(context.Background(), QueueRequest{JobID: "job", ProviderInstanceID: "custom-provider", Article: value, ApprovedContentHash: "hash"}); err != nil {
		t.Fatalf("通用发布 Provider 应可入队: %v", err)
	}
	if queue.last.Kind != "publication" {
		t.Fatalf("任务类型未抽象: %s", queue.last.Kind)
	}
}

func TestConfirmWeChatDraftRequiresCopiedCurrentHash(t *testing.T) {
	t.Parallel()

	store := &capturingPublicationStore{record: Record{
		ID: "publication_1", ArticleID: "article_1", ProviderInstanceID: "wechat_1",
		State: domainpublication.StateCopied, ContentHash: "hash-v1",
	}}
	service := NewService(&capturingQueue{}, store)
	if err := service.ConfirmWeChatDraft(context.Background(), ConfirmRequest{
		ArticleID: "article_1", ProviderInstanceID: "wechat_1", CurrentContentHash: "hash-v1",
	}); err != nil {
		t.Fatalf("确认微信草稿: %v", err)
	}
	if store.saved.State != domainpublication.StateConfirmed || store.eventType != "confirmed" {
		t.Fatalf("确认状态未持久化: %+v event=%s", store.saved, store.eventType)
	}
}

func TestMarkWeChatCopiedRequiresPreparedCurrentHash(t *testing.T) {
	store := &capturingPublicationStore{record: Record{ID: "publication_1", ArticleID: "article_1", ProviderInstanceID: "wechat_1", State: domainpublication.StatePrepared, ContentHash: "hash-v1"}}
	service := NewService(&capturingQueue{}, store)
	if err := service.MarkWeChatCopied(context.Background(), ConfirmRequest{ArticleID: "article_1", ProviderInstanceID: "wechat_1", CurrentContentHash: "hash-v1"}); err != nil {
		t.Fatalf("记录微信复制: %v", err)
	}
	if store.saved.State != domainpublication.StateCopied || store.eventType != "copied" {
		t.Fatalf("复制状态未持久化: %+v event=%s", store.saved, store.eventType)
	}
}

type capturingQueue struct{ last JobIntent }

func (q *capturingQueue) Enqueue(_ context.Context, intent JobIntent) (string, error) {
	q.last = intent
	return intent.ID, nil
}

type capturingPublicationStore struct {
	record    Record
	saved     Record
	eventType string
}

func (s *capturingPublicationStore) Find(context.Context, string, string) (Record, error) {
	return s.record, nil
}

func (s *capturingPublicationStore) SaveWithEvent(_ context.Context, record Record, eventType string) error {
	s.saved, s.eventType = record, eventType
	return nil
}
