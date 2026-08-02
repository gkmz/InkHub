package httptransport

import "testing"

func TestParseXiaohongshuPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path, article, suffix string
		ok                    bool
	}{
		{path: "/api/v1/articles/a1/xiaohongshu", article: "a1", ok: true},
		{path: "/api/v1/articles/a1/xiaohongshu/drafts/generate", article: "a1", suffix: "drafts/generate", ok: true},
		{path: "/api/v1/articles/a1/wechat", ok: false},
	}
	for _, test := range tests {
		article, suffix, ok := parseXiaohongshuPath(test.path)
		if article != test.article || suffix != test.suffix || ok != test.ok {
			t.Fatalf("parseXiaohongshuPath(%q) = %q, %q, %v", test.path, article, suffix, ok)
		}
	}
}

func TestRuntimeXiaohongshuLabel(t *testing.T) {
	t.Parallel()
	if got := runtimeXiaohongshuLabel("published", "hash-1", "hash-1"); got != "已发布" {
		t.Fatalf("published label = %q", got)
	}
	if got := runtimeXiaohongshuLabel("draft", "hash-1", "hash-2"); got != "内容已更新" {
		t.Fatalf("stale label = %q", got)
	}
}
