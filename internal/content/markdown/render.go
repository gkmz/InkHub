package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
)

// NewRenderer 创建 InkHub 统一 Markdown 渲染器。
//
// GFM 覆盖表格、任务列表和删除线等 Obsidian 常用语法，Chroma 为带语言
// 标记的代码块生成行内样式，供通用预览与渠道模板复用。
func NewRenderer() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
			),
		),
	)
}

// Render 将 Markdown 转换为带 GFM 和代码高亮的 HTML。
func Render(source []byte) (string, error) {
	var output bytes.Buffer
	if err := NewRenderer().Convert(source, &output); err != nil {
		return "", err
	}
	return output.String(), nil
}
