package hugo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestDiscoverReadsHugoConfigFrontmatterAndTermPage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "hugo.toml"), "[taxonomies]\ntopic = 'topics'\ntag = 'tags'\n")
	writeFixture(t, filepath.Join(root, "content", "posts", "one.md"), "---\ntitle: One\ntopics: [Go]\ntags: [InkHub, Go]\n---\n正文")
	writeFixture(t, filepath.Join(root, "content", "topics", "go", "_index.md"), "---\ntitle: Go 语言\ndescription: Go 相关文章\naliases: [/golang/]\n---\n")
	provider, err := New(Config{Root: root, ContentDir: "content"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := provider.Discover(context.Background(), contracts.TaxonomyCursor{})
	if err != nil {
		t.Fatalf("发现 Hugo taxonomy: %v", err)
	}
	assertTerm(t, snapshot.Terms, "topic", "go", "Go 语言", 1)
	assertTerm(t, snapshot.Terms, "tag", "inkhub", "InkHub", 1)
	if snapshot.Revision == "" || !snapshot.Complete {
		t.Fatalf("快照不完整: %+v", snapshot)
	}
	unchanged, err := provider.Discover(context.Background(), contracts.TaxonomyCursor{Revision: snapshot.Revision})
	if err != nil || !unchanged.Unchanged || len(unchanged.Terms) != 0 {
		t.Fatalf("相同 revision 应返回未变化: %+v err=%v", unchanged, err)
	}
}

func TestDiscoverUsesHugoDefaultTaxonomies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "hugo.yaml"), "title: Test\n")
	writeFixture(t, filepath.Join(root, "content", "post.md"), "---\ncategories: [Engineering]\ntags: [Go]\n---\n")
	provider, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := provider.Discover(context.Background(), contracts.TaxonomyCursor{})
	if err != nil {
		t.Fatal(err)
	}
	assertTerm(t, snapshot.Terms, "category", "engineering", "Engineering", 1)
	assertTerm(t, snapshot.Terms, "tag", "go", "Go", 1)
}

func TestPlanAndApplyCreateStandardHugoTermPage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "hugo.yaml"), "title: Test\n")
	provider, err := New(Config{ProviderID: "hugo-1", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	before, err := provider.Discover(context.Background(), contracts.TaxonomyCursor{})
	if err != nil {
		t.Fatal(err)
	}
	change, err := provider.PlanChange(context.Background(), contracts.TaxonomyCommand{
		Kind: contracts.TaxonomyCreateTerm, ExpectedRevision: before.Revision,
		Term: contracts.TaxonomyTerm{Kind: "category", Key: "engineering", Name: "Engineering", Metadata: map[string]string{"description": "工程文章"}},
	})
	if err != nil || len(change.Files) != 1 || change.Files[0].RelativePath != "content/categories/engineering/_index.md" {
		t.Fatalf("规划 term page: change=%+v err=%v", change, err)
	}
	after, err := provider.ApplyChange(context.Background(), change)
	if err != nil {
		t.Fatalf("应用 term page: %v", err)
	}
	assertTerm(t, after.Terms, "category", "engineering", "Engineering", 0)
	content, err := os.ReadFile(filepath.Join(root, "content", "categories", "engineering", "_index.md"))
	if err != nil || !strings.Contains(string(content), "description: 工程文章") {
		t.Fatalf("标准 term page 未写入: %s err=%v", content, err)
	}
}

func TestApplyRejectsStaleTaxonomyRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "hugo.yaml"), "title: Test\n")
	provider, _ := New(Config{ProviderID: "hugo-1", Root: root})
	before, _ := provider.Discover(context.Background(), contracts.TaxonomyCursor{})
	change, err := provider.PlanChange(context.Background(), contracts.TaxonomyCommand{Kind: contracts.TaxonomyCreateTerm, ExpectedRevision: before.Revision, Term: contracts.TaxonomyTerm{Kind: "tag", Key: "go", Name: "Go"}})
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, "content", "post.md"), "---\ntags: [external]\n---\n")
	if _, err := provider.ApplyChange(context.Background(), change); err == nil {
		t.Fatal("外部修改后应拒绝旧 taxonomy 变更")
	}
}

func TestDiscoverRejectsEscapedContentDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "hugo.yaml"), "contentDir: ../private\n")
	provider, _ := New(Config{Root: root})
	if _, err := provider.Discover(context.Background(), contracts.TaxonomyCursor{}); err == nil {
		t.Fatal("越界 contentDir 应被拒绝")
	}
}

func TestApplyDoesNotOverwritePreexistingTermDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "hugo.yaml"), "title: Test\n")
	provider, _ := New(Config{ProviderID: "hugo-1", Root: root})
	before, _ := provider.Discover(context.Background(), contracts.TaxonomyCursor{})
	change, err := provider.PlanChange(context.Background(), contracts.TaxonomyCommand{Kind: contracts.TaxonomyCreateTerm, ExpectedRevision: before.Revision, Term: contracts.TaxonomyTerm{Kind: "tag", Key: "go", Name: "Go"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "content", "tags", "go"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ApplyChange(context.Background(), change); err == nil {
		t.Fatal("已有 term 目录不应被覆盖")
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertTerm(t *testing.T, terms []contracts.TaxonomyTerm, kind, key, name string, usage int) {
	t.Helper()
	for _, term := range terms {
		if term.Kind == kind && term.Key == key {
			if term.Name != name || term.UsageCount != usage {
				t.Fatalf("term 不匹配: %+v", term)
			}
			return
		}
	}
	t.Fatalf("未找到 term: kind=%s key=%s terms=%+v", kind, key, terms)
}
