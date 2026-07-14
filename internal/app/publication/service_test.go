package publication

import (
	"context"
	"errors"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	domainpublication "github.com/gkmz/InkHub/internal/domain/publication"
)

func TestQueueHugoAndWeChatBindCurrentApprovedContentHash(t *testing.T) {
	t.Parallel()

	queue := &capturingQueue{}
	service := NewService(queue, &capturingPublicationStore{})
	value := article.Article{ID: "article_1", WorkspaceID: "workspace_1", ContentHash: "hash-v1"}
	for _, channel := range []Channel{ChannelHugo, ChannelWeChat} {
		jobID, err := service.Queue(context.Background(), QueueRequest{
			JobID: "job_" + string(channel), ProviderInstanceID: "provider_" + string(channel),
			Channel: channel, Article: value, ApprovedContentHash: "hash-v1",
		})
		if err != nil {
			t.Fatalf("%s 入队: %v", channel, err)
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
		JobID: "job", ProviderInstanceID: "provider", Channel: ChannelHugo,
		Article:             article.Article{ID: "article_1", WorkspaceID: "workspace_1", ContentHash: "new-hash"},
		ApprovedContentHash: "old-hash",
	})
	if !errors.Is(err, ErrContentChanged) {
		t.Fatalf("文章变化后仍进入发布队列: %T %v", err, err)
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
