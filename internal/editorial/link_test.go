package editorial

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestProcessWikiLinksConvertsBasicWikiLink(t *testing.T) {
	t.Parallel()
	body := "参见 [[target|显示文本]] 的内容。"
	result := ProcessWikiLinks(body, func(target, alias string) string {
		if target != "target" || alias != "显示文本" {
			t.Fatalf("解析参数错误: target=%q alias=%q", target, alias)
		}
		return "[显示文本](https://example.com/target/)"
	})
	if result != "参见 [显示文本](https://example.com/target/) 的内容。" {
		t.Fatalf("转换结果不符合预期: %q", result)
	}
}

func TestProcessWikiLinksOnlyConvertsMarkdownText(t *testing.T) {
	t.Parallel()
	body := "正文 [[normal]]\n\n`[[inline-code]]`\n\n```md\n[[code-block]]\n```\n\n[已有 [[link-label]]](https://example.com)\n\n![[image.png]]"
	var targets []string
	result := ProcessWikiLinks(body, func(target, alias string) string {
		targets = append(targets, target)
		return "<" + DefaultLabel(target, alias) + ">"
	})
	if len(targets) != 1 || targets[0] != "normal" {
		t.Fatalf("只应处理普通正文 WikiLink，实际目标: %+v", targets)
	}
	for _, untouched := range []string{"`[[inline-code]]`", "[[code-block]]", "[[link-label]]", "![[image.png]]"} {
		if !strings.Contains(result, untouched) {
			t.Fatalf("受保护语法不应变化 %q: %s", untouched, result)
		}
	}
}

func TestProcessWikiLinksHandlesMultipleLinksAndAliases(t *testing.T) {
	t.Parallel()
	body := "[[a]] 和 [[b|别名]] 以及 [[c]]"
	result := ProcessWikiLinks(body, func(target, alias string) string {
		return "<" + DefaultLabel(target, alias) + ">"
	})
	if result != "<a> 和 <别名> 以及 <c>" {
		t.Fatalf("多链接处理结果不符合预期: %q", result)
	}
}

func TestProcessWikiLinksConvertsWikiLinksInsideListItems(t *testing.T) {
	t.Parallel()
	body := "- [[first|第一篇]]；\n- [[second]]；"
	result := ProcessWikiLinks(body, func(target, alias string) string {
		return "<" + DefaultLabel(target, alias) + ">"
	})
	if result != "- <第一篇>；\n- <second>；" {
		t.Fatalf("列表项中的 WikiLink 应被转换: %q", result)
	}
}

func TestProcessWikiLinksReturnsBodyWhenNoMatches(t *testing.T) {
	t.Parallel()
	body := "普通文本没有 WikiLink"
	result := ProcessWikiLinks(body, func(target, alias string) string {
		t.Fatal("不应调用 resolve")
		return ""
	})
	if result != body {
		t.Fatalf("无 WikiLink 时应原样返回: %q", result)
	}
}

func TestArticleLinkResolverFindsByStemAndNestedPath(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedArticle(t, db, "w1", "s1", "article_one", "ai-note", "notes/ai-note.md", "AI Note")
	resolver := NewArticleLinkResolver(db, "w1")
	for _, target := range []string{"ai-note", "notes/ai-note", "ai-note#章节"} {
		resolution, err := resolver.Resolve(context.Background(), target)
		if err != nil || !resolution.Found || resolution.StableID != "article_one" {
			t.Fatalf("应解析目标 %q: resolution=%+v err=%v", target, resolution, err)
		}
	}
}

func TestArticleLinkResolverReportsMissingAmbiguousAndWorkspaceScope(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedWorkspace(t, db, "w2", "s2")
	seedArticle(t, db, "w1", "s1", "article_one", "one", "one/note.md", "One")
	seedArticle(t, db, "w1", "s1", "article_two", "two", "two/note.md", "Two")
	seedArticle(t, db, "w2", "s2", "article_three", "three", "note.md", "Three")
	if _, err := NewArticleLinkResolver(db, "w1").Resolve(context.Background(), "note"); err == nil {
		t.Fatal("同工作区同名目标必须报告歧义")
	} else if _, ok := err.(*AmbiguousLinkError); !ok {
		t.Fatalf("歧义错误类型错误: %T", err)
	}
	resolution, err := NewArticleLinkResolver(db, "w2").Resolve(context.Background(), "note")
	if err != nil || !resolution.Found || resolution.StableID != "article_three" {
		t.Fatalf("应限定当前工作区: resolution=%+v err=%v", resolution, err)
	}
	missing, err := NewArticleLinkResolver(db, "w2").Resolve(context.Background(), "missing")
	if err != nil || missing.Found {
		t.Fatalf("不存在目标应返回 Found=false: %+v err=%v", missing, err)
	}
}

func TestProcessWebWikiLinksUsesPublishedSlug(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	root := t.TempDir()
	seedHugoProvider(t, db, "w1", root, "https://blog.hankmo.com")
	seedArticle(t, db, "w1", "s1", "article_one", "my-slug", "notes/target.md", "Target")
	seedHugoBundle(t, root, "posts/original-directory", "article_one", "", "my-slug")
	result := ProcessWebWikiLinks(context.Background(), NewArticleLinkResolver(db, "w1"), "[[target|相关内容]]", db, "w1")
	expected := "[相关内容](https://blog.hankmo.com/my-slug/)"
	if result.Body != expected || len(result.Links) != 1 || result.Links[0].Status != LinkStatusConverted {
		t.Fatalf("Web 链接应使用已发布 slug: 期望 %q, 实际 %+v", expected, result)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "internal_link.converted" {
		t.Fatalf("转换成功诊断错误: %+v", result.Diagnostics)
	}
}

func TestProcessWebWikiLinksUsesExplicitPublishedURL(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	root := t.TempDir()
	seedHugoProvider(t, db, "w1", root, "https://blog.hankmo.com/base")
	seedArticle(t, db, "w1", "s1", "article_one", "", "target.md", "Target")
	seedHugoBundle(t, root, "posts/target", "article_one", "/fixed/path", "")
	result := ProcessWebWikiLinks(context.Background(), NewArticleLinkResolver(db, "w1"), "[[target]]", db, "w1")
	if result.Body != "[target](https://blog.hankmo.com/fixed/path/)" {
		t.Fatalf("显式 Hugo URL 应优先: %s", result.Body)
	}
}

func TestProcessWebWikiLinksFallsBackToPublishedBundlePath(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	root := t.TempDir()
	seedHugoProvider(t, db, "w1", root, "https://blog.hankmo.com")
	seedArticle(t, db, "w1", "s1", "article_one", "", "target-note.md", "Target")
	seedHugoBundle(t, root, "posts/target-note", "article_one", "", "")
	result := ProcessWebWikiLinks(context.Background(), NewArticleLinkResolver(db, "w1"), "[[target-note]]", db, "w1")
	expected := "[target-note](https://blog.hankmo.com/posts/target-note/)"
	if result.Body != expected {
		t.Fatalf("应使用已发布 Bundle 路径: 期望 %q, 实际 %q", expected, result.Body)
	}
}

func TestProcessWebWikiLinksReturnsPlainTextWithoutBlogConfig(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedArticle(t, db, "w1", "s1", "article_one", "slug", "target.md", "Target")
	result := ProcessWebWikiLinks(context.Background(), NewArticleLinkResolver(db, "w1"), "[[target|显示文本]]", db, "w1")
	if result.Body != "显示文本" || result.Links[0].Status != LinkStatusUnavailable {
		t.Fatalf("未配置博客时应清理 WikiLink 并保留纯文本: %+v", result)
	}
}

func TestProcessWebWikiLinksReportsUnpublishedAndMissingTargets(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	root := t.TempDir()
	seedHugoProvider(t, db, "w1", root, "https://blog.hankmo.com")
	seedArticle(t, db, "w1", "s1", "article_one", "draft", "draft.md", "Draft")
	result := ProcessWebWikiLinks(context.Background(), NewArticleLinkResolver(db, "w1"), "[[draft|尚未发布]] 和 [[missing|不存在]]", db, "w1")
	if result.Body != "尚未发布 和 不存在" || len(result.Links) != 2 || result.Links[0].Status != LinkStatusUnpublished || result.Links[1].Status != LinkStatusMissing {
		t.Fatalf("未发布和不存在目标应降级并区分诊断: %+v", result)
	}
	if len(result.Diagnostics) != 2 || result.Diagnostics[0].Code != "internal_link.unpublished" || result.Diagnostics[1].Code != "internal_link.missing" {
		t.Fatalf("内部链接诊断错误: %+v", result.Diagnostics)
	}
}

func TestProcessWebWikiLinksBlocksAmbiguousTarget(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	result := ProcessWebWikiLinks(context.Background(), errorLinkResolver{err: &AmbiguousLinkError{Target: "同名文章"}}, "[[同名文章]]", db, "w1")
	if result.Body != "同名文章" || !result.Links[0].Blocking || !result.Diagnostics[0].Blocking || result.Diagnostics[0].Code != "internal_link.ambiguous" {
		t.Fatalf("歧义目标必须阻断发布: %+v", result)
	}
}

func TestProcessHugoWikiLinksBuildsRelrefForPublishedTarget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	seedHugoBundle(t, root, "posts/target", "article_123", "", "")
	resolver := &mockLinkResolver{resolutions: map[string]LinkResolution{"目标文章": {Found: true, StableID: "article_123"}}}
	config := `{"root":` + mustJSONString(t, root) + `}`
	result := ProcessHugoWikiLinks(context.Background(), resolver, "[[目标文章|相关内容]]", config)
	expected := `[相关内容]({{< relref "posts/target/index.md" >}})`
	if result.Body != expected || result.Links[0].Status != LinkStatusConverted {
		t.Fatalf("Hugo 应生成 relref: 期望 %q, 实际 %+v", expected, result)
	}
}

func TestProcessHugoWikiLinksFallsBackToPlainTextWhenConfigInvalid(t *testing.T) {
	t.Parallel()
	resolver := &mockLinkResolver{resolutions: map[string]LinkResolution{"目标文章": {Found: true, StableID: "article_123"}}}
	body := "请查看 [[目标文章|相关内容]]。"
	for _, config := range []string{"", `{"base_url":"https://example.com"}`} {
		result := ProcessHugoWikiLinks(context.Background(), resolver, body, config)
		if result.Body != "请查看 相关内容。" || result.Links[0].Status != LinkStatusUnavailable {
			t.Fatalf("无效配置应退化为纯文本: %+v", result)
		}
	}
}

type mockLinkResolver struct{ resolutions map[string]LinkResolution }

func (m *mockLinkResolver) Resolve(_ context.Context, target string) (LinkResolution, error) {
	if resolution, ok := m.resolutions[target]; ok {
		return resolution, nil
	}
	return LinkResolution{Found: false}, nil
}

type errorLinkResolver struct{ err error }

func (r errorLinkResolver) Resolve(context.Context, string) (LinkResolution, error) {
	return LinkResolution{}, r.err
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatalf("创建测试数据库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedWorkspace(t, db, "w1", "s1")
	return db
}

func seedWorkspace(t *testing.T, db *sql.DB, workspaceID, sourceID string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES(?,?,'/tmp/test',?,?,?)`, workspaceID, workspaceID, now, now, now); err != nil {
		t.Fatalf("插入测试工作区: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES(?,?,'obsidian','/tmp/vault',?,?)`, sourceID, workspaceID, now, now); err != nil {
		t.Fatalf("插入测试 Source: %v", err)
	}
}

func seedArticle(t *testing.T, db *sql.DB, workspaceID, sourceID, stableID, slug, relativePath, title string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	_, err := db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,slug,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, stableID+"_id", workspaceID, sourceID, stableID, relativePath, title, slug, "hash1", "fh1", now, now, now, "ready")
	if err != nil {
		t.Fatalf("插入测试文章: %v", err)
	}
}

func seedHugoProvider(t *testing.T, db *sql.DB, workspaceID, root, baseURL string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	config := `{"root":` + mustJSONString(t, root) + `,"staging_root":` + mustJSONString(t, filepath.Join(root, "staging")) + `,"base_url":` + mustJSONString(t, baseURL) + `}`
	_, err := db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at)
VALUES(?,?,'hugo','Hugo',1,?,?,?)`, "hugo_"+workspaceID, workspaceID, config, now, now)
	if err != nil {
		t.Fatalf("插入测试 Hugo Provider: %v", err)
	}
}

func seedHugoBundle(t *testing.T, root, relative, sourceID, fixedURL, slug string) {
	t.Helper()
	directory := filepath.Join(root, "content", filepath.FromSlash(relative))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nsource_id: " + sourceID + "\n"
	if fixedURL != "" {
		content += "url: " + fixedURL + "\n"
	}
	if slug != "" {
		content += "slug: " + slug + "\n"
	}
	content += "---\n\n正文\n"
	if err := os.WriteFile(filepath.Join(directory, "index.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSONString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
