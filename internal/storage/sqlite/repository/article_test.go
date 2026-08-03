package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestArticleRepositoryUpsertAndFindByStableID(t *testing.T) {
	t.Parallel()

	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedWorkspace(t, db)

	repo := NewArticleRepository(db)
	want := article.Article{ID: "a1", WorkspaceID: "w1", SourceID: "s1", StableID: "article_ONE", RelativePath: "one.md", Title: "标题", Keywords: []string{"go"}}
	if err := repo.Upsert(context.Background(), want); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	got, err := repo.FindByStableID(context.Background(), "w1", "article_ONE")
	if err != nil {
		t.Fatalf("FindByStableID() error = %v", err)
	}
	if got.Title != want.Title || len(got.Keywords) != 1 || got.Keywords[0] != "go" {
		t.Fatalf("FindByStableID() = %#v", got)
	}
}

func TestArticleRepositoryPersistsContentStageAndIssue(t *testing.T) {
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repo := NewArticleRepository(db)
	ready := article.Article{
		ID: "a-ready", WorkspaceID: "w1", SourceID: "s1", StableID: "article_READY",
		RelativePath: "ready.md", ContentStage: article.ContentStageReady,
	}
	if err := repo.Upsert(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	var stage, issue string
	if err := db.QueryRow(`SELECT content_stage,content_stage_issue FROM articles WHERE id='a-ready'`).Scan(&stage, &issue); err != nil {
		t.Fatal(err)
	}
	if stage != string(article.ContentStageReady) || issue != "" {
		t.Fatalf("stored stage = %q, issue = %q", stage, issue)
	}

	draft := ready
	draft.ID = "a-invalid"
	draft.StableID = "article_INVALID"
	draft.RelativePath = "invalid.md"
	draft.ContentStage = article.ContentStageDraft
	draft.ContentStageIssue = "publish.status 仅支持 draft 或 ready"
	if err := repo.Upsert(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT content_stage,content_stage_issue FROM articles WHERE id='a-invalid'`).Scan(&stage, &issue); err != nil {
		t.Fatal(err)
	}
	if stage != string(article.ContentStageDraft) || issue == "" {
		t.Fatalf("stored invalid stage = %q, issue = %q", stage, issue)
	}
}

func TestArticleRepositoryRejectsInvalidStoredTime(t *testing.T) {
	t.Parallel()

	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedWorkspace(t, db)
	_, err = db.Exec(`INSERT INTO articles
(id,workspace_id,source_id,stable_id,relative_path,source_mtime,indexed_at,created_at,updated_at)
VALUES ('a1','w1','s1','article_ONE','one.md','invalid','2026-01-01T00:00:00Z','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewArticleRepository(db).FindByStableID(context.Background(), "w1", "article_ONE"); err == nil {
		t.Fatal("FindByStableID() must reject an invalid stored timestamp")
	}
}

func TestArticleRepositoryMarksApprovedReviewChangedWithContentHash(t *testing.T) {
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repo := NewArticleRepository(db)
	value := article.Article{ID: "a1", WorkspaceID: "w1", SourceID: "s1", StableID: "article_ONE", RelativePath: "one.md", Title: "标题", Tags: []string{}, Keywords: []string{}, ContentHash: "hash-v1", FrontmatterHash: "front-v1"}
	if err := repo.Upsert(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-v1','front-v1','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	value.ContentHash = "hash-v2"
	if err := repo.Upsert(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM editorial_reviews WHERE article_id='a1'`).Scan(&state); err != nil || state != "changed" {
		t.Fatalf("审核未失效: state=%s err=%v", state, err)
	}
}

func TestArticleRepositoryInvalidatesLegacyApprovalWithoutValidStableID(t *testing.T) {
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repo := NewArticleRepository(db)
	value := article.Article{ID: "a1", WorkspaceID: "w1", SourceID: "s1", RelativePath: "legacy.md", Title: "旧文章", Tags: []string{}, Keywords: []string{}, ContentHash: "hash-v1", FrontmatterHash: "front-v1"}
	if err := repo.Upsert(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-v1','front-v1','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM editorial_reviews WHERE article_id='a1'`).Scan(&state); err != nil || state != "changed" {
		t.Fatalf("缺少稳定 ID 的旧审核未失效: state=%s err=%v", state, err)
	}
}

func TestArticleRepositoryMarksMissingAndRestoresReappearingArticle(t *testing.T) {
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repo := NewArticleRepository(db)
	for _, value := range []article.Article{
		{ID: "a1", WorkspaceID: "w1", SourceID: "s1", RelativePath: "one.md"},
		{ID: "a2", WorkspaceID: "w1", SourceID: "s1", RelativePath: "two.md"},
	} {
		if err := repo.Upsert(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.MarkMissing(context.Background(), "w1", "s1", []string{"one.md"}); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE deleted_at IS NULL`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("软删除结果错误: active=%d err=%v", active, err)
	}
	if err := repo.Upsert(context.Background(), article.Article{ID: "a2", WorkspaceID: "w1", SourceID: "s1", RelativePath: "two.md"}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE deleted_at IS NULL`).Scan(&active); err != nil || active != 2 {
		t.Fatalf("重新纳入未恢复: active=%d err=%v", active, err)
	}
}
