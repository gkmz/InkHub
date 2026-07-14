package obsidian

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestResolveAssetSupportsObsidianFolderAndMarkdownRelativePath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".obsidian", "app.json"), []byte(`{"attachmentFolderPath":"assets"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"Areas/note.md", "assets/image.png", "Areas/local.png"} {
		full := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	provider, _ := New(Config{Root: root})
	wiki, err := provider.ResolveAsset(context.Background(), contracts.SourceRef{RelativePath: "Areas/note.md"}, "image.png", AssetWikiEmbed)
	if err != nil || wiki.RelativePath != "assets/image.png" {
		t.Fatalf("Wiki 附件解析错误: asset=%#v err=%v", wiki, err)
	}
	markdown, err := provider.ResolveAsset(context.Background(), contracts.SourceRef{RelativePath: "Areas/note.md"}, "local.png", AssetMarkdownImage)
	if err != nil || markdown.RelativePath != "Areas/local.png" {
		t.Fatalf("Markdown 图片解析错误: asset=%#v err=%v", markdown, err)
	}
}

func TestResolveAssetAllowsRemoteAndRejectsVaultEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider, _ := New(Config{Root: root})
	remote, err := provider.ResolveAsset(context.Background(), contracts.SourceRef{RelativePath: "note.md"}, "https://example.com/image.png", AssetMarkdownImage)
	if err != nil || remote.RemoteURL == "" {
		t.Fatalf("远程图片解析错误: %#v err=%v", remote, err)
	}
	if _, err := provider.ResolveAsset(context.Background(), contracts.SourceRef{RelativePath: "note.md"}, "../outside.png", AssetMarkdownImage); err == nil {
		t.Fatal("Vault 外路径必须被拒绝")
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ResolveAsset(context.Background(), contracts.SourceRef{RelativePath: "note.md"}, "linked.png", AssetMarkdownImage); err == nil {
		t.Fatal("指向 Vault 外的符号链接必须被拒绝")
	}
}
