package hugo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestPrepareBuildsStagingWithoutChangingRealBundleAndIsIdempotent(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	staging := filepath.Join(t.TempDir(), "staging")
	builder := &fakeBuilder{revision: "build-v1"}
	provider, err := New(Config{
		Root: root, StagingRoot: staging, Section: "posts", BaseURL: "https://example.com/", ArtifactTTL: time.Hour,
	}, builder)
	if err != nil {
		t.Fatalf("创建 Hugo Provider: %v", err)
	}
	targetIndex := filepath.Join(root, "content", "posts", "existing", "index.md")
	before, _ := os.ReadFile(targetIndex)
	input := contracts.PublishInput{
		OperationID: "operation_1", ContentHash: "hash-v2", Body: "新正文。",
		Article: article.Article{
			StableID: "article_EXISTING", RelativePath: "文章.md", Title: "新标题", Category: "AI应用开发",
			Series: "InkHub 开发日志", Tags: []string{"go"}, Keywords: []string{"Hugo"}, Slug: "ignored-new-slug",
		},
	}
	artifact, err := provider.Prepare(context.Background(), input)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	after, _ := os.ReadFile(targetIndex)
	if string(after) != string(before) {
		t.Fatal("Prepare 修改了真实 Hugo bundle")
	}
	if !strings.HasPrefix(artifact.Location, staging+string(filepath.Separator)) || artifact.TargetPath != filepath.Dir(targetIndex) {
		t.Fatalf("Artifact 路径不正确: %+v", artifact)
	}
	stagedIndex := filepath.Join(artifact.Location, "index.md")
	staged, err := os.ReadFile(stagedIndex)
	if err != nil || !strings.Contains(string(staged), "title: 新标题") {
		t.Fatalf("staging bundle 未生成: content=%s err=%v", staged, err)
	}
	if artifact.PreviewURL != "https://example.com/posts/existing/" || builder.calls != 1 {
		t.Fatalf("预览地址或构建次数不正确: artifact=%+v calls=%d", artifact, builder.calls)
	}

	second, err := provider.Prepare(context.Background(), input)
	if err != nil || second.Location != artifact.Location || builder.calls != 1 {
		t.Fatalf("重复 OperationID 未复用 artifact: second=%+v calls=%d err=%v", second, builder.calls, err)
	}
}

func TestPreflightRejectsUnknownTaxonomy(t *testing.T) {
	t.Parallel()

	provider, err := New(Config{
		Root: copyHugoFixture(t), StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts",
	}, &fakeBuilder{})
	if err != nil {
		t.Fatalf("创建 Hugo Provider: %v", err)
	}
	result, err := provider.Preflight(context.Background(), contracts.PublishInput{
		OperationID: "operation_1", ContentHash: "hash", Article: article.Article{
			StableID: "article_NEW", Title: "文章", Category: "不存在分类", Tags: []string{"unknown"},
		},
	})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if result.Ready || len(result.Diagnostics) == 0 || !result.Diagnostics[0].Blocking {
		t.Fatalf("未知 taxonomy 未阻断: %+v", result)
	}
}

func TestPreparePreservesRealSiteWhenStagingBuildFails(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	target := filepath.Join(root, "content", "posts", "existing", "index.md")
	before, _ := os.ReadFile(target)
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts"}, &fakeBuilder{err: errors.New("build failed")})
	if err != nil {
		t.Fatalf("创建 Hugo Provider: %v", err)
	}
	_, err = provider.Prepare(context.Background(), contracts.PublishInput{
		OperationID: "operation_1", ContentHash: "hash", Body: "新正文",
		Article: article.Article{StableID: "article_EXISTING", Title: "新标题", Category: "AI应用开发", Tags: []string{"go"}},
	})
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "hugo.build_failed" {
		t.Fatalf("构建失败未映射为 ProviderError: %T %v", err, err)
	}
	after, _ := os.ReadFile(target)
	if string(after) != string(before) {
		t.Fatal("staging 构建失败修改了真实站点")
	}
}

func TestPrepareRejectsSlugOwnedByDifferentSourceID(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts"}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Prepare(context.Background(), contracts.PublishInput{
		OperationID: "operation_conflict", ContentHash: "hash", Body: "正文",
		Article: article.Article{
			StableID: "article_DIFFERENT", Title: "另一篇文章", Slug: "existing",
			Category: "AI应用开发", Tags: []string{"go"},
		},
	})
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "hugo.bundle_conflict" {
		t.Fatalf("slug 冲突应被拒绝: %T %v", err, err)
	}
}

func TestPrepareRejectsOperationIDReusedForDifferentContent(t *testing.T) {
	t.Parallel()

	provider, err := New(Config{
		Root: copyHugoFixture(t), StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts",
	}, &fakeBuilder{revision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	input := existingArticleInput()
	input.OperationID = "operation_conflicting_hash"
	if _, err := provider.Prepare(context.Background(), input); err != nil {
		t.Fatalf("首次 Prepare: %v", err)
	}
	input.ContentHash = "different-hash"
	_, err = provider.Prepare(context.Background(), input)
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != contracts.ErrorConflict {
		t.Fatalf("OperationID 复用不同内容应冲突: %T %v", err, err)
	}
}

func TestValidateRejectsDuplicateTaxonomyValues(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	taxonomyPath := filepath.Join(root, "data", "taxonomy.yaml")
	content, _ := os.ReadFile(taxonomyPath)
	content = []byte(strings.Replace(string(content), "  - AI应用开发\n", "  - AI应用开发\n  - AI应用开发\n", 1))
	if err := os.WriteFile(taxonomyPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Validate(context.Background()); err == nil {
		t.Fatal("重复 Category 应使 taxonomy 校验失败")
	}
}

type fakeBuilder struct {
	calls    int
	revision string
	err      error
	failAt   int
}

func (b *fakeBuilder) Build(_ context.Context, root string) (string, error) {
	b.calls++
	if _, err := os.Stat(filepath.Join(root, "hugo.yaml")); err != nil {
		return "", err
	}
	if b.failAt > 0 && b.calls == b.failAt {
		return "", errors.New("build failed at configured call")
	}
	return b.revision, b.err
}

func copyHugoFixture(t *testing.T) string {
	t.Helper()
	source := filepath.Join("..", "..", "..", "..", "testdata", "hugo", "site")
	target := filepath.Join(t.TempDir(), "site")
	if err := copyTree(source, target); err != nil {
		t.Fatalf("复制 Hugo fixture: %v", err)
	}
	return target
}
