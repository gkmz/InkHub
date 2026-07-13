// Package editorial 编排文章检查、建议和人工审核用例。
package editorial

import (
	"regexp"
	"strings"

	"github.com/gkmz/InkHub/internal/domain/article"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Severity 是检查发现项的严重程度。
type Severity string

const (
	SeverityBlocking    Severity = "blocking"
	SeverityRecommended Severity = "recommended"
	SeverityOptional    Severity = "optional"
	SeverityPassed      Severity = "passed"
)

// Finding 描述一项确定性内容检查结果。
type Finding struct {
	Code     string
	Severity Severity
	Message  string
}

// CheckArticle 执行不修改文章的通用元数据和 Markdown 检查。
func CheckArticle(value article.Article, body string) []Finding {
	var findings []Finding
	if strings.TrimSpace(value.Title) == "" {
		findings = append(findings, Finding{Code: "metadata.title_missing", Severity: SeverityBlocking, Message: "缺少标题"})
	}
	if strings.TrimSpace(value.Description) == "" {
		findings = append(findings, Finding{Code: "metadata.description_missing", Severity: SeverityBlocking, Message: "缺少摘要"})
	}
	if value.Slug == "" || !slugPattern.MatchString(value.Slug) {
		findings = append(findings, Finding{Code: "metadata.slug_invalid", Severity: SeverityBlocking, Message: "Slug 只能使用小写字母、数字和短横线"})
	}
	if len(value.Keywords) == 0 {
		findings = append(findings, Finding{Code: "metadata.keywords_missing", Severity: SeverityRecommended, Message: "建议补充 SEO Keywords"})
	}
	if strings.TrimSpace(body) == "" {
		findings = append(findings, Finding{Code: "markdown.body_empty", Severity: SeverityBlocking, Message: "正文为空"})
	}
	return findings
}
