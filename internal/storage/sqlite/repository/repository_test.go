package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func openRepositoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedWorkspace(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at)
VALUES ('w1','test','/tmp/test','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at)
VALUES ('s1','w1','obsidian','/tmp/vault','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
}

func seedPublicationParents(t *testing.T, db *sql.DB) {
	t.Helper()
	seedWorkspace(t, db)
	_, err := db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,indexed_at,created_at,updated_at)
VALUES ('a1','w1','s1','article_ONE','one.md','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at)
VALUES ('provider1','w1','hugo','Hugo','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
}
