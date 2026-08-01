package wechat

import (
	"strings"
	"testing"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
)

func TestRenderUsesSameSafePipelineForDifferentTemplates(t *testing.T) {
	t.Parallel()

	body := `# 标题

正文包含 **重点** 和 [链接](https://example.com)。

> 引用

<script>alert(1)</script>`
	defaultTemplate, err := domaintemplate.Builtin(domaintemplate.BuiltinDefaultID)
	if err != nil {
		t.Fatal(err)
	}
	minimalTemplate, err := domaintemplate.Builtin(domaintemplate.BuiltinMinimalID)
	if err != nil {
		t.Fatal(err)
	}

	defaultHTML, err := Render(defaultTemplate, body, nil)
	if err != nil {
		t.Fatalf("渲染 Default: %v", err)
	}
	minimalHTML, err := Render(minimalTemplate, body, nil)
	if err != nil {
		t.Fatalf("渲染 Minimal: %v", err)
	}
	if defaultHTML == minimalHTML {
		t.Fatal("不同模板不应产生相同 HTML")
	}
	for _, output := range []string{defaultHTML, minimalHTML} {
		lower := strings.ToLower(output)
		for _, forbidden := range []string{"<style", "<script", "class=", "data-inkhub", "javascript:", "{{"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("最终 HTML 包含禁止内容 %q:\n%s", forbidden, output)
			}
		}
		if !strings.Contains(output, `style="`) || !strings.Contains(output, "标题") {
			t.Fatalf("样式未内联或正文丢失:\n%s", output)
		}
	}
	again, err := Render(defaultTemplate, body, nil)
	if err != nil || again != defaultHTML {
		t.Fatal("相同输入渲染结果不确定")
	}
}

func TestRenderRejectsLocalAndScriptImageURLs(t *testing.T) {
	t.Parallel()

	validated := renderTemplate("safe", `.inkhub-root img { max-width: 100%; }`)
	for _, body := range []string{`![图](file:///tmp/a.png)`, `![图](javascript:alert(1))`, `![图](./local.png)`} {
		if _, err := Render(validated, body, nil); err == nil {
			t.Fatalf("不安全图片 URL 应被拒绝: %s", body)
		}
	}
}

func TestRenderResolvesTypedVariables(t *testing.T) {
	t.Parallel()

	validated := domaintemplate.Validated{
		Manifest: domaintemplate.Manifest{ID: "variables", Variables: map[string]domaintemplate.Variable{
			"accentColor": {Type: "color", Label: "强调色", Default: "#1677ff"},
		}},
		CSS: `.inkhub-root p { color: {{ color.accentColor }}; }`,
	}
	output, err := Render(validated, "正文", map[string]any{"accentColor": "#ff0000"})
	if err != nil {
		t.Fatalf("渲染变量: %v", err)
	}
	if !strings.Contains(output, "color:#ff0000") {
		t.Fatalf("变量未映射为内联样式: %s", output)
	}
}

func TestRenderSupportsTableAndHighlightedCode(t *testing.T) {
	t.Parallel()

	validated := renderTemplate("rich", `.inkhub-root table { border-collapse: collapse; } .inkhub-root code { color: #111111; }`)
	output, err := Render(validated, "| 字段 | 值 |\n| --- | --- |\n| 状态 | ready |\n\n```go\nfunc main() {}\n```", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<table", "<th", "<span", "font-weight:bold"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("微信 HTML 缺少 %q: %s", fragment, output)
		}
	}
}

func renderTemplate(id, css string) domaintemplate.Validated {
	return domaintemplate.Validated{Manifest: domaintemplate.Manifest{ID: id, Variables: map[string]domaintemplate.Variable{}}, CSS: css}
}
