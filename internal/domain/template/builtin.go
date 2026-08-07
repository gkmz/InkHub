package template

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	BuiltinDefaultID = "inkhub-default"
)

// Builtin 返回经过同一 CSS 安全校验的内置模板。
func Builtin(id string) (Validated, error) {
	manifest := Manifest{
		SpecVersion: "1.1", Target: TargetWeChatHTML, Format: "css", Renderer: RendererWeChatHTMLV1,
		Compatibility: Compatibility{Providers: []string{"wechat"}, RendererVersion: "1"},
		ID:            id, Version: "1.0.0", Author: Author{Name: "InkHub"},
		License: "Apache-2.0", InkHubVersion: ">=1.0.0 <2.0.0", Variables: map[string]Variable{
			"accentColor": {Type: "color", Label: "强调色", Default: "#42b883"},
			"bodyFont":    {Type: "font-family", Label: "正文字体", Default: "system-sans", Options: []string{"system-sans", "system-serif"}},
		},
	}
	var css string
	switch id {
	case BuiltinDefaultID:
		manifest.Name = "InkHub 墨绿"
		manifest.Description = "InkHub 原版墨绿色微信公众号模板"
		css = `.inkhub-root { color: #2e3f4f; font-family: {{ font-family.bodyFont }}; font-size: 16px; line-height: 1.88; }
.inkhub-root h1 { color: #1a2733; font-size: 30px; font-weight: 700; line-height: 1.28; margin: 0 0 16px; }
.inkhub-root h2 { color: #1a2733; font-size: 23px; font-weight: 700; line-height: 1.32; margin: 44px 0 14px; padding-left: 14px; border-left: 4px solid #42b883; }
.inkhub-root h3 { color: #253545; font-size: 19px; font-weight: 700; line-height: 1.36; margin: 35px 0 10px; padding-left: 10px; border-left: 3px solid #42b883; }
.inkhub-root h4 { color: #3a4f5e; font-size: 17px; font-weight: 700; line-height: 1.38; margin: 29px 0 8px; }
.inkhub-root h5 { color: #1a2733; font-size: 16px; font-weight: 600; margin: 20px 0 6px; }
.inkhub-root h6 { color: #778899; font-size: 15px; font-weight: 600; margin: 20px 0 6px; }
.inkhub-root p { font-size: 16px; line-height: 1.88; margin: 21px 0; }
.inkhub-root strong { color: #1a2733; font-weight: 700; }
.inkhub-root em { color: #6a7d8a; font-style: italic; }
.inkhub-root a { color: {{ color.accentColor }}; font-weight: 600; text-decoration: none; border-bottom: 1px solid #a4ddc4; }
.inkhub-root ul { line-height: 1.88; margin: 16px 0 19px; padding-left: 26px; list-style-type: disc; }
.inkhub-root ol { line-height: 1.88; margin: 16px 0 19px; padding-left: 26px; list-style-type: decimal; }
.inkhub-root li { margin: 8px 0; }
.inkhub-root blockquote { background-color: #f7fcf9; border-left: 3px solid #42b883; border-radius: 0 8px 8px 0; color: #5c7080; font-style: italic; margin: 29px 0; padding: 11px 20px; }
.inkhub-root blockquote p { margin: 6px 0; }
.inkhub-root code { background-color: #f0f4f8; border-radius: 5px; color: #c7522a; display: inline; font-family: system-monospace; font-size: 1em; padding: 2px 7px; white-space: pre-wrap; word-break: break-word; }
.inkhub-root pre { background-color: #1a1b26; border: 1px solid #2a2d3e; border-radius: 8px; color: #c0caf5; line-height: 1.72; margin: 26px 0; overflow-wrap: break-word; padding: 18px; white-space: pre-wrap; }
.inkhub-root pre code { background-color: #1a1b26; color: #c0caf5; display: block; font-family: system-monospace; font-size: 13px; line-height: 1.72; margin: 0; padding: 0; white-space: pre-wrap; }
.inkhub-root table { border-collapse: collapse; border-spacing: 0; display: table; font-size: 15px; line-height: 1.65; margin-bottom: 22px; max-width: 100%; width: 100%; }
.inkhub-root tr { border-top: 1px solid #e4ecf2; }
.inkhub-root th { background-color: #eef3f8; border: 1px solid #dde6ee; color: #1a2733; font-weight: 700; padding: 9px 16px; text-align: left; vertical-align: top; }
.inkhub-root td { border: 1px solid #dde6ee; padding: 9px 16px; text-align: left; vertical-align: top; }
.inkhub-root img { display: block; height: auto; margin: 29px auto; max-width: 100%; }
.inkhub-root hr { border: 0; border-bottom: 1px solid #e4ecf2; margin: 35px 0; }`
		css += tokyoNightCodeCSS
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
