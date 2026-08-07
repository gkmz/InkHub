package httptransport

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestRequestedXiaohongshuModeDefaultsAndValidates(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("GET", "/api/v1/articles/a1/xiaohongshu", nil)
	mode, err := requestedXiaohongshuMode(request)
	if err != nil || mode != "long_card" {
		t.Fatalf("默认模式 = %q, err=%v", mode, err)
	}
	request = httptest.NewRequest("GET", "/api/v1/articles/a1/xiaohongshu?mode=visual_script", nil)
	mode, err = requestedXiaohongshuMode(request)
	if err != nil || mode != "visual_script" {
		t.Fatalf("分镜模式 = %q, err=%v", mode, err)
	}
	request = httptest.NewRequest("GET", "/api/v1/articles/a1/xiaohongshu?mode=unknown", nil)
	if _, err := requestedXiaohongshuMode(request); err == nil {
		t.Fatal("未知模式必须被拒绝")
	}
}

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

func TestParseXiaohongshuAIOutput(t *testing.T) {
	generated, err := parseXiaohongshuAIOutput([]contracts.Suggestion{
		{Field: "title", Value: json.RawMessage(`"短标题"`)},
		{Field: "body_html", Value: json.RawMessage(`"<p>短正文</p>"`)},
		{Field: "topics", Value: json.RawMessage(`"#AI编程 #效率工具"`)},
		{Field: "source_note", Value: json.RawMessage(`"来源"`)},
		{Field: "comment_copy", Value: json.RawMessage(`"欢迎讨论"`)},
	})
	if err != nil || generated.Title != "短标题" || generated.BodyHTML != "<p>短正文</p>" || len(generated.Topics) != 2 || formatXiaohongshuTopics(generated.Topics) != "#AI编程 #效率工具" {
		t.Fatalf("小红书 AI 字段映射错误: %+v err=%v", generated, err)
	}
}

func TestParseXiaohongshuStoryboardOutput(t *testing.T) {
	pages := make([]map[string]string, 0, 5)
	for index := 0; index < 5; index++ {
		pages = append(pages, map[string]string{"title": "页面", "prompt": "完整提示词"})
	}
	generated, err := parseXiaohongshuStoryboardOutput([]contracts.Suggestion{
		{Field: "title", Value: json.RawMessage(`"短标题"`)},
		{Field: "body", Value: json.RawMessage(`"补充正文"`)},
		{Field: "topics", Value: json.RawMessage(`"#AI #知识库"`)},
		{Field: "script_pages", Value: mustMarshalTestJSON(t, pages)},
	})
	if err != nil || len(generated.ScriptPages) != 5 || generated.ScriptPages[0].ID == "" || len(generated.Topics) != 2 {
		t.Fatalf("分镜 AI 字段映射错误: %+v err=%v", generated, err)
	}
}

func mustMarshalTestJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
