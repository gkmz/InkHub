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

func TestXiaohongshuRepositoryRoundTripsPagesAndBlocks(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	if _, err := db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,content_hash,indexed_at,created_at,updated_at) VALUES('a-pages','w1','s1','pages','pages.md','hash-pages','2026-01-01','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	want := []domain.Page{{ID: "page-1", Blocks: []domain.Block{{ID: "code-1", Kind: "code", HTML: "<pre><code>go test</code></pre>", Splittable: false}}, MeasuredHeight: 220}, {ID: "page-2", Blocks: []domain.Block{{ID: "text-1", Kind: "paragraph", HTML: "<p>正文</p>", Splittable: true}}, MeasuredHeight: 180}}
	r := NewXiaohongshuRepository(db)
	if err := r.SaveDraft(context.Background(), domain.Draft{ID: "draft-pages", ArticleID: "a-pages", WorkspaceID: "w1", SourceContentHash: "hash-pages", Title: "页面草稿", BodyHTML: "<p>正文</p>", Pages: want, State: domain.DraftStateDraft}); err != nil {
		t.Fatal(err)
	}
	got, err := r.FindDraft(context.Background(), "w1", "a-pages", "draft-pages")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 2 || got.Pages[0].Blocks[0].Kind != "code" || got.Pages[1].MeasuredHeight != 180 {
		t.Fatalf("页面模型往返错误: %+v", got.Pages)
	}
}

func TestXiaohongshuRepositorySeparatesDraftModes(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	if _, err := db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,content_hash,indexed_at,created_at,updated_at) VALUES('a-modes','w1','s1','modes','modes.md','hash-modes','2026-01-01','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	r := NewXiaohongshuRepository(db)
	longDraft := domain.Draft{ID: "draft-long", ArticleID: "a-modes", WorkspaceID: "w1", SourceContentHash: "hash-modes", Mode: domain.DraftModeLongCard, Title: "长文", BodyHTML: "<p>正文</p>", State: domain.DraftStateDraft}
	scriptDraft := domain.Draft{ID: "draft-script", ArticleID: "a-modes", WorkspaceID: "w1", SourceContentHash: "hash-modes", Mode: domain.DraftModeVisualScript, Title: "分镜", BodyHTML: "发布正文", ScriptPages: []domain.ScriptPage{{ID: "page-1", Title: "封面", Prompt: "生成封面"}}, State: domain.DraftStateDraft}
	for _, draft := range []domain.Draft{longDraft, scriptDraft} {
		if err := r.SaveDraft(context.Background(), draft); err != nil {
			t.Fatal(err)
		}
	}
	items, err := r.ListDraftsByMode(context.Background(), "w1", "a-modes", domain.DraftModeVisualScript, 20)
	if err != nil || len(items) != 1 || items[0].Mode != domain.DraftModeVisualScript || len(items[0].ScriptPages) != 1 {
		t.Fatalf("分镜草稿过滤或往返错误: %+v err=%v", items, err)
	}
}
