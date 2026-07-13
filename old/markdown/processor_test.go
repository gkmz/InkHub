package markdown

import (
	"testing"

	"github.com/gkmz/InkHub/old/models"
)

// TestFindArticle 验证 relref/ref 路径能按相对路径、文件名和扩展名差异匹配文章。
func TestFindArticle(t *testing.T) {
	processor := &Processor{
		articles: []models.Article{
			{ID: "go_20240618", RelPath: "go/20240618-Data Race vs Race Condition.md"},
			{ID: "java_20220328", RelPath: "java/20220328-读《Java并发》- Java内存模型.adoc"},
			{ID: "other_simple", RelPath: "other/simple.md"},
		},
	}

	tests := []struct {
		name     string
		refPath  string
		expected string
	}{
		{name: "file name", refPath: "20240618-Data Race vs Race Condition.md", expected: "go_20240618"},
		{name: "unicode file name", refPath: "20220328-读《Java并发》- Java内存模型.adoc", expected: "java_20220328"},
		{name: "relative path", refPath: "go/20240618-Data Race vs Race Condition.md", expected: "go_20240618"},
		{name: "different extension", refPath: "20220328-读《Java并发》- Java内存模型.md", expected: "java_20220328"},
		{name: "simple file", refPath: "simple.md", expected: "other_simple"},
		{name: "not found", refPath: "not-exist.md", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			article := processor.findArticle(tt.refPath)
			if tt.expected == "" {
				if article != nil {
					t.Fatalf("expected no article, got %s", article.ID)
				}
				return
			}
			if article == nil {
				t.Fatalf("expected article %s, got nil", tt.expected)
			}
			if article.ID != tt.expected {
				t.Fatalf("expected article %s, got %s", tt.expected, article.ID)
			}
		})
	}
}
