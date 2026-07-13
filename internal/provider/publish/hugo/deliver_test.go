package hugo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestDeliverReplacesBundleAndReusesCompletedOperation(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	builder := &fakeBuilder{revision: "revision-v1"}
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts", BaseURL: "https://example.com"}, builder)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := provider.Prepare(context.Background(), existingArticleInput())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	result, err := provider.Deliver(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "content", "posts", "existing", "index.md"))
	if !strings.Contains(string(content), "title: 新标题") || result.State != "published" || builder.calls != 2 {
		t.Fatalf("交付结果不正确: result=%+v calls=%d content=%s", result, builder.calls, content)
	}
	second, err := provider.Deliver(context.Background(), artifact)
	if err != nil || second.ProviderRevision != result.ProviderRevision || builder.calls != 2 {
		t.Fatalf("重复 Deliver 未幂等复用: second=%+v calls=%d err=%v", second, builder.calls, err)
	}
}

func TestDeliverRestoresOldBundleWhenRealBuildFails(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	target := filepath.Join(root, "content", "posts", "existing", "index.md")
	before, _ := os.ReadFile(target)
	builder := &fakeBuilder{revision: "revision", failAt: 2}
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts"}, builder)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := provider.Prepare(context.Background(), existingArticleInput())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	_, err = provider.Deliver(context.Background(), artifact)
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "hugo.build_failed" {
		t.Fatalf("真实构建失败未返回稳定错误: %T %v", err, err)
	}
	after, _ := os.ReadFile(target)
	if string(after) != string(before) {
		t.Fatalf("真实构建失败后旧 bundle 未恢复:\n%s", after)
	}
}

func TestDeliverRestoresBackupWhenReplacementFails(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	target := filepath.Join(root, "content", "posts", "existing", "index.md")
	before, _ := os.ReadFile(target)
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts"}, &fakeBuilder{revision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := provider.Prepare(context.Background(), existingArticleInput())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	provider.replace = func(_ string, target, backup string) error {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		return errors.New("replace failed")
	}
	if _, err := provider.Deliver(context.Background(), artifact); err == nil {
		t.Fatal("替换失败应返回错误")
	}
	after, _ := os.ReadFile(target)
	if string(after) != string(before) {
		t.Fatalf("替换失败后旧 bundle 未恢复:\n%s", after)
	}
}

func existingArticleInput() contracts.PublishInput {
	return contracts.PublishInput{
		OperationID: "operation_deliver", ContentHash: "hash-v2", Body: "新正文。",
		Article: article.Article{
			StableID: "article_EXISTING", RelativePath: "文章.md", Title: "新标题",
			Category: "AI应用开发", Tags: []string{"go"}, Slug: "new-slug",
		},
	}
}
