package editorial

import (
	"context"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
)

func TestReviewArticleRejectsBlockingFindingsAndApprovesCurrentHash(t *testing.T) {
	t.Parallel()

	store := &fakeReviewStore{}
	value := article.Article{ID: "a1", StableID: "article_ONE", ContentHash: "hash1", FrontmatterHash: "front1"}
	if _, err := ReviewArticle(context.Background(), store, value, []Finding{{Severity: SeverityBlocking}}); err == nil {
		t.Fatal("ReviewArticle() must reject blocking findings")
	}
	review, err := ReviewArticle(context.Background(), store, value, nil)
	if err != nil {
		t.Fatalf("ReviewArticle() error = %v", err)
	}
	if review.ApprovedContentHash != "hash1" || store.saved.ApprovedContentHash != "hash1" {
		t.Fatalf("review = %#v", review)
	}
}

type fakeReviewStore struct{ saved Review }

func (s *fakeReviewStore) Save(_ context.Context, review Review) error {
	s.saved = review
	return nil
}
