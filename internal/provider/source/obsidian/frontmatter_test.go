package obsidian

import "testing"

func TestParseDocumentReadsURLAndDateFromFrontmatter(t *testing.T) {
	document, err := parseDocument([]byte("---\nurl: superpowers-workflow\ndate: 2026-07-30\npublish:\n  slug: old-slug\n---\n正文"))
	if err != nil {
		t.Fatalf("解析 frontmatter: %v", err)
	}
	if document.Article.URL != "superpowers-workflow" || document.Article.PublishDate != "2026-07-30" {
		t.Fatalf("未读取 URL/date: %+v", document.Article)
	}
}

func TestParseDocumentTreatsNullScalarsAsMissing(t *testing.T) {
	document, err := parseDocument([]byte("---\nid: null\ntitle:\ndescription: ~\n---\n正文"))
	if err != nil {
		t.Fatalf("解析可空 frontmatter: %v", err)
	}
	if document.Article.StableID != "" || document.Article.Title != "" || document.Article.Description != "" {
		t.Fatalf("YAML null 不应成为字符串值: %+v", document.Article)
	}
}
