package template

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	BuiltinDefaultID = "inkhub-default"
	BuiltinMinimalID = "inkhub-minimal"
)

// Builtin 返回经过同一 CSS 安全校验的内置模板。
func Builtin(id string) (Validated, error) {
	manifest := Manifest{
		SpecVersion: "1.1", Target: TargetWeChatHTML, Format: "css", Renderer: RendererWeChatHTMLV1,
		Compatibility: Compatibility{Providers: []string{"wechat"}, RendererVersion: "1"},
		ID:            id, Version: "1.0.0", Author: Author{Name: "InkHub"},
		License: "Apache-2.0", InkHubVersion: ">=1.0.0 <2.0.0", Variables: map[string]Variable{
			"accentColor": {Type: "color", Label: "强调色", Default: "#1677ff"},
			"bodyFont":    {Type: "font-family", Label: "正文字体", Default: "system-sans", Options: []string{"system-sans", "system-serif"}},
		},
	}
	var css string
	switch id {
	case BuiltinDefaultID:
		manifest.Name = "InkHub Default"
		manifest.Description = "InkHub 默认微信公众号模板"
		css = `.inkhub-root { color: #222222; font-family: {{ font-family.bodyFont }}; }
.inkhub-root h1 { color: {{ color.accentColor }}; font-size: 26px; text-align: center; margin-bottom: 24px; }
.inkhub-root h2 { color: {{ color.accentColor }}; font-size: 22px; border-left: 4px solid #1677ff; padding-left: 12px; }
.inkhub-root h3 { font-size: 19px; }
.inkhub-root p { font-size: 16px; line-height: 1.8; margin-bottom: 16px; }
.inkhub-root blockquote { background-color: #f6f8fa; border-left: 4px solid #d0d7de; padding: 12px; }
.inkhub-root img { max-width: 100%; height: auto; }
.inkhub-root code { background-color: #f3f4f6; border-radius: 4px; padding: 2px; }
.inkhub-root pre { background-color: #f6f8fa; padding: 16px; overflow-wrap: break-word; }
.inkhub-root table { width: 100%; border-collapse: collapse; }
.inkhub-root th { border: 1px solid #d0d7de; padding: 8px; }
.inkhub-root td { border: 1px solid #d0d7de; padding: 8px; }`
	case BuiltinMinimalID:
		manifest.Name = "InkHub Minimal"
		manifest.Description = "InkHub 极简微信公众号模板"
		css = `.inkhub-root { color: #111111; font-family: {{ font-family.bodyFont }}; }
.inkhub-root h1 { font-size: 24px; margin-bottom: 20px; }
.inkhub-root h2 { font-size: 20px; margin-top: 24px; }
.inkhub-root h3 { font-size: 18px; }
.inkhub-root p { font-size: 16px; line-height: 1.7; margin-bottom: 14px; }
.inkhub-root blockquote { border-left: 3px solid #999999; padding-left: 12px; }
.inkhub-root img { max-width: 100%; height: auto; }
.inkhub-root pre { background-color: #f7f7f7; padding: 12px; }
.inkhub-root table { width: 100%; border-collapse: collapse; }
.inkhub-root th { border: 1px solid #cccccc; padding: 6px; }
.inkhub-root td { border: 1px solid #cccccc; padding: 6px; }`
	default:
		return Validated{}, fmt.Errorf("未知内置模板: %s", id)
	}
	if err := validateCSS(css, manifest.Variables); err != nil {
		return Validated{}, fmt.Errorf("内置模板校验失败: %w", err)
	}
	// 摘要必须绑定渲染语义，不能只绑定 CSS，否则目标变更会复用旧缓存。
	sum := sha256.Sum256([]byte(id + "\x00" + manifest.Version + "\x00" + manifest.Target + "\x00" + manifest.Format + "\x00" + manifest.Renderer + "\x00" + css))
	return Validated{Manifest: manifest, CSS: css, Digest: hex.EncodeToString(sum[:])}, nil
}
