package markdown

import (
	"strings"
	"testing"
)

func TestRenderSupportsGFMTableAndCodeHighlighting(t *testing.T) {
	t.Parallel()

	html, err := Render([]byte("| 名称 | 值 |\n| --- | --- |\n| InkHub | ready |\n\n```go\nfunc main() {}\n```"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<table>", "<th>名称</th>", `class="chroma"`, `class="kd"`, `class="nf"`} {
		if !strings.Contains(html, fragment) {
			t.Fatalf("Markdown HTML 缺少 %q: %s", fragment, html)
		}
	}
}

func TestRenderKeepsMermaidFenceAsMarkedCode(t *testing.T) {
	t.Parallel()

	html, err := Render([]byte("```mermaid\ngraph TD; A-->B;\n```"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "language-mermaid") || !strings.Contains(html, "A--&gt;B") {
		t.Fatalf("Mermaid 代码块未保留语言标记: %s", html)
	}
}
