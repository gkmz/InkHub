package obsidian

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestProviderReadsFixedFrontmatterAndChinesePath(t *testing.T) {
	t.Parallel()

	root := copyFixtureVault(t, "valid")
	provider, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "文章.md"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if document.Article.StableID != "article_01J2ABCDEF" || document.Article.Keywords[0] != "Markdown 发布" {
		t.Fatalf("Read() article = %#v", document.Article)
	}
	if !strings.Contains(document.Body, "[[内部链接]]") {
		t.Fatal("Read() must preserve the Markdown body")
	}
}

func TestProviderWritesMetadataWithoutChangingBodyOrUnknownFieldOrder(t *testing.T) {
	t.Parallel()

	root := copyFixtureVault(t, "valid")
	provider, _ := New(Config{Root: root})
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "文章.md"})
	if err != nil {
		t.Fatal(err)
	}
	written, err := provider.WriteMetadata(context.Background(), contracts.MetadataWriteCommand{
		Ref:                 document.Ref,
		ExpectedFingerprint: document.Fingerprint,
		Patch:               contracts.MetadataPatch{Title: ptr("新标题"), Keywords: &[]string{"SEO", "InkHub"}},
	})
	if err != nil {
		t.Fatalf("WriteMetadata() error = %v", err)
	}
	if written.Body != document.Body {
		t.Fatal("WriteMetadata() changed the Markdown body")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "文章.md"))
	text := string(raw)
	if !strings.Contains(text, "custom: keep-me") {
		t.Fatal("WriteMetadata() removed an unknown frontmatter field")
	}
	if strings.Index(text, "custom:") > strings.Index(text, "description:") {
		t.Fatal("WriteMetadata() changed unknown field order")
	}
}

func TestProviderRejectsStaleMetadataWrite(t *testing.T) {
	t.Parallel()

	root := copyFixtureVault(t, "valid")
	provider, _ := New(Config{Root: root})
	document, _ := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "文章.md"})
	path := filepath.Join(root, "文章.md")
	file, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = file.WriteString("\n外部修改")
	file.Close()

	_, err := provider.WriteMetadata(context.Background(), contracts.MetadataWriteCommand{
		Ref: document.Ref, ExpectedFingerprint: document.Fingerprint, Patch: contracts.MetadataPatch{Title: ptr("覆盖")},
	})
	if !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("WriteMetadata() error = %v, want ErrSourceChanged", err)
	}
}

func TestProviderRejectsConflictingPathAndStableID(t *testing.T) {
	t.Parallel()

	root := copyFixtureVault(t, "valid")
	provider, _ := New(Config{Root: root})
	_, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "文章.md", StableID: "article_OTHER"})
	if !errors.Is(err, ErrSourceConflict) {
		t.Fatalf("Read() error = %v, want ErrSourceConflict", err)
	}
}

func TestProviderReadsPlainMarkdownWithFilenameTitle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "普通笔记.md"), []byte("# 正文标题\n\n正文"), 0o640); err != nil {
		t.Fatal(err)
	}
	provider, _ := New(Config{Root: root})
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "普通笔记.md"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if document.Article.Title != "普通笔记" || document.Body != "# 正文标题\n\n正文" || document.RawFrontmatter != "" {
		t.Fatalf("普通 Markdown 解析错误: %#v", document)
	}
}

func TestProviderNormalizesSingleTagAndFallsBackEmptyTitle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: \ntags: go\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(root, "草稿.md"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	provider, _ := New(Config{Root: root})
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "草稿.md"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if document.Article.Title != "草稿" || len(document.Article.Tags) != 1 || document.Article.Tags[0] != "go" {
		t.Fatalf("单值标签或标题回退错误: %#v", document.Article)
	}
}

func TestProviderRejectsInvalidFrontmatterClosingDelimiter(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: invalid\n---suffix\nbody\n"
	if err := os.WriteFile(filepath.Join(root, "invalid.md"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	provider, _ := New(Config{Root: root})
	if _, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "invalid.md"}); err == nil {
		t.Fatal("Read() must reject a non-delimiter line beginning with dashes")
	}
}

func copyFixtureVault(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join("..", "..", "..", "..", "testdata", "obsidian", name, "文章.md")
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "文章.md"), content, 0o640); err != nil {
		t.Fatal(err)
	}
	return root
}

func ptr(value string) *string { return &value }
