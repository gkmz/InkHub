package markdown

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/registry"
)

func TestMarkdownFolderBuildsThroughRegistryAndReadsStandardFrontmatter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	content := "---\nid: article_MARKDOWN1\ntitle: 普通 Markdown\ndescription: 通用来源\ntags: [Go]\nkeywords: [InkHub]\ncategory: Engineering\n---\n正文"
	if err := os.WriteFile(filepath.Join(root, "article.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := registry.New(nil)
	if err := runtime.RegisterSource(NewFactory()); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"root": root})
	provider, err := runtime.BuildSource(context.Background(), contracts.ProviderRef{ID: "source-md", Type: contracts.ProviderMarkdownFolder}, contracts.ConfigView{Data: data, AllowedRoots: []string{root}})
	if err != nil {
		t.Fatalf("构建 Markdown Folder: %v", err)
	}
	scan, err := provider.Scan(context.Background(), contracts.ScanCursor{})
	if err != nil || len(scan.Documents) != 1 {
		t.Fatalf("扫描 Markdown Folder: result=%+v err=%v", scan, err)
	}
	document, err := provider.Read(context.Background(), scan.Documents[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	if document.Article.Title != "普通 Markdown" || document.Article.Category != "Engineering" || document.Body != "正文" {
		t.Fatalf("标准 Markdown 解析错误: %+v body=%q", document.Article, document.Body)
	}
}

func TestMarkdownFolderRejectsMetadataWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	data, _ := json.Marshal(map[string]string{"root": root})
	provider, err := NewFactory().Build(context.Background(), contracts.ProviderRef{ID: "source-md", Type: contracts.ProviderMarkdownFolder}, contracts.ConfigView{Data: data, AllowedRoots: []string{root}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.WriteMetadata(context.Background(), contracts.MetadataWriteCommand{}); err == nil {
		t.Fatal("只读 Markdown Folder 不应写回元数据")
	}
}

func TestMarkdownFolderRejectsUnauthorizedRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	data, _ := json.Marshal(map[string]string{"root": root})
	if _, err := NewFactory().Build(context.Background(), contracts.ProviderRef{ID: "source-md", Type: contracts.ProviderMarkdownFolder}, contracts.ConfigView{Data: data, AllowedRoots: []string{t.TempDir()}}, nil); err == nil {
		t.Fatal("未授权 Markdown Folder 应被拒绝")
	}
}

func TestMarkdownFolderSupportsCRLFAndRejectsStableIDConflict(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "windows.md"), []byte("---\r\nid: article_WINDOWS1\r\ntitle: Windows\r\n---\r\n正文"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{SourceID: "source-md", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	document, err := provider.Read(context.Background(), contracts.SourceRef{SourceID: "source-md", RelativePath: "windows.md"})
	if err != nil || document.Article.Title != "Windows" || document.Body != "正文" {
		t.Fatalf("CRLF Markdown 解析错误: document=%+v err=%v", document, err)
	}
	if _, err := provider.Read(context.Background(), contracts.SourceRef{SourceID: "source-md", RelativePath: "windows.md", StableID: "article_OTHER"}); err == nil {
		t.Fatal("StableID 冲突应被拒绝")
	}
}
