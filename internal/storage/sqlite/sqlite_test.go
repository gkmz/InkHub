package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"testing"
)

func TestOpenMigratesEmptyDatabaseAndIsRepeatable(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "inkhub.db")
	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	db.Close()

	db, err = Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if version != 2 {
		t.Fatalf("migration version = %d, want 2", version)
	}
}

func TestOpenEnablesForeignKeys(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, want 1", enabled)
	}
}

func TestSchemaCommentsCoverEveryTableAndColumn(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	rows, err := db.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		assertCommentExists(t, db, "table", table)

		columns, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatalf("query columns for %s: %v", table, err)
		}
		for columns.Next() {
			var column string
			if err := columns.Scan(&column); err != nil {
				t.Fatal(err)
			}
			assertCommentExists(t, db, "column", table+"."+column)
		}
		columns.Close()
	}
}

func TestSchemaRejectsStableIDReuseAndActiveJobDuplicates(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	insertWorkspaceAndSource(t, db)
	_, err := db.Exec(`INSERT INTO articles
(id, workspace_id, source_id, stable_id, relative_path, indexed_at, created_at, updated_at)
VALUES ('a1','w1','s1','article_ONE','one.md','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatalf("insert first article: %v", err)
	}
	_, err = db.Exec(`INSERT INTO articles
(id, workspace_id, source_id, stable_id, relative_path, indexed_at, deleted_at, created_at, updated_at)
VALUES ('a2','w1','s1','article_ONE','two.md','2026-01-01','2026-01-02','2026-01-01','2026-01-01')`)
	if err == nil {
		t.Fatal("stable ID reuse must fail even when one article is deleted")
	}

	_, err = db.Exec(`INSERT INTO jobs
(id, workspace_id, kind, dedupe_key, state, available_at, created_at, updated_at)
VALUES ('j1','w1','scan','scan:w1','queued','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatalf("insert first job: %v", err)
	}
	_, err = db.Exec(`INSERT INTO jobs
(id, workspace_id, kind, dedupe_key, state, available_at, created_at, updated_at)
VALUES ('j2','w1','scan','scan:w1','running','2026-01-01','2026-01-01','2026-01-01')`)
	if err == nil {
		t.Fatal("duplicate active dedupe key must fail")
	}
}

func TestSchemaAllowsMultipleArticlesWithoutStableID(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	insertWorkspaceAndSource(t, db)
	for _, values := range []string{
		`('a1','w1','s1','','one.md','2026-01-01','2026-01-01','2026-01-01')`,
		`('a2','w1','s1','','two.md','2026-01-01','2026-01-01','2026-01-01')`,
	} {
		if _, err := db.Exec(`INSERT INTO articles
(id,workspace_id,source_id,stable_id,relative_path,indexed_at,created_at,updated_at) VALUES ` + values); err != nil {
			t.Fatalf("无稳定 ID文章应按路径共存: %v", err)
		}
	}
}

func TestOptionalStableIDMigrationPreservesArticleRelations(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "inkhub.db")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE schema_migrations (
version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL
)`); err != nil {
		t.Fatal(err)
	}
	initial, err := migrationFiles.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(string(initial)); err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(initial)
	if _, err := legacy.Exec(`INSERT INTO schema_migrations(version,name,checksum,applied_at) VALUES(1,'0001_init.sql',?,'2026-01-01')`, hex.EncodeToString(checksum[:])); err != nil {
		t.Fatal(err)
	}
	insertWorkspaceAndSource(t, legacy)
	if _, err := legacy.Exec(`INSERT INTO articles
(id,workspace_id,source_id,stable_id,relative_path,indexed_at,created_at,updated_at)
VALUES('a1','w1','s1','','one.md','2026-01-01','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO editorial_reviews(article_id,state,updated_at) VALUES('a1','draft','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	migrated, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("迁移 v1 数据库: %v", err)
	}
	defer migrated.Close()
	var reviews int
	if err := migrated.QueryRow(`SELECT COUNT(*) FROM editorial_reviews WHERE article_id='a1'`).Scan(&reviews); err != nil || reviews != 1 {
		t.Fatalf("迁移丢失文章关系: count=%d err=%v", reviews, err)
	}
	if _, err := migrated.Exec(`INSERT INTO articles
(id,workspace_id,source_id,stable_id,relative_path,indexed_at,created_at,updated_at)
VALUES('a2','w1','s1','','two.md','2026-01-01','2026-01-01','2026-01-01')`); err != nil {
		t.Fatalf("迁移后空稳定 ID仍冲突: %v", err)
	}
}

func TestBackupCreatesValidDatabase(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	insertWorkspaceAndSource(t, db)
	destination := filepath.Join(t.TempDir(), "backup.db")
	if err := Backup(context.Background(), db, destination); err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	backup, err := sql.Open("sqlite", "file:"+filepath.ToSlash(destination))
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer backup.Close()
	var result string
	if err := backup.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if result != "ok" {
		t.Fatalf("integrity_check = %q, want ok", result)
	}
}

func TestOpenBacksUpExistingDatabaseBeforeMigration(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "inkhub.db")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE legacy_state(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	db.Close()

	backups, err := filepath.Glob(filepath.Join(dir, "backups", "inkhub-before-migration-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("migration backups = %d, want 1", len(backups))
	}
}

func TestOpenRejectsChangedMigrationChecksum(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "inkhub.db")
	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum='changed' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := Open(context.Background(), dbPath); err == nil {
		t.Fatal("Open() must reject a changed migration checksum")
	}
}

func TestSchemaRejectsCrossWorkspaceArticleSource(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	insertWorkspaceAndSource(t, db)
	_, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at)
VALUES ('w2','other','/tmp/other','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO articles
(id,workspace_id,source_id,stable_id,relative_path,indexed_at,created_at,updated_at)
VALUES ('a1','w2','s1','article_ONE','one.md','2026-01-01','2026-01-01','2026-01-01')`)
	if err == nil {
		t.Fatal("cross-workspace article/source relation must fail")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func assertCommentExists(t *testing.T, db *sql.DB, objectType, objectName string) {
	t.Helper()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM schema_comments WHERE object_type = ? AND object_name = ?`, objectType, objectName).Scan(&count)
	if err != nil {
		t.Fatalf("query comment %s %s: %v", objectType, objectName, err)
	}
	if count != 1 {
		t.Errorf("missing schema comment for %s %s", objectType, objectName)
	}
}

func insertWorkspaceAndSource(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at)
VALUES ('w1','test','/tmp/test','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	_, err = db.Exec(`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at)
VALUES ('s1','w1','obsidian','/tmp/vault','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatalf("insert source: %v", err)
	}
}
