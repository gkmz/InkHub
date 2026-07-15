package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

func TestTaxonomyOverviewReturnsPersistedTermsAndStatus(t *testing.T) {
	t.Parallel()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := taxonomySite(t)
	config, _ := json.Marshal(map[string]string{"root": root, "staging_root": filepath.Join(t.TempDir(), "staging")})
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test','/tmp','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('h1','w1','hugo','我的博客',?,'2026-01-01','2026-01-01')`, string(config))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := contracts.TaxonomySnapshot{ProviderRef: contracts.ProviderRef{ID: "h1", Type: contracts.ProviderHugo}, Revision: "revision-1", Complete: true, Terms: []contracts.TaxonomyTerm{{Kind: "category", Key: "engineering", Name: "Engineering", CanonicalName: "Engineering", UsageCount: 3}}}
	if err := repository.NewTaxonomyRepository(db).ReplaceSnapshot(context.Background(), "w1", snapshot, time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/taxonomy", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"ready"`) || !strings.Contains(response.Body.String(), `"name":"Engineering"`) || !strings.Contains(response.Body.String(), `"usage_count":3`) || !strings.Contains(response.Body.String(), `"source":"我的博客"`) {
		t.Fatalf("taxonomy 快照响应错误: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTaxonomyRefreshInvokesApplicationCallback(t *testing.T) {
	t.Parallel()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	called := 0
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{RefreshTaxonomy: func(context.Context) error { called++; return nil }})
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/taxonomy/refresh", strings.NewReader("{}"))
	request.Header.Set("Origin", "http://localhost")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || called != 1 {
		t.Fatalf("taxonomy 刷新未执行: code=%d calls=%d body=%s", response.Code, called, response.Body.String())
	}
}

func taxonomySite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hugo.yaml"), []byte("title: Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
