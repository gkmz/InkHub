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
