package editorial

import (
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
)

func TestCheckArticleReturnsBlockingAndRecommendedFindings(t *testing.T) {
	t.Parallel()

	findings := CheckArticle(article.Article{Title: "标题", Slug: "Invalid Slug"}, "# 标题\n\n正文")
	assertFinding(t, findings, "metadata.description_missing", SeverityBlocking)
	assertFinding(t, findings, "metadata.slug_invalid", SeverityBlocking)
	assertFinding(t, findings, "metadata.keywords_missing", SeverityRecommended)
}

func assertFinding(t *testing.T, findings []Finding, code string, severity Severity) {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code && finding.Severity == severity {
			return
		}
	}
	t.Fatalf("missing finding %s/%s in %#v", code, severity, findings)
}
