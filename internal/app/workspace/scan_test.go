package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestScanWorkspaceKeepsSuccessfulArticlesWhenOneReadFails(t *testing.T) {
	t.Parallel()

	source := fakeSource{
		result: contracts.ScanResult{Complete: true, Documents: []contracts.SourceDocumentRef{
			{Ref: contracts.SourceRef{SourceID: "source1", RelativePath: "good.md", StableID: "article_ONE"}},
			{Ref: contracts.SourceRef{SourceID: "source1", RelativePath: "bad.md"}},
		}},
		documents: map[string]contracts.SourceDocument{
			"good.md": {Article: article.Article{SourceID: "source1", RelativePath: "good.md", StableID: "article_ONE"}, Fingerprint: "fingerprint"},
		},
	}
	store := &fakeArticleStore{}
	report, err := ScanWorkspace(context.Background(), source, store, ScanOptions{WorkspaceID: "w1", SourceID: "source1"}, contracts.ScanCursor{})
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	if len(store.saved) != 1 || report.Failed != 1 || report.Indexed != 1 {
		t.Fatalf("saved=%d report=%#v", len(store.saved), report)
	}
	if store.saved[0].ID == "" || store.saved[0].WorkspaceID != "w1" || store.saved[0].SourceFingerprint != "fingerprint" || store.saved[0].ContentHash == "" {
		t.Fatalf("indexed article is incomplete: %#v", store.saved[0])
	}
	if len(store.seen) != 2 {
		t.Fatalf("完整扫描必须保留失败文章身份: %#v", store.seen)
	}
}

func TestScanWorkspaceDoesNotMarkMissingAfterIncrementalResult(t *testing.T) {
	t.Parallel()

	store := &fakeArticleStore{}
	_, err := ScanWorkspace(context.Background(), fakeSource{result: contracts.ScanResult{Complete: false}}, store, ScanOptions{WorkspaceID: "w1", SourceID: "source1"}, contracts.ScanCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if store.markedMissing {
		t.Fatal("非完整扫描不得软删除未见文章")
	}
}

type fakeSource struct {
	result    contracts.ScanResult
	documents map[string]contracts.SourceDocument
}

func (f fakeSource) Scan(context.Context, contracts.ScanCursor) (contracts.ScanResult, error) {
	return f.result, nil
}

func (f fakeSource) Read(_ context.Context, ref contracts.SourceRef) (contracts.SourceDocument, error) {
	document, ok := f.documents[ref.RelativePath]
	if !ok {
		return contracts.SourceDocument{}, errors.New("invalid document")
	}
	return document, nil
}

type fakeArticleStore struct {
	saved         []article.Article
	seen          []string
	markedMissing bool
}

func (s *fakeArticleStore) Upsert(_ context.Context, value article.Article) error {
	s.saved = append(s.saved, value)
	return nil
}

func (s *fakeArticleStore) MarkMissing(_ context.Context, _, _ string, seenIDs []string) error {
	s.markedMissing = true
	s.seen = append([]string(nil), seenIDs...)
	return nil
}
