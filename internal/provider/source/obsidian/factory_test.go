package obsidian

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestFactoryBuildsObsidianFromAuthorizedConfig(t *testing.T) {
	t.Parallel()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"root": vault, "content_roots": []string{"Areas"}})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewFactory().Build(context.Background(), contracts.ProviderRef{ID: "source-1", Type: contracts.ProviderObsidian}, contracts.ConfigView{Data: data, AllowedRoots: []string{vault}}, nil)
	if err != nil {
		t.Fatalf("构建 Obsidian Provider: %v", err)
	}
	if provider.Descriptor().Type != contracts.ProviderObsidian {
		t.Fatalf("Provider 类型错误: %s", provider.Descriptor().Type)
	}
}

func TestFactoryRejectsUnauthorizedVault(t *testing.T) {
	t.Parallel()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"root": vault})
	_, err := NewFactory().Build(context.Background(), contracts.ProviderRef{ID: "source-1", Type: contracts.ProviderObsidian}, contracts.ConfigView{Data: data, AllowedRoots: []string{t.TempDir()}}, nil)
	if err == nil {
		t.Fatal("未授权 Vault 应被拒绝")
	}
}
