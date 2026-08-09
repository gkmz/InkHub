package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestSaveContentScopeReconcilesMovedArticlesOnFirstRequest(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	for _, directory := range []string{".obsidian", "Areas/new"} {
		if err := os.MkdirAll(filepath.Join(vault, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, stableID := range map[string]string{
		"Areas/new/one.md": "article_ONE",
		"Areas/new/two.md": "article_TWO",
	} {
		content := "---\nid: " + stableID + "\n---\n正文\n"
		if err := os.WriteFile(filepath.Join(vault, filepath.FromSlash(path)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := "2026-01-01T00:00:00Z"
	_, err = db.Exec(`
INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试',?,?,?,?);
INSERT INTO sources(id,workspace_id,provider_type,root_path,config_json,created_at,updated_at) VALUES('s1','w1','obsidian',?,'{"content_roots":["Areas"]}',?,?);
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,indexed_at,created_at,updated_at) VALUES
('a1','w1','s1','article_ONE','Areas/old/one.md',?,?,?),
('a2','w1','s1','article_TWO','Areas/new/one.md',?,?,?);`,
		vault, now, now, now, vault, now, now, now, now, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	request := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/settings/content-scope", strings.NewReader(`{"content_roots":["Areas"],"ignored_folders":[],"ignored_file_names":["index.md","_index.md"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"indexed":2`) {
		t.Fatalf("首次保存内容范围失败: code=%d body=%s", response.Code, response.Body.String())
	}
	for stableID, wantPath := range map[string]string{"article_ONE": "Areas/new/one.md", "article_TWO": "Areas/new/two.md"} {
		var gotPath string
		if err := db.QueryRow(`SELECT relative_path FROM articles WHERE stable_id=? AND deleted_at IS NULL`, stableID).Scan(&gotPath); err != nil || gotPath != wantPath {
			t.Fatalf("%s 路径 = %q, err=%v，期望 %q", stableID, gotPath, err, wantPath)
		}
	}
}
