package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestPublicationSettingsEndpointNormalizesAndReturnsExcludedSections(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-01-01T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试',?,?,?,?)`, t.TempDir(), now, now, now); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	request := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/settings/publication-content", strings.NewReader(`{"excluded_sections":[" 相关链接 ","参考资料","相关链接"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != `{"excluded_sections":["相关链接","参考资料"]}`+"\n" {
		t.Fatalf("发布内容设置响应错误: code=%d body=%s", response.Code, response.Body.String())
	}
	var stored string
	if err := db.QueryRow(`SELECT value_json FROM settings WHERE workspace_id='w1' AND key='publication_content'`).Scan(&stored); err != nil || !strings.Contains(stored, `"相关链接"`) {
		t.Fatalf("发布内容设置没有持久化: value=%s err=%v", stored, err)
	}
}
