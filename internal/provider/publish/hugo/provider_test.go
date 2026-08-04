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

func TestPrepareMigratesExistingBundleToDateURLPath(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{revision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := provider.Prepare(context.Background(), contracts.PublishInput{
		OperationID: "operation_rename", ContentHash: "hash", Body: "正文",
		Article: article.Article{StableID: "article_EXISTING", Title: "旧文章", URL: "superpowers", PublishDate: "2026-07-31"},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	want := filepath.Join(root, "content", "posts", "20260731-superpowers")
	old := filepath.Join(root, "content", "posts", "existing")
	if artifact.TargetPath != want || artifact.PreviousTargetPath != old {
		t.Fatalf("旧 Bundle 未迁移到日期 URL 路径: %+v", artifact)
	}
	if _, err := os.Stat(filepath.Join(artifact.Location, "index.md")); err != nil {
		t.Fatalf("新 Bundle staging 不存在: %v", err)
	}
}

func TestPrepareUsesSelectedSectionAndBuildsFileManifest(t *testing.T) {
	root := copyHugoFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "content", "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{revision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := provider.Prepare(context.Background(), contracts.PublishInput{
		OperationID: "operation_notes", ContentHash: "hash", TargetSection: "notes", Body: "正文",
		Article: article.Article{StableID: "article_NEW", Title: "新文章", Slug: "new-article"},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if artifact.TargetPath != filepath.Join(root, "content", "notes", "new-article") || artifact.TargetRelativePath != "content/notes/new-article" || artifact.Change != "added" {
		t.Fatalf("目标 Section 或变更类型错误: %+v", artifact)
	}
	if len(artifact.Files) != 1 || artifact.Files[0].RelativePath != "index.md" || artifact.Files[0].Size == 0 || artifact.Files[0].SHA256 == "" || artifact.Files[0].MediaType != "text/markdown" {
		t.Fatalf("Artifact manifest 错误: %+v", artifact.Files)
	}
	if _, err := os.Stat(filepath.Join(root, "content", "notes", "new-article")); !os.IsNotExist(err) {
		t.Fatal("Prepare 不应修改正式 content")
	}
}

func TestPrepareCreatesPageBundleInSelectedCategoryDirectory(t *testing.T) {
	root := copyHugoFixture(t)
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "ai", "existing", "index.md"), "---\ntitle: Existing\n---\n")
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{revision: "revision"})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := provider.Prepare(context.Background(), contracts.PublishInput{
		OperationID: "operation_category", ContentHash: "hash", TargetSection: "posts", TargetDirectory: "ai", Body: "正文",
		Article: article.Article{StableID: "article_NEW", Title: "新文章", Slug: "new-article"},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	want := filepath.Join(root, "content", "posts", "ai", "new-article")
	if artifact.TargetPath != want || artifact.TargetRelativePath != "content/posts/ai/new-article" {
		t.Fatalf("Page Bundle 分类目标错误: %+v", artifact)
	}
}

func TestBundleSegmentPrefersURLAndPrefixesPublishDate(t *testing.T) {
	got := bundleSegment(contracts.PublishInput{Article: article.Article{
		StableID: "article_NEW", Slug: "same-slug", URL: "superpowers-workflow", PublishDate: "2026-07-30",
	}})
	if got != "20260730-superpowers-workflow" {
		t.Fatalf("URL 和日期未组成排序目录: %q", got)
	}
	if got := bundleSegment(contracts.PublishInput{Article: article.Article{
		StableID: "article_NEW", URL: "20260730-superpowers-workflow", PublishDate: "2026-07-30",
	}}); got != "20260730-superpowers-workflow" {
		t.Fatalf("已有日期前缀不应重复添加: %q", got)
	}
}

func TestCopyTreeSkipsUnrelatedSymbolicLinks(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	writeSectionFixture(t, filepath.Join(source, "hugo.toml"), "baseURL='https://example.com'\n")
	writeSectionFixture(t, filepath.Join(source, "content", "posts", "demo", "index.md"), "---\ntitle: Demo\n---\n")
	external := t.TempDir()
	writeSectionFixture(t, filepath.Join(external, "tool.txt"), "external")
	if err := os.MkdirAll(filepath.Join(source, "tools"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(source, "tools", "markdown-preview")); err != nil {
		t.Fatal(err)
	}

	if err := copyTree(source, target); err != nil {
		t.Fatalf("无关符号链接不应阻断 staging: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "content", "posts", "demo", "index.md")); err != nil {
		t.Fatalf("站点内容未复制: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "tools", "markdown-preview")); !os.IsNotExist(err) {
		t.Fatalf("staging 不应复制或跟随符号链接: %v", err)
	}
}

func TestPrepareLocksExistingArticleToOriginalSection(t *testing.T) {
	root := copyHugoFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "content", "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}
	input := existingArticleInput()
	input.TargetSection = "notes"
	if _, err := provider.Prepare(context.Background(), input); err == nil {
		t.Fatal("已有文章不应跨 Section 移动")
	}
}

func TestPreflightAcceptsTermsManagedByHugoStandardTaxonomy(t *testing.T) {
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
	if !result.Ready {
		t.Fatalf("Hugo 原生 term 不应被私有白名单阻断: %+v", result)
	}
}

func TestPreflightBlocksUnresolvedSourceImage(t *testing.T) {
	t.Parallel()
	provider, err := New(Config{Root: copyHugoFixture(t), StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Preflight(context.Background(), contracts.PublishInput{
		OperationID: "operation_1", ContentHash: "hash", Article: article.Article{StableID: "article_NEW", Title: "文章"},
		Diagnostics: []contracts.Diagnostic{{Code: "source.image_unresolved", Message: "图片引用无法解析", Blocking: false}},
	})
	if err != nil || result.Ready || len(result.Diagnostics) != 1 || !result.Diagnostics[0].Blocking {
		t.Fatalf("unresolved image must block Hugo publication: result=%+v err=%v", result, err)
	}
}

func TestPreflightReportsSpecificMissingArticleFields(t *testing.T) {
	tests := []struct {
		name     string
		input    contracts.PublishInput
		wantCode string
		wantText string
	}{
		{
			name:     "缺少内容版本",
			input:    contracts.PublishInput{OperationID: "operation_content", Article: article.Article{StableID: "article_VALID", Title: "文章"}},
			wantCode: "hugo.content_version_missing",
			wantText: "Hugo 文章缺少内容版本",
		},
		{
			name:     "缺少稳定 ID",
			input:    contracts.PublishInput{OperationID: "operation_identity", ContentHash: "hash", Article: article.Article{Title: "文章"}},
			wantCode: "hugo.stable_id_missing",
			wantText: "Hugo 文章缺少稳定 ID",
		},
		{
			name:     "缺少标题",
			input:    contracts.PublishInput{OperationID: "operation_title", ContentHash: "hash", Article: article.Article{StableID: "article_VALID"}},
			wantCode: "hugo.title_missing",
			wantText: "Hugo 文章缺少标题",
		},
		{
			name:     "稳定 ID 非法",
			input:    contracts.PublishInput{OperationID: "operation_identity_invalid", ContentHash: "hash", Article: article.Article{StableID: "legacy-invalid", Title: "文章"}},
			wantCode: "hugo.stable_id_invalid",
			wantText: "Hugo 文章稳定 ID 格式无效",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := New(Config{Root: copyHugoFixture(t), StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Preflight(context.Background(), test.input)
			if err != nil {
				t.Fatal(err)
			}
			if result.Ready || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != test.wantCode || result.Diagnostics[0].Message != test.wantText || !result.Diagnostics[0].Blocking {
				t.Fatalf("缺失字段诊断不具体: result=%+v", result)
			}
		})
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
		OperationID: "operation_conflict", ContentHash: "hash", TargetSection: "posts", Body: "正文",
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

func TestValidatePreparedArtifactRejectsTamperedIdentity(t *testing.T) {
	t.Parallel()

	provider, err := New(Config{Root: copyHugoFixture(t), StagingRoot: filepath.Join(t.TempDir(), "staging"), Section: "posts"}, &fakeBuilder{revision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	input := existingArticleInput()
	input.OperationID = "operation_validate"
	artifact, err := provider.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.ValidatePreparedArtifact(context.Background(), artifact); err != nil {
		t.Fatalf("有效 Artifact 未通过校验: %v", err)
	}
	artifact.ContentHash = "tampered"
	if err := provider.ValidatePreparedArtifact(context.Background(), artifact); err == nil {
		t.Fatal("篡改后的 Artifact 仍通过校验")
	}
}

func TestValidateDoesNotRequirePrivateTaxonomyFile(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	if err := os.Remove(filepath.Join(root, "data", "taxonomy.yaml")); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Validate(context.Background()); err != nil {
		t.Fatalf("Hugo 标准配置有效时不应依赖私有 taxonomy 文件: %v", err)
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
