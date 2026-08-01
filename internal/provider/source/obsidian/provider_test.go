package obsidian

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestProviderReadCollectsLocalImagesAndKeepsRemoteImages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"notes/local.png", "wiki.png"} {
		file, err := os.Create(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
	}
	body := "![本地](local.png)\n\n![[wiki.png]]\n\n![远程](https://example.com/remote.png)"
	if err := os.WriteFile(filepath.Join(root, "notes", "article.md"), []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "notes/article.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.ResourceRefs) != 2 || document.ResourceRefs[0].Original != "local.png" || document.ResourceRefs[1].Original != "wiki.png" {
		t.Fatalf("本地图片引用错误: %+v", document.ResourceRefs)
	}
	if !strings.Contains(document.Body, "https://example.com/remote.png") {
		t.Fatal("远程图片正文被修改")
	}
}

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
	if document.Article.ContentStage != article.ContentStageDraft {
		t.Fatalf("missing publish.status must default to draft: %#v", document.Article)
	}
	if !strings.Contains(document.Body, "[[内部链接]]") {
		t.Fatal("Read() must preserve the Markdown body")
	}
}

func TestProviderReadsReadyContentStage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Ready\npublish:\n  status: ready\n---\n正文\n"
	if err := os.WriteFile(filepath.Join(root, "ready.md"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "ready.md"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Article.ContentStage != article.ContentStageReady || document.Article.ContentStageIssue != "" {
		t.Fatalf("ready stage = %q, issue = %q", document.Article.ContentStage, document.Article.ContentStageIssue)
	}
}

func TestProviderResolvesNestedRelativeWikiImageWithoutBlockingArticle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	articleDir := filepath.Join(root, "B-Area", "01-工程大脑", "AI", "效率工具")
	if err := os.MkdirAll(filepath.Join(root, ".obsidian", ""), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(articleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "20260731.png"), []byte("image"), 0o640); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Ready\npublish:\n  status: ready\n---\n![[../../../../assets/20260731.png]]\n"
	if err := os.WriteFile(filepath.Join(articleDir, "article.md"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "B-Area/01-工程大脑/AI/效率工具/article.md"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Article.ContentStage != article.ContentStageReady || len(document.Diagnostics) != 0 || len(document.ResourceRefs) != 1 {
		t.Fatalf("嵌套相对图片不应阻塞已就绪文章: stage=%s diagnostics=%+v resources=%+v", document.Article.ContentStage, document.Diagnostics, document.ResourceRefs)
	}
}

func TestProviderTreatsInvalidContentStageAsNonBlockingDraft(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Invalid\npublish:\n  status: published\n---\n正文\n"
	if err := os.WriteFile(filepath.Join(root, "invalid-stage.md"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	document, err := provider.Read(context.Background(), contracts.SourceRef{RelativePath: "invalid-stage.md"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Article.ContentStage != article.ContentStageDraft || document.Article.ContentStageIssue == "" {
		t.Fatalf("invalid stage = %q, issue = %q", document.Article.ContentStage, document.Article.ContentStageIssue)
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
	if document.Article.Tags == nil || document.Article.Keywords == nil {
		t.Fatalf("标准列表字段不得为 nil: tags=%#v keywords=%#v", document.Article.Tags, document.Article.Keywords)
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
