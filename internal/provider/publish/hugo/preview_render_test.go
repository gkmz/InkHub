package hugo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindPreviewRenderPathUsesExistingCandidate(t *testing.T) {
	root := t.TempDir()
	writePreviewRenderFixture(t, filepath.Join(root, "posts", "demo", "index.html"), "<main>当前文章</main>")
	writePreviewRenderFixture(t, filepath.Join(root, "posts", "other", "index.html"), "<main>其他文章</main>")

	result, err := findPreviewRenderPath(root, []string{"custom/index.html", "posts/demo/index.html"})
	if err != nil || result != "posts/demo/index.html" {
		t.Fatalf("定位当前文章渲染结果: path=%q err=%v", result, err)
	}
	if _, err := findPreviewRenderPath(root, []string{"missing/index.html"}); err == nil {
		t.Fatal("候选页面不存在时必须返回错误")
	}
}

func writePreviewRenderFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
