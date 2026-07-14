package web

import "testing"

func TestAssetsEmbedProductionEntry(t *testing.T) {
	content, err := Assets.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("读取嵌入 UI: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("嵌入的 React 入口不能为空")
	}
}
