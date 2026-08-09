package editorial

import (
	"strings"
	"testing"
)

func TestExcludePublishSectionsRemovesMatchingHeadingAndChildren(t *testing.T) {
	body := "引言\n\n## 相关链接\n- [[第一篇]]\n\n### 延伸阅读\n![图](hidden.png)\n\n## 总结\n保留内容\n"
	result := ExcludePublishSections(body, []string{" 相关链接 "})
	if strings.Contains(result.Body, "相关链接") || strings.Contains(result.Body, "hidden.png") || strings.Contains(result.Body, "## \n") || !strings.Contains(result.Body, "## 总结") {
		t.Fatalf("章节裁剪结果错误: %q", result.Body)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Title != "相关链接" || result.Excluded[0].Occurrences != 1 || result.Excluded[0].BlockCount != 3 {
		t.Fatalf("章节诊断错误: %+v", result.Excluded)
	}
}

func TestExcludePublishSectionsMatchesEveryOccurrenceRegardlessOfLevel(t *testing.T) {
	body := "# 正文\n保留\n\n### 相关链接\n一\n\n### 下一节\n保留二\n\n## 相关链接\n二\n"
	result := ExcludePublishSections(body, []string{"相关链接", "相关链接", ""})
	if strings.Contains(result.Body, "\n一\n") || strings.Contains(result.Body, "\n二\n") || !strings.Contains(result.Body, "### 下一节") {
		t.Fatalf("同名章节裁剪结果错误: %q", result.Body)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Occurrences != 2 {
		t.Fatalf("同名章节没有汇总: %+v", result.Excluded)
	}
}

func TestExcludePublishSectionsDoesNotTreatFencedCodeAsHeading(t *testing.T) {
	body := "```md\n## 相关链接\n代码内容\n```\n\n## 正文\n保留\n"
	result := ExcludePublishSections(body, []string{"相关链接"})
	if result.Body != body || len(result.Excluded) != 0 {
		t.Fatalf("代码块中的标题不应被裁剪: %+v", result)
	}
}

func TestExcludePublishSectionsParentRangeDoesNotDoubleCountNestedMatch(t *testing.T) {
	body := "## 相关链接\n父内容\n\n### 相关链接\n子内容\n\n## 正文\n保留\n"
	result := ExcludePublishSections(body, []string{"相关链接"})
	if len(result.Excluded) != 1 || result.Excluded[0].Occurrences != 1 || strings.Contains(result.Body, "子内容") {
		t.Fatalf("嵌套匹配不应重复计数: %+v body=%q", result.Excluded, result.Body)
	}
}

func TestExcludePublishSectionsRemovesAdjacentHorizontalRule(t *testing.T) {
	body := "正文内容\n\n---\n\n## 相关链接\n- [[第一篇]]\n"
	result := ExcludePublishSections(body, []string{"相关链接"})
	if strings.Contains(result.Body, "---") || strings.Contains(result.Body, "相关链接") || strings.TrimSpace(result.Body) != "正文内容" {
		t.Fatalf("章节前水平线没有随章节删除: %q", result.Body)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].BlockCount != 2 {
		t.Fatalf("水平线应计入被排除内容块: %+v", result.Excluded)
	}
}
