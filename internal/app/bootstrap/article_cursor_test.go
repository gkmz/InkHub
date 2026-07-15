package bootstrap

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestArticleCursorRoundTripsAndRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	want := articleCursor{ModifiedAt: "2026-07-15T10:00:00Z", ID: "article_1"}
	encoded, err := encodeArticleCursor(want)
	if err != nil {
		t.Fatalf("encodeArticleCursor() error = %v", err)
	}
	got, err := decodeArticleCursor(encoded)
	if err != nil || got != want {
		t.Fatalf("decodeArticleCursor() = %+v, %v; want %+v", got, err, want)
	}
	for _, invalid := range []string{"%%%", strings.Repeat("a", 1025), "e30"} {
		if _, err := decodeArticleCursor(invalid); err == nil {
			t.Fatalf("decodeArticleCursor(%q) error = nil", invalid)
		}
	}
}

func TestListArticlesPaginatesWithStableCursor(t *testing.T) {
	t.Parallel()

	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test','/tmp','2026-01-01','2026-01-01','2026-01-01');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,indexed_at,created_at,updated_at,source_mtime) VALUES
('a3','w1','s1','stable_3','3.md','第三篇','2026-01-03','2026-01-03','2026-01-03','2026-01-03'),
('a2','w1','s1','stable_2','2.md','第二篇','2026-01-02','2026-01-02','2026-01-02','2026-01-02'),
('a1','w1','s1','stable_1','1.md','第一篇','2026-01-01','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	api := newDatabaseAPI(db)

	first, err := api.ListArticles(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "a3" || first.Items[1].ID != "a2" || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := api.ListArticles(context.Background(), first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "a1" || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
}
