package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	appdisposition "github.com/gkmz/InkHub/internal/app/disposition"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestDispositionStorePublishesMultipleChannelsAtomically(t *testing.T) {
	t.Parallel()
	service, db := seedDispositionService(t, true)
	command := appdisposition.Command{
		Operation: appdisposition.OperationPublished,
		Articles:  []appdisposition.ArticleVersion{{ID: "a1", ContentVersion: "hash-1"}, {ID: "a2", ContentVersion: "hash-2"}},
		Channels:  []string{"hugo", "wechat"},
	}
	result, err := service.Apply(context.Background(), command)
	if err != nil || result.Processed != 2 || result.Changed != 2 {
		t.Fatalf("Apply() = %+v, %v", result, err)
	}
	assertDispositionRows(t, db, "article_dispositions", 2)
	assertDispositionRows(t, db, "publications", 4)
	assertDispositionRows(t, db, "publication_events", 4)

	repeated, err := service.Apply(context.Background(), command)
	if err != nil || repeated.Changed != 0 || repeated.Unchanged != 2 {
		t.Fatalf("repeated Apply() = %+v, %v", repeated, err)
	}
	assertDispositionRows(t, db, "publication_events", 4)
}

func TestDispositionStoreRollsBackOnVersionConflict(t *testing.T) {
	t.Parallel()
	service, db := seedDispositionService(t, true)
	_, err := service.Apply(context.Background(), appdisposition.Command{
		Operation: appdisposition.OperationPublished,
		Articles:  []appdisposition.ArticleVersion{{ID: "a1", ContentVersion: "hash-1"}, {ID: "a2", ContentVersion: "stale"}},
		Channels:  []string{"hugo"},
	})
	if !errors.Is(err, appdisposition.ErrContentChanged) {
		t.Fatalf("Apply() error = %v", err)
	}
	assertDispositionRows(t, db, "article_dispositions", 0)
	assertDispositionRows(t, db, "publications", 0)
	assertDispositionRows(t, db, "publication_events", 0)
}

func TestDispositionStoreIgnoresAcrossVersionsAndRestores(t *testing.T) {
	t.Parallel()
	service, db := seedDispositionService(t, true)
	_, err := service.Apply(context.Background(), appdisposition.Command{
		Operation: appdisposition.OperationIgnored,
		Articles:  []appdisposition.ArticleVersion{{ID: "a1", ContentVersion: "hash-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE articles SET content_hash='hash-new' WHERE id='a1'`); err != nil {
		t.Fatal(err)
	}
	var kind, savedHash string
	var cleared sql.NullString
	if err := db.QueryRow(`SELECT kind,content_hash,cleared_at FROM article_dispositions WHERE article_id='a1'`).Scan(&kind, &savedHash, &cleared); err != nil || kind != "ignored" || savedHash != "hash-1" || cleared.Valid {
		t.Fatalf("ignored projection kind=%s hash=%s cleared=%v err=%v", kind, savedHash, cleared, err)
	}
	result, err := service.Apply(context.Background(), appdisposition.Command{
		Operation: appdisposition.OperationRestore,
		Articles:  []appdisposition.ArticleVersion{{ID: "a1", ContentVersion: "hash-new"}},
	})
	if err != nil || result.Changed != 1 {
		t.Fatalf("restore = %+v, %v", result, err)
	}
	if err := db.QueryRow(`SELECT cleared_at FROM article_dispositions WHERE article_id='a1'`).Scan(&cleared); err != nil || !cleared.Valid {
		t.Fatalf("cleared_at=%v err=%v", cleared, err)
	}
}

func TestDispositionStoreRejectsUnavailableChannel(t *testing.T) {
	t.Parallel()
	service, db := seedDispositionService(t, false)
	_, err := service.Apply(context.Background(), appdisposition.Command{
		Operation: appdisposition.OperationPublished,
		Articles:  []appdisposition.ArticleVersion{{ID: "a1", ContentVersion: "hash-1"}},
		Channels:  []string{"wechat"},
	})
	if !errors.Is(err, appdisposition.ErrChannelUnavailable) {
		t.Fatalf("Apply() error = %v", err)
	}
	assertDispositionRows(t, db, "article_dispositions", 0)
}

func seedDispositionService(t *testing.T, includeWechat bool) (*appdisposition.Service, *sql.DB) {
	t.Helper()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','当前','/tmp','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-07-30','2026-07-30');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,content_hash,indexed_at,created_at,updated_at) VALUES
('a1','w1','s1','one','one.md','hash-1','2026-07-30','2026-07-30','2026-07-30'),
('a2','w1','s1','two','two.md','hash-2','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('h1','w1','hugo','Hugo','2026-07-30','2026-07-30')`)
	if err != nil {
		t.Fatal(err)
	}
	if includeWechat {
		if _, err := db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('m1','w1','wechat','微信','2026-07-30','2026-07-30')`); err != nil {
			t.Fatal(err)
		}
	}
	return newDispositionService(db), db
}

func assertDispositionRows(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil || got != want {
		t.Fatalf("%s rows=%d want=%d err=%v", table, got, want, err)
	}
}
