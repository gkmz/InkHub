package wechat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestFactoryBuildsAuthorizedWeChatProvider(t *testing.T) {
	t.Parallel()

	staging := filepath.Join(t.TempDir(), "staging")
	config, _ := json.Marshal(map[string]any{"staging_root": staging})
	factory := NewFactory(staticLoader{template: renderTemplate("default", `.inkhub-root p { color: #111111; }`)}, nil, &memoryClipboard{})
	provider, err := factory.Build(context.Background(), contracts.ProviderRef{ID: "wechat_1", Type: contracts.ProviderWeChat}, contracts.ConfigView{
		Data: config, AllowedRoots: []string{filepath.Dir(staging)},
	}, nil)
	if err != nil || provider.Descriptor().Type != contracts.ProviderWeChat {
		t.Fatalf("构建 WeChat Provider: provider=%v err=%v", provider, err)
	}
	if provider.Descriptor().DeliveryMode != contracts.DeliveryPrepareOnly {
		t.Fatalf("构建后的微信交付模式错误: %s", provider.Descriptor().DeliveryMode)
	}
}

func TestFactoryDeclaresPrepareOnlyDelivery(t *testing.T) {
	t.Parallel()
	if mode := NewFactory(staticLoader{}, nil, nil).Descriptor().DeliveryMode; mode != contracts.DeliveryPrepareOnly {
		t.Fatalf("微信交付模式错误: %s", mode)
	}
}

func TestFactoryBuildsConfiguredAssetUploaderFromSecret(t *testing.T) {
	t.Parallel()

	staging := filepath.Join(t.TempDir(), "staging")
	config, _ := json.Marshal(map[string]any{
		"staging_root": staging, "github_owner": "gkmz", "github_repository": "images",
		"github_branch": "main", "github_prefix": "inkhub", "github_secret_ref": "wechat-token",
	})
	called := false
	factory := NewFactoryWithUploaderBuilder(staticLoader{}, &memoryClipboard{}, func(_ context.Context, value AssetUploaderConfig, token []byte) (AssetUploader, error) {
		called = true
		if value.Owner != "gkmz" || value.Repository != "images" || string(token) != "secret-value" {
			t.Fatalf("上传器配置错误: config=%+v token=%q", value, token)
		}
		return staticUploader{}, nil
	})
	_, err := factory.Build(context.Background(), contracts.ProviderRef{ID: "wechat_1", Type: contracts.ProviderWeChat}, contracts.ConfigView{
		Data: config, AllowedRoots: []string{filepath.Dir(staging)},
	}, staticSecretResolver{value: []byte("secret-value")})
	if err != nil || !called {
		t.Fatalf("未使用 Secret 构建上传器: called=%v err=%v", called, err)
	}
}

type staticSecretResolver struct{ value []byte }

func (resolver staticSecretResolver) Resolve(context.Context, string) (contracts.SecretValue, error) {
	return contracts.SecretValue{Bytes: resolver.value}, nil
}
