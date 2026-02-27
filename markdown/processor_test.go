package markdown

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Article struct {
	ID      string
	RelPath string
}

func findArticle(path string, articles []Article) *Article {
	path = filepath.ToSlash(path)
	pathBase := filepath.Base(path)

	for i := range articles {
		art := &articles[i]
		artRelPath := filepath.ToSlash(art.RelPath)
		artBase := filepath.Base(artRelPath)

		if artRelPath == path {
			return art
		}

		if artBase == pathBase {
			return art
		}

		pathExt := filepath.Ext(pathBase)
		artExt := filepath.Ext(artBase)
		if pathExt != "" && artExt != "" {
			pathNameNoExt := strings.TrimSuffix(pathBase, pathExt)
			artNameNoExt := strings.TrimSuffix(artBase, artExt)
			if pathNameNoExt == artNameNoExt {
				return art
			}
		}

		if strings.HasSuffix(artRelPath, path) {
			return art
		}
	}

	return nil
}

func TestFindAriticle() {
	articles := []Article{
		{ID: "go_20240618", RelPath: "go/20240618-Data Race vs Race Condition.md"},
		{ID: "java_20220328", RelPath: "java/20220328-读《Java并发》- Java内存模型.adoc"},
		{ID: "other_simple", RelPath: "other/simple.md"},
	}

	testCases := []struct {
		refPath  string
		expected string
	}{
		{"20240618-Data Race vs Race Condition.md", "go_20240618"},
		{"20220328-读《Java并发》- Java内存模型.adoc", "java_20220328"},
		{"go/20240618-Data Race vs Race Condition.md", "go_20240618"},
		{"20220328-读《Java并发》- Java内存模型.md", "java_20220328"},
		{"simple.md", "other_simple"},
		{"not-exist.md", ""},
	}

	fmt.Println("测试文章匹配逻辑：")
	for i, tc := range testCases {
		result := findArticle(tc.refPath, articles)
		if result != nil {
			if result.ID == tc.expected {
				fmt.Printf("✅ 测试 %d: %s -> 找到 %s\n", i+1, tc.refPath, result.ID)
			} else {
				fmt.Printf("❌ 测试 %d: %s -> 期望 %s, 实际 %s\n", i+1, tc.refPath, tc.expected, result.ID)
			}
		} else {
			if tc.expected == "" {
				fmt.Printf("✅ 测试 %d: %s -> 正确返回 nil\n", i+1, tc.refPath)
			} else {
				fmt.Printf("❌ 测试 %d: %s -> 期望 %s, 但返回 nil\n", i+1, tc.refPath, tc.expected)
			}
		}
	}
}