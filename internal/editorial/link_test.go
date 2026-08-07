package editorial

import (
	"context"
	"database/sql"
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

func TestProcessWikiLinksUsesTargetAsLabelWhenAliasMissing(t *testing.T) {
	t.Parallel()
	body := "[[note]]"
	result := ProcessWikiLinks(body, func(target, alias string) string {
		return DefaultLabel(target, alias)
	})
	if result != "note" {
		t.Fatalf("无 alias 时应使用 target 作为 label: %q", result)
	}
}

func TestProcessWikiLinksSkipsImageEmbeds(t *testing.T) {
	t.Parallel()
	body := "正文 [[note]] 图片 ![[image.png]] 结尾"
	calls := 0
	result := ProcessWikiLinks(body, func(target, alias string) string {
		calls++
		if target == "image.png" {
			t.Fatal("不应处理图片嵌入")
		}
		return DefaultLabel(target, alias)
	})
	if calls != 1 {
		t.Fatalf("应只处理 1 个 wiki 链接，实际处理 %d 个", calls)
	}
	if !strings.Contains(result, "![[image.png]]") {
		t.Fatalf("图片嵌入应保留原样: %q", result)
	}
}

func TestProcessWikiLinksHandlesMultipleLinks(t *testing.T) {
	t.Parallel()
	body := "[[a]] 和 [[b|别名]] 以及 [[c]]"
	result := ProcessWikiLinks(body, func(target, alias string) string {
		return "<" + DefaultLabel(target, alias) + ">"
	})
	if result != "<a> 和 <别名> 以及 <c>" {
		t.Fatalf("多链接处理结果不符合预期: %q", result)
	}
}

func TestProcessWikiLinksReturnsBodyWhenNoMatches(t *testing.T) {
	t.Parallel()
	body := "普通文本没有 wiki 链接"
	result := ProcessWikiLinks(body, func(target, alias string) string {
		t.Fatal("不应调用 resolve")
		return ""
	})
	if result != body {
		t.Fatalf("无 wiki 链接时应原样返回: %q", result)
	}
}

func TestArticleLinkResolverFindsByStem(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedArticle(t, db, "w1", "s1", "article_one", "ai-note", "notes/ai-note.md", "ai-note")
	resolver := NewArticleLinkResolver(db, "w1")
	resolution, err := resolver.Resolve(context.Background(), "ai-note")
	if err != nil {
		t.Fatalf("Resolve 错误: %v", err)
	}
	if !resolution.Found {
		t.Fatal("应找到目标文章")
	}
	if resolution.StableID != "article_one" || resolution.Slug != "ai-note" {
		t.Fatalf("解析结果不符合预期: %+v", resolution)
	}
}

func TestArticleLinkResolverFindsByNestedPath(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedArticle(t, db, "w1", "s1", "article_two", "nested-note", "folder/sub/nested-note.md", "nested-note")
	resolver := NewArticleLinkResolver(db, "w1")
	resolution, err := resolver.Resolve(context.Background(), "nested-note")
	if err != nil {
		t.Fatalf("Resolve 错误: %v", err)
	}
	if !resolution.Found || resolution.StableID != "article_two" {
		t.Fatalf("应通过嵌套路径 stem 找到文章: %+v", resolution)
	}
}

func TestArticleLinkResolverReturnsNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedArticle(t, db, "w1", "s1", "article_one", "ai-note", "notes/ai-note.md", "ai-note")
	resolver := NewArticleLinkResolver(db, "w1")
	resolution, err := resolver.Resolve(context.Background(), "不存在的笔记")
	if err != nil {
		t.Fatalf("Resolve 错误: %v", err)
	}
	if resolution.Found {
		t.Fatal("未发布目标应返回 Found=false")
	}
	if resolution.Label != "不存在的笔记" {
		t.Fatalf("Label 应保留原始 target: %q", resolution.Label)
	}
}

func TestArticleLinkResolverScopedToWorkspace(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedWorkspace(t, db, "w2", "s2")
	seedArticle(t, db, "w1", "s1", "article_one", "note", "note.md", "note")
	seedArticle(t, db, "w2", "s2", "article_two", "note", "note.md", "note")
	resolver := NewArticleLinkResolver(db, "w2")
	resolution, err := resolver.Resolve(context.Background(), "note")
	if err != nil {
		t.Fatalf("Resolve 错误: %v", err)
	}
	if !resolution.Found || resolution.StableID != "article_two" {
		t.Fatalf("应只查询当前工作区文章: %+v", resolution)
	}
}

func TestProcessWebWikiLinksUsesSlug(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedHugoProvider(t, db, "w1", "https://blog.hankmo.com")
	seedArticle(t, db, "w1", "s1", "article_one", "my-slug", "notes/target.md", "my-slug")
	resolver := NewArticleLinkResolver(db, "w1")
	body := "[[target|相关内容]]"
	result := ProcessWebWikiLinks(context.Background(), resolver, body, db, "w1")
	expected := "[相关内容](https://blog.hankmo.com/my-slug/)"
	if result != expected {
		t.Fatalf("Web 链接应使用 slug: 期望 %q, 实际 %q", expected, result)
	}
}

func TestProcessWebWikiLinksFallsBackToStemWhenSlugEmpty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedHugoProvider(t, db, "w1", "https://blog.hankmo.com")
	seedArticle(t, db, "w1", "s1", "article_one", "", "notes/target-note.md", "")
	resolver := NewArticleLinkResolver(db, "w1")
	body := "[[target-note]]"
	result := ProcessWebWikiLinks(context.Background(), resolver, body, db, "w1")
	expected := "[target-note](https://blog.hankmo.com/target-note/)"
	if result != expected {
		t.Fatalf("slug 为空时应使用 stem: 期望 %q, 实际 %q", expected, result)
	}
}

func TestProcessWebWikiLinksReturnsBodyWhenBaseURLEmpty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedArticle(t, db, "w1", "s1", "article_one", "slug", "target.md", "slug")
	resolver := NewArticleLinkResolver(db, "w1")
	body := "[[target]]"
	result := ProcessWebWikiLinks(context.Background(), resolver, body, db, "w1")
	if result != body {
		t.Fatalf("未配置 baseURL 时应原样返回: %q", result)
	}
}

func TestProcessWebWikiLinksReturnsLabelForUnpublished(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedHugoProvider(t, db, "w1", "https://blog.hankmo.com")
	resolver := NewArticleLinkResolver(db, "w1")
	body := "[[未发布|显示文本]]"
	result := ProcessWebWikiLinks(context.Background(), resolver, body, db, "w1")
	if result != "显示文本" {
		t.Fatalf("未发布目标应保留纯文本 label: %q", result)
	}
}

func TestProcessHugoWikiLinksFallsBackToPlainTextWhenConfigInvalid(t *testing.T) {
	t.Parallel()
	resolver := &mockLinkResolver{
		resolutions: map[string]LinkResolution{
			"目标文章": {Found: true, StableID: "article_123"},
		},
	}
	body := "请查看 [[目标文章|相关内容]]。"

	// 空配置应该退化为纯文本
	result := ProcessHugoWikiLinks(context.Background(), resolver, body, "")
	if !strings.Contains(result, "相关内容") {
		t.Fatalf("空配置应退化为纯文本 label: %q", result)
	}
	if strings.Contains(result, "[[") || strings.Contains(result, "]]") {
		t.Fatalf("空配置应移除 wiki 语法: %q", result)
	}

	// 无效配置（无 root）也应退化为纯文本
	result = ProcessHugoWikiLinks(context.Background(), resolver, body, `{"base_url":"https://example.com"}`)
	if !strings.Contains(result, "相关内容") {
		t.Fatalf("无 root 配置应退化为纯文本 label: %q", result)
	}
}

type mockLinkResolver struct {
	resolutions map[string]LinkResolution
}

func (m *mockLinkResolver) Resolve(ctx context.Context, target string) (LinkResolution, error) {
	if r, ok := m.resolutions[target]; ok {
		return r, nil
	}
	return LinkResolution{Found: false}, nil
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
	_, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES(?,?,'/tmp/test',?,?,?)`, workspaceID, workspaceID, now, now, now)
	if err != nil {
		t.Fatalf("插入测试工作区: %v", err)
	}
	_, err = db.Exec(`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES(?,?,'obsidian','/tmp/vault',?,?)`, sourceID, workspaceID, now, now)
	if err != nil {
		t.Fatalf("插入测试 Source: %v", err)
	}
}

func seedArticle(t *testing.T, db *sql.DB, workspaceID, sourceID, stableID, slug, relativePath, title string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	_, err := db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,slug,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		stableID+"_id", workspaceID, sourceID, stableID, relativePath, title, slug, "hash1", "fh1", now, now, now, "ready")
	if err != nil {
		t.Fatalf("插入测试文章: %v", err)
	}
}

func seedHugoProvider(t *testing.T, db *sql.DB, workspaceID, baseURL string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	config := `{"root":"/tmp/hugo","staging_root":"/tmp/staging","base_url":"` + baseURL + `"}`
	_, err := db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at)
VALUES(?,?,'hugo','Hugo',1,?,?,?)`,
		"hugo_"+workspaceID, workspaceID, config, now, now)
	if err != nil {
		t.Fatalf("插入测试 Hugo Provider: %v", err)
	}
}
