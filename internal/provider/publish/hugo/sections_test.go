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
	target, section, found, err := findBundleBySourceID(root, "article_ONE")
	if err != nil || !found || section != "notes" || target != filepath.Join(root, "content", "notes", "existing") {
		t.Fatalf("findBundleBySourceID() = %q, %q, %v, %v", target, section, found, err)
	}
	writeSectionFixture(t, filepath.Join(root, "content", "posts", "duplicate", "index.md"), "---\nsource_id: article_ONE\n---\n")
	if _, _, _, err := findBundleBySourceID(root, "article_ONE"); err == nil {
		t.Fatal("重复 source_id 应阻止目标选择")
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
