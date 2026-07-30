package publication

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPublicationHistoryMapsChannelsAndBuildsBoundCursor(t *testing.T) {
	when := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	resolver := staticHistoryResolver{article: HistoryArticle{ArticleID: "a1", WorkspaceID: "w1"}}
	store := &staticHistoryStore{pages: []HistoryEventPage{{Items: []HistoryEvent{
		{ID: "e4", ProviderType: "wechat", Type: "marked_published", PayloadJSON: `{}`, CreatedAt: when.Add(2 * time.Second)},
		{ID: "e3", ProviderType: "hugo", Type: "marked_published", PayloadJSON: `{}`, CreatedAt: when.Add(time.Second)},
		{ID: "e2", ProviderType: "hugo", Type: "failed", PayloadJSON: `{"message":"/secret/hugo failed"}`, CreatedAt: when},
		{ID: "e1", ProviderType: "wechat", Type: "confirmed", PayloadJSON: `{}`, CreatedAt: when.Add(-time.Second)},
	}, HasMore: true}}}
	service := NewPublicationHistoryService(resolver, store)
	page, err := service.History(context.Background(), "a1", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 4 || page.Items[0].Title != "已标记为微信已发表" || page.Items[0].State != "published" || page.Items[1].Title != "已标记为 Hugo 已发表" || page.Items[1].State != "published" || page.Items[2].Title != "Hugo 同步失败" || page.Items[2].Detail == "/secret/hugo failed" || page.Items[3].Title != "已确认保存微信草稿" || page.NextCursor == "" {
		t.Fatalf("发布历史映射错误: %+v", page)
	}
	otherService := NewPublicationHistoryService(staticHistoryResolver{article: HistoryArticle{ArticleID: "other", WorkspaceID: "w1"}}, store)
	if _, err := otherService.History(context.Background(), "other", page.NextCursor, 2); !errors.Is(err, ErrHistoryCursorInvalid) {
		t.Fatalf("跨文章 cursor 未拒绝: %v", err)
	}
}

type staticHistoryStore struct {
	pages []HistoryEventPage
	calls int
}

type staticHistoryResolver struct{ article HistoryArticle }

func (r staticHistoryResolver) ResolveHistoryArticle(context.Context, string) (HistoryArticle, error) {
	return r.article, nil
}

func (s *staticHistoryStore) ListPublicationEvents(context.Context, string, string, HistoryEventCursor, int) (HistoryEventPage, error) {
	if s.calls >= len(s.pages) {
		return HistoryEventPage{Items: []HistoryEvent{}}, nil
	}
	page := s.pages[s.calls]
	s.calls++
	return page, nil
}
