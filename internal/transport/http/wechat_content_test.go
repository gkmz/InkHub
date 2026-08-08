package httptransport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestWeChatContentReturnsOnlyCurrentVersionFromEnabledProvider(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	currentJob := "job_current_http"
	staleJob := "job_stale_http"
	currentLocation := filepath.Join(root, currentJob, "content.html")
	staleLocation := filepath.Join(root, staleJob, "content.html")
	writeWeChatArtifactFixture(t, root, currentJob, "hash-current", []byte("<p>当前版本</p>"), currentLocation)
	writeWeChatArtifactFixture(t, root, staleJob, "hash-old", []byte("<p>旧版本</p>"), staleLocation)
	now := "2026-08-08T00:00:00Z"
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试','/tmp',?,?,?)`, []any{now, now, now}},
		{`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp',?,?)`, []any{now, now}},
		{`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,frontmatter_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','article_WECHAT','article.md','微信文章','hash-current','front',?,?,?)`, []any{now, now, now}},
		{`INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at) VALUES('wx1','w1','wechat','微信',1,?,?,?)`, []any{mustJSON(t, map[string]string{"staging_root": root}), now, now}},
		{`INSERT INTO jobs(id,workspace_id,kind,state,progress,payload_json,result_json,available_at,finished_at,created_at,updated_at) VALUES(?, 'w1','wechat_prepare','succeeded',100,?,?,?,?,?,?)`, []any{staleJob, mustJSON(t, map[string]string{"article_id": "a1", "provider_instance_id": "wx1", "content_hash": "hash-old"}), mustJSON(t, map[string]string{"location": staleLocation}), now, "2026-08-08T02:00:00Z", now, now}},
		{`INSERT INTO jobs(id,workspace_id,kind,state,progress,payload_json,result_json,available_at,finished_at,created_at,updated_at) VALUES(?, 'w1','wechat_prepare','succeeded',100,?,?,?,?,?,?)`, []any{currentJob, mustJSON(t, map[string]string{"article_id": "a1", "provider_instance_id": "wx1", "content_hash": "hash-current"}), mustJSON(t, map[string]string{"location": currentLocation}), now, "2026-08-08T01:00:00Z", now, now}},
	}
	for _, item := range queries {
		if _, err := db.Exec(item.query, item.args...); err != nil {
			t.Fatal(err)
		}
	}

	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/wechat/content/a1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "当前版本") || strings.Contains(response.Body.String(), "旧版本") {
		t.Fatalf("微信接口未限定当前版本: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestReadVerifiedWeChatContentAcceptsCurrentUntamperedArtifact(t *testing.T) {
	root := t.TempDir()
	jobID := "job_current"
	operationRoot := filepath.Join(root, jobID)
	content := []byte("<p>当前微信内容</p>")
	writeWeChatArtifactFixture(t, root, jobID, "hash-current", content, filepath.Join(operationRoot, "content.html"))

	got, err := readVerifiedWeChatContent(jobID, "hash-current", mustJSON(t, map[string]string{"staging_root": root}), mustJSON(t, map[string]string{"location": filepath.Join(operationRoot, "content.html")}))
	if err != nil || string(got) != string(content) {
		t.Fatalf("当前微信产物读取失败: content=%q err=%v", got, err)
	}
}

func TestReadVerifiedWeChatContentRejectsStaleTamperedAndEscapedArtifact(t *testing.T) {
	root := t.TempDir()
	jobID := "job_rejected"
	operationRoot := filepath.Join(root, jobID)
	content := []byte("<p>原始内容</p>")
	writeWeChatArtifactFixture(t, root, jobID, "hash-current", content, filepath.Join(operationRoot, "content.html"))
	config := mustJSON(t, map[string]string{"staging_root": root})

	if _, err := readVerifiedWeChatContent(jobID, "hash-old", config, mustJSON(t, map[string]string{"location": filepath.Join(operationRoot, "content.html")})); err == nil {
		t.Fatal("旧版本微信产物不应被读取")
	}

	if err := os.WriteFile(filepath.Join(operationRoot, "content.html"), []byte("<p>被篡改</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readVerifiedWeChatContent(jobID, "hash-current", config, mustJSON(t, map[string]string{"location": filepath.Join(operationRoot, "content.html")})); err == nil {
		t.Fatal("被篡改的微信产物不应被读取")
	}

	escaped := filepath.Join(root, "outside.html")
	if err := os.WriteFile(escaped, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readVerifiedWeChatContent(jobID, "hash-current", config, mustJSON(t, map[string]string{"location": escaped})); err == nil {
		t.Fatal("指向 staging 外部的微信产物不应被读取")
	}
}

func writeWeChatArtifactFixture(t *testing.T, root, jobID, contentHash string, content []byte, location string) {
	t.Helper()
	operationRoot := filepath.Join(root, jobID)
	if err := os.MkdirAll(operationRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	manifest := map[string]any{"artifact": map[string]string{"OperationID": jobID, "ContentHash": contentHash, "Location": location}, "html_digest": hex.EncodeToString(sum[:])}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(operationRoot, "artifact.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
