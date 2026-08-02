package repository

import (
	"context"
	"testing"

	domain "github.com/gkmz/InkHub/internal/domain/xiaohongshu"
)

func TestXiaohongshuRepositoryKeepsGeneratedHistoryAndScopesArticle(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	if _, err := db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,content_hash,indexed_at,created_at,updated_at) VALUES
('a1','w1','s1','one','one.md','hash-1','2026-01-01','2026-01-01','2026-01-01'),
('a2','w1','s1','two','two.md','hash-2','2026-01-01','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	r := NewXiaohongshuRepository(db)
	for _, id := range []string{"draft_1", "draft_2"} {
		if err := r.SaveDraft(context.Background(), domain.Draft{ID: id, ArticleID: "a1", WorkspaceID: "w1", SourceContentHash: "hash-1", Title: id, BodyHTML: "<p>body</p>", State: domain.DraftStateDraft}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := r.ListDrafts(context.Background(), "w1", "a1", 20)
	if err != nil || len(items) != 2 {
		t.Fatalf("草稿历史 = %+v, err=%v", items, err)
	}
	if _, err := r.FindDraft(context.Background(), "w1", "a2", "draft_1"); err == nil {
		t.Fatal("跨文章查询不应返回草稿")
	}
	if err := r.SaveEvent(context.Background(), domain.Event{ID: "event_1", DraftID: "draft_2", EventType: "generated"}); err != nil {
		t.Fatal(err)
	}
}

func TestXiaohongshuRepositoryRejectsIncompleteDraft(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	r := NewXiaohongshuRepository(db)
	if err := r.SaveDraft(context.Background(), domain.Draft{}); err == nil {
		t.Fatal("缺少身份字段必须失败")
	}
}
