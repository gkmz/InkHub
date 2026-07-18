package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestWeChatSettingsSaveGitHubTokenWithoutExposingIt(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试','/tmp','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO sources(id,workspace_id,provider_type,root_path,config_json,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','{}','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	secrets := &memoryAISecretStore{values: map[string]string{}}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{AISecrets: secrets, DataDir: t.TempDir(), ProviderRuntime: diagnosticWeChatRuntime{provider: diagnosticPublishProvider{err: errors.New("private repository secret body")}}})
	request := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/settings/wechat", strings.NewReader(`{"enabled":true,"template":"default","github_owner":"gkmz","github_repository":"images","github_branch":"main","github_prefix":"inkhub","github_token":"secret-token"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret-token") {
		t.Fatalf("保存微信设置响应错误: %d %s", response.Code, response.Body.String())
	}
	var config string
	if err := db.QueryRow(`SELECT config_json FROM provider_instances WHERE workspace_id='w1' AND provider_type='wechat'`).Scan(&config); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(config, "secret-token") || !strings.Contains(config, `"github_owner":"gkmz"`) || len(secrets.values) != 1 {
		t.Fatalf("微信配置或 Secret 存储错误: config=%s secrets=%v", config, secrets.values)
	}

	view := httptest.NewRecorder()
	handler.ServeHTTP(view, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/settings", nil))
	if !strings.Contains(view.Body.String(), `"github_token_saved":true`) || strings.Contains(view.Body.String(), "secret-token") {
		t.Fatalf("微信设置视图错误: %s", view.Body.String())
	}
	if !strings.Contains(view.Body.String(), `"name":"微信图片仓库"`) || !strings.Contains(view.Body.String(), `"state":"需要处理"`) || strings.Contains(view.Body.String(), "private repository secret body") {
		t.Fatalf("微信诊断视图错误: %s", view.Body.String())
	}
}

type diagnosticWeChatRuntime struct{ provider contracts.PublishProvider }

func (runtime diagnosticWeChatRuntime) SupportsTaxonomy(contracts.ProviderType) bool { return false }
func (runtime diagnosticWeChatRuntime) BuildSource(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.SourceProvider, error) {
	return nil, nil
}
func (runtime diagnosticWeChatRuntime) BuildAI(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.AIProvider, error) {
	return nil, nil
}
func (runtime diagnosticWeChatRuntime) BuildPublish(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.PublishProvider, error) {
	return runtime.provider, nil
}
func (runtime diagnosticWeChatRuntime) BuildTaxonomy(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.TaxonomyProvider, error) {
	return nil, nil
}

type diagnosticPublishProvider struct{ err error }

func (provider diagnosticPublishProvider) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{}
}
func (provider diagnosticPublishProvider) Validate(context.Context) error { return provider.err }
func (diagnosticPublishProvider) Preflight(context.Context, contracts.PublishInput) (contracts.PreflightResult, error) {
	return contracts.PreflightResult{}, nil
}
func (diagnosticPublishProvider) Prepare(context.Context, contracts.PublishInput) (contracts.PreparedArtifact, error) {
	return contracts.PreparedArtifact{}, nil
}
func (diagnosticPublishProvider) Deliver(context.Context, contracts.PreparedArtifact) (contracts.DeliveryResult, error) {
	return contracts.DeliveryResult{}, nil
}
