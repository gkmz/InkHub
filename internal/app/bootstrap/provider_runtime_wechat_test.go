package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestProviderRuntimeBuildsWeChatGitHubUploader(t *testing.T) {
	t.Parallel()

	runtime, err := newProviderRuntime(staticRuntimeSecret{value: []byte("secret-token")})
	if err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(t.TempDir(), "wechat")
	config, _ := json.Marshal(map[string]any{
		"staging_root": staging, "template": "default", "github_owner": "gkmz",
		"github_repository": "images", "github_branch": "main", "github_prefix": "inkhub", "github_secret_ref": "wechat-token",
	})
	provider, err := runtime.BuildPublish(context.Background(), contracts.ProviderRef{ID: "wechat_1", Type: contracts.ProviderWeChat}, contracts.ConfigView{Data: config, AllowedRoots: []string{filepath.Dir(staging)}})
	if err != nil || provider == nil {
		t.Fatalf("构建微信 GitHub 图片 Provider: provider=%v err=%v", provider, err)
	}
}

func TestConfiguredTemplateFallsBackToDefaultBuiltin(t *testing.T) {
	t.Parallel()

	config, _ := json.Marshal(map[string]string{"template": "classic"})
	ref, err := configuredTemplate(config)
	if err != nil || ref == nil || ref.ID != domaintemplate.BuiltinDefaultID {
		t.Fatalf("旧模板配置未回退到默认模板: ref=%+v err=%v", ref, err)
	}
}

type staticRuntimeSecret struct{ value []byte }

func (resolver staticRuntimeSecret) Resolve(context.Context, string) (contracts.SecretValue, error) {
	return contracts.SecretValue{Bytes: append([]byte(nil), resolver.value...)}, nil
}
