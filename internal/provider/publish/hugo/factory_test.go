package hugo

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestFactoryBuildsAuthorizedHugoProviderAndRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	staging := filepath.Join(t.TempDir(), "staging")
	factory := NewFactory(&fakeBuilder{})
	config, _ := json.Marshal(map[string]any{
		"root": root, "staging_root": staging, "section": "posts", "artifact_ttl_seconds": 3600,
	})
	provider, err := factory.Build(context.Background(), contracts.ProviderRef{ID: "hugo_1", Type: contracts.ProviderHugo}, contracts.ConfigView{
		Data: config, AllowedRoots: []string{root, filepath.Dir(staging)},
	}, nil)
	if err != nil || provider.Descriptor().Type != contracts.ProviderHugo {
		t.Fatalf("构建 Hugo Provider: provider=%v err=%v", provider, err)
	}
	if factory.Descriptor().Type != contracts.ProviderHugo || factory.Descriptor().ConfigSchema == "" {
		t.Fatalf("Hugo Descriptor 不完整: %+v", factory.Descriptor())
	}
	if provider.Descriptor().DeliveryMode != contracts.DeliveryAutomatic {
		t.Fatalf("构建后的 Hugo 交付模式错误: %s", provider.Descriptor().DeliveryMode)
	}

	invalid := json.RawMessage(`{"root":"` + root + `","staging_root":"` + staging + `","unknown":true}`)
	_, err = factory.Build(context.Background(), contracts.ProviderRef{ID: "hugo_1", Type: contracts.ProviderHugo}, contracts.ConfigView{
		Data: invalid, AllowedRoots: []string{root, filepath.Dir(staging)},
	}, nil)
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != contracts.ErrorValidation {
		t.Fatalf("未知配置字段应被拒绝: %T %v", err, err)
	}
}

func TestFactoryRejectsUnauthorizedHugoRoot(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	staging := filepath.Join(t.TempDir(), "staging")
	config, _ := json.Marshal(map[string]any{"root": root, "staging_root": staging})
	_, err := NewFactory(&fakeBuilder{}).Build(context.Background(), contracts.ProviderRef{ID: "hugo_1", Type: contracts.ProviderHugo}, contracts.ConfigView{
		Data: config, AllowedRoots: []string{t.TempDir()},
	}, nil)
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != contracts.ErrorUnauthorizedResource {
		t.Fatalf("未授权路径应被拒绝: %T %v", err, err)
	}
}

func TestFactoryDeclaresAutomaticDelivery(t *testing.T) {
	t.Parallel()
	if mode := NewFactory(&fakeBuilder{}).Descriptor().DeliveryMode; mode != contracts.DeliveryAutomatic {
		t.Fatalf("Hugo 交付模式错误: %s", mode)
	}
}
