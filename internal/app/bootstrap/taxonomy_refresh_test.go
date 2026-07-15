package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

func TestRefreshRecentTaxonomyPersistsHugoStandardSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := inksqlite.Open(ctx, filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	site := t.TempDir()
	if err := os.WriteFile(filepath.Join(site, "hugo.yaml"), []byte("title: Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(site, "content"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, "content", "post.md"), []byte("---\ntags: [Go]\ncategories: [Engineering]\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]string{"root": site, "staging_root": filepath.Join(t.TempDir(), "staging")})
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test','/tmp','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('h1','w1','hugo','Hugo',?,'2026-01-01','2026-01-01')`, string(config))
	if err != nil {
		t.Fatal(err)
	}
	runtime, _ := newProviderRuntime()
	if _, err := RefreshRecentTaxonomy(ctx, db, runtime); err != nil {
		t.Fatalf("刷新最近 taxonomy: %v", err)
	}
	snapshot, _, err := repository.NewTaxonomyRepository(db).GetSnapshot(ctx, "w1", "h1")
	if err != nil || len(snapshot.Terms) != 2 || snapshot.Revision == "" {
		t.Fatalf("taxonomy 未持久化: snapshot=%+v err=%v", snapshot, err)
	}
}
