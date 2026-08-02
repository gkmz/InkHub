package bootstrap

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

func TestListArticlesScopesSearchesAndFiltersDispositions(t *testing.T) {
	t.Parallel()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedArticleListRows(t, db)
	api := newDatabaseAPI(db)

	page, err := api.ListArticles(context.Background(), httptransport.ArticleListQuery{Search: "SQLite", State: "pending_review", Disposition: "unresolved", Limit: 50})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "match" || page.Items[0].ContentVersion != "hash-match" {
		t.Fatalf("filtered ListArticles() = %+v, %v", page, err)
	}
	if !reflect.DeepEqual(page.AvailableChannels, []string{"hugo"}) {
		t.Fatalf("available channels = %v", page.AvailableChannels)
	}

	assertArticleListIDs(t, api, httptransport.ArticleListQuery{Disposition: "published", Limit: 50}, "published")
	assertArticleListIDs(t, api, httptransport.ArticleListQuery{Disposition: "ignored", Limit: 50}, "ignored")
	assertArticleListIDs(t, api, httptransport.ArticleListQuery{Limit: 50}, "stale", "published", "match")
}

func TestListArticlesPrioritizesReadyBeforeDrafts(t *testing.T) {
	t.Parallel()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test','/tmp','2026-01-01','2026-01-01','2026-01-01');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,indexed_at,created_at,updated_at,source_mtime,content_stage) VALUES
('draft-new','w1','s1','draft-new','draft-new.md','较新的草稿','draft-new','2026-01-03','2026-01-03','2026-01-03','2026-01-03','draft'),
('ready-old','w1','s1','ready-old','ready-old.md','较早的就绪文章','ready-old','2026-01-02','2026-01-02','2026-01-02','2026-01-02','ready'),
('ready-new','w1','s1','ready-new','ready-new.md','较新的就绪文章','ready-new','2026-01-01','2026-01-01','2026-01-01','2026-01-01','ready')`)
	if err != nil {
		t.Fatal(err)
	}
	api := newDatabaseAPI(db)
	first, err := api.ListArticles(context.Background(), httptransport.ArticleListQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "ready-old" || first.Items[1].ID != "ready-new" {
		t.Fatalf("ready-first page = %+v", first.Items)
	}
	if first.NextCursor == "" {
		t.Fatal("ready page should include a cursor for the remaining draft")
	}
	assertArticleListIDs(t, api, httptransport.ArticleListQuery{Cursor: first.NextCursor, Limit: 2}, "draft-new")
}

func seedArticleListRows(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES
('w1','旧工作区','/tmp','2026-07-01','2026-07-01','2026-07-01'),
('w2','当前工作区','/tmp','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES
('s1','w1','obsidian','/tmp/old','2026-07-01','2026-07-01'),
('s2','w2','obsidian','/tmp/current','2026-07-30','2026-07-30');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,indexed_at,created_at,updated_at,source_mtime,content_stage) VALUES
('old','w1','s1','old','old.md','SQLite 旧文章','hash-old','2026-07-01','2026-07-01','2026-07-01','2026-07-31T12:00:00Z','ready'),
('match','w2','s2','match','notes/match.md','SQLite 当前文章','hash-match','2026-07-30','2026-07-30','2026-07-30','2026-07-30T09:00:00Z','ready'),
('ignored','w2','s2','ignored','notes/ignored.md','忽略文章','hash-ignored','2026-07-30','2026-07-30','2026-07-30','2026-07-30T11:00:00Z','ready'),
('published','w2','s2','published','notes/published.md','已发表文章','hash-published','2026-07-30','2026-07-30','2026-07-30','2026-07-30T10:00:00Z','ready'),
('stale','w2','s2','stale','notes/stale.md','已更新文章','hash-new','2026-07-30','2026-07-30','2026-07-30','2026-07-30T12:00:00Z','ready');
INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,created_at,updated_at) VALUES
('ignored','w2','ignored','hash-before-ignore','2026-07-30','2026-07-30'),
('published','w2','published','hash-published','2026-07-30','2026-07-30'),
('stale','w2','published','hash-old','2026-07-30','2026-07-30');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,created_at,updated_at) VALUES
('h2','w2','hugo','Hugo',1,'2026-07-30','2026-07-30'),
('m2','w2','wechat','微信',0,'2026-07-30','2026-07-30')`)
	if err != nil {
		t.Fatal(err)
	}
}

func assertArticleListIDs(t *testing.T, api databaseAPI, query httptransport.ArticleListQuery, want ...string) {
	t.Helper()
	page, err := api.ListArticles(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		got = append(got, item.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("article IDs = %v, want %v", got, want)
	}
}
