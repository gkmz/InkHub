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

func TestAISettingsSavesProviderWithoutExposingSecret(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试','/tmp','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	secrets := &memoryAISecretStore{values: map[string]string{}}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{AISecrets: secrets})
	request := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/settings/ai", strings.NewReader(`{"enabled":true,"base_url":"https://ai.example.com/v1","model":"model-1","api_key":"secret-value"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret-value") {
		t.Fatalf("保存 AI 设置响应错误: %d %s", response.Code, response.Body.String())
	}
	var config string
	if err := db.QueryRow(`SELECT config_json FROM provider_instances WHERE workspace_id='w1' AND provider_type='openai-compatible'`).Scan(&config); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(config, "secret-value") || !strings.Contains(config, `"model":"model-1"`) || len(secrets.values) != 1 {
		t.Fatalf("AI 配置或 Secret 存储错误: config=%s secrets=%v", config, secrets.values)
	}

	view := httptest.NewRecorder()
	handler.ServeHTTP(view, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/settings", nil))
	if !strings.Contains(view.Body.String(), `"ai_enabled":true`) || !strings.Contains(view.Body.String(), `"ai_secret_saved":true`) || strings.Contains(view.Body.String(), "secret-value") {
		t.Fatalf("AI 设置视图错误: %s", view.Body.String())
	}
}

type memoryAISecretStore struct{ values map[string]string }

func (s *memoryAISecretStore) Get(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}
func (s *memoryAISecretStore) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}
func (s *memoryAISecretStore) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}
