package markdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFindsMissingLocalImageAndHeadingJump(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	findings := Check([]byte("# 标题\n\n### 跳级\n\n![missing](assets/missing.png)\n"), base)
	assertFinding(t, findings, "markdown.heading_jump", SeverityRecommended)
	assertFinding(t, findings, "markdown.image_missing", SeverityBlocking)

	if err := os.MkdirAll(filepath.Join(base, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "assets", "missing.png"), []byte("image"), 0o640); err != nil {
		t.Fatal(err)
	}
	findings = Check([]byte("![exists](assets/missing.png)"), base)
	if hasFinding(findings, "markdown.image_missing") {
		t.Fatal("existing local image must not be reported missing")
	}
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

func hasFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
