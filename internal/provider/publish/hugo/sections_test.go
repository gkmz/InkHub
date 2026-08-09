package hugo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSectionsFiltersUnsafeEntriesAndCountsMarkdown(t *testing.T) {
	root := t.TempDir()
	writeSectionFixture(t, filepath.Join(root, "hugo.toml"), "baseURL='https://example.com'\n")
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "one.md"), "---\ntitle: One\n---\n")
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "nested", "two.md"), "---\ntitle: Two\n---\n")
	writeSectionFixture(t, filepath.Join(root, "content", "notes", "readme.txt"), "not markdown")
	writeSectionFixture(t, filepath.Join(root, "content", ".hidden", "hidden.md"), "hidden")
	writeSectionFixture(t, filepath.Join(root, "content", "plain-file"), "file")
	if err := os.Symlink(filepath.Join(root, "content", "posts"), filepath.Join(root, "content", "linked")); err != nil {
		t.Fatal(err)
	}
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}

	discovery, err := provider.DiscoverSections(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverSections() error = %v", err)
	}
	if len(discovery.Sections) != 2 || discovery.Sections[0].Name != "notes" || discovery.Sections[0].ArticleCount != 0 || discovery.Sections[1].Name != "posts" || discovery.Sections[1].ArticleCount != 2 {
		t.Fatalf("DiscoverSections() = %+v", discovery)
	}
}

func TestFindBundleAcrossSectionsReturnsOriginalSection(t *testing.T) {
	root := t.TempDir()
	writeSectionFixture(t, filepath.Join(root, "content", "notes", "existing", "index.md"), "---\nsource_id: article_ONE\n---\n")
	target, section, found, err := FindBundleBySourceID(root, "article_ONE")
	if err != nil || !found || section != "notes" || target != filepath.Join(root, "content", "notes", "existing") {
		t.Fatalf("FindBundleBySourceID() = %q, %q, %v, %v", target, section, found, err)
	}
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "duplicate", "index.md"), "---\nsource_id: article_ONE\n---\n")
	if _, _, _, err := FindBundleBySourceID(root, "article_ONE"); err == nil {
		t.Fatal("重复 source_id 应阻止目标选择")
	}
}

func TestFindPublishedBundleExcludesCascadeDrafts(t *testing.T) {
	root := t.TempDir()
	writeSectionFixture(t, filepath.Join(root, "content", "drafts", "_index.md"), "---\ncascade:\n  draft: true\n---\n")
	writeSectionFixture(t, filepath.Join(root, "content", "drafts", "article", "index.md"), "---\nsource_id: article_DRAFT\n---\n")
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "article", "index.md"), "---\nsource_id: article_PUBLIC\n---\n")

	if _, _, published, err := FindPublishedBundleBySourceID(root, "article_DRAFT"); err != nil || published {
		t.Fatalf("继承 cascade.draft 的 Bundle 不应视为已发布: published=%v err=%v", published, err)
	}
	if _, _, published, err := FindPublishedBundleBySourceID(root, "article_PUBLIC"); err != nil || !published {
		t.Fatalf("普通 Bundle 应视为已发布: published=%v err=%v", published, err)
	}
}

func TestDiscoverSectionsReturnsPageBundleCategoryDirectories(t *testing.T) {
	root := t.TempDir()
	writeSectionFixture(t, filepath.Join(root, "hugo.toml"), "baseURL='https://example.com'\n")
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "ai", "article-one", "index.md"), "---\ntitle: One\n---\n")
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "tools", "article-two", "index.md"), "---\ntitle: Two\n---\n")
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "flat-article", "index.md"), "---\ntitle: Flat\n---\n")
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}

	discovery, err := provider.DiscoverSections(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverSections() error = %v", err)
	}
	if len(discovery.Sections) != 1 || len(discovery.Sections[0].Directories) != 2 {
		t.Fatalf("未发现 Page Bundle 分类目录: %+v", discovery)
	}
	if discovery.Sections[0].Directories[0].Path != "ai" || discovery.Sections[0].Directories[0].ArticleCount != 1 || discovery.Sections[0].Directories[1].Path != "tools" {
		t.Fatalf("分类目录信息错误: %+v", discovery.Sections[0].Directories)
	}
}

func TestDiscoverSectionsLocksExistingBundleCategoryDirectory(t *testing.T) {
	root := t.TempDir()
	writeSectionFixture(t, filepath.Join(root, "hugo.toml"), "baseURL='https://example.com'\n")
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "ai", "existing", "index.md"), "---\nsource_id: article_ONE\n---\n")
	provider, err := New(Config{Root: root, StagingRoot: filepath.Join(t.TempDir(), "staging")}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}

	discovery, err := provider.DiscoverSections(context.Background(), "article_ONE")
	if err != nil {
		t.Fatal(err)
	}
	if !discovery.SelectionLocked || discovery.ExistingSection != "posts" || discovery.ExistingDirectory != "ai" {
		t.Fatalf("已有 Page Bundle 未锁定原分类目录: %+v", discovery)
	}
}

func writeSectionFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
