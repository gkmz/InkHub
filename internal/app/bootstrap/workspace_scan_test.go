package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestRescanRecentWorkspaceIndexesConfiguredScopeIdempotently(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "Areas", "普通笔记.md"), []byte("正文"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string][]string{"content_roots": {"Areas"}, "ignored_folders": {}})
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test',?,?,?,?)`, t.TempDir(), now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sources(id,workspace_id,provider_type,root_path,config_json,created_at,updated_at) VALUES('s1','w1','obsidian',?,?,?,?)`, vault, string(config), now, now); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		report, err := RescanRecentWorkspace(context.Background(), db)
		if err != nil || report.Indexed != 1 {
			t.Fatalf("启动重扫失败: report=%#v err=%v", report, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE deleted_at IS NULL AND title='普通笔记'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("启动重扫不幂等: count=%d err=%v", count, err)
	}
}
