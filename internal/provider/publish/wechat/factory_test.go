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
}
