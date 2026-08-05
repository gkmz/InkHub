package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const mermaidInkEndpoint = "https://mermaid.ink/img/"

const (
	// MermaidThemeHandDrawn 是旧版默认的暖色手绘图表样式。
	MermaidThemeHandDrawn = "handdrawn"
	// MermaidThemeModern 是适合技术流程图的蓝灰现代样式。
	MermaidThemeModern = "modern"
)

const mermaidModernInit = "%%{init: {'theme': 'base', 'themeVariables': {'fontFamily': 'Arial, PingFang SC, Microsoft YaHei, sans-serif', 'fontSize': '16px', 'primaryColor': '#F8FAFC', 'primaryTextColor': '#0F2742', 'primaryBorderColor': '#2E6FD8', 'lineColor': '#4A6079', 'secondaryColor': '#ECF2FF', 'tertiaryColor': '#F4F7FB', 'clusterBkg': '#F1F5FA', 'clusterBorder': '#A7B6C7', 'edgeLabelBackground': '#FFFFFF', 'nodeBorder': '#2E6FD8', 'mainBkg': '#FFFFFF', 'textColor': '#102A43'}, 'flowchart': {'curve': 'catmullRom', 'htmlLabels': false, 'nodeSpacing': 46, 'rankSpacing': 58, 'padding': 18, 'wrappingWidth': 180, 'diagramPadding': 8}}}%%"
const mermaidHandDrawnInit = "%%{init: {'theme': 'base', 'look': 'handDrawn', 'themeVariables': {'fontFamily': 'Comic Sans MS, Bradley Hand, PingFang SC, Microsoft YaHei, sans-serif', 'fontSize': '18px', 'primaryColor': '#FFF8E8', 'primaryTextColor': '#3A2A10', 'primaryBorderColor': '#C77700', 'lineColor': '#8A4B00', 'secondaryColor': '#FFF3D8', 'tertiaryColor': '#FFF9EE', 'clusterBkg': '#FFF6E3', 'clusterBorder': '#D3912D', 'edgeLabelBackground': '#FFFDF7', 'nodeBorder': '#C77700', 'mainBkg': '#FFFFFF', 'textColor': '#3A2A10'}, 'flowchart': {'curve': 'basis', 'htmlLabels': false, 'nodeSpacing': 64, 'rankSpacing': 84, 'padding': 44}}}%%"

// MermaidInkRenderer 使用 mermaid.ink 将图表源码转换为公开 HTTPS 图片。
//
// 微信图文只能稳定接收可访问图片，因此这里把源码编码进 URL；相同源码
// 会得到相同地址，并附带摘要版本参数，方便 CDN 缓存失效。
type MermaidInkRenderer struct{}

// NewMermaidInkRenderer 创建默认 Mermaid 图片转换器。
func NewMermaidInkRenderer() MermaidRenderer {
	return MermaidInkRenderer{}
}

// NormalizeMermaidTheme 校验并规范化微信支持的 Mermaid 样式。
func NormalizeMermaidTheme(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == MermaidThemeHandDrawn {
		return MermaidThemeHandDrawn, nil
	}
	if value == MermaidThemeModern {
		return MermaidThemeModern, nil
	}
	return "", fmt.Errorf("不支持的 Mermaid 样式: %s", value)
}

// Render 返回带指定主题的 Mermaid 图表公开图片地址，不在本地执行浏览器或脚本。
func (MermaidInkRenderer) Render(ctx context.Context, source, digest, theme string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("Mermaid 源码为空")
	}
	if len(source) > 64<<10 {
		return "", fmt.Errorf("Mermaid 源码超过 64 KiB 限制")
	}
	normalizedTheme, err := NormalizeMermaidTheme(theme)
	if err != nil {
		return "", err
	}
	// 文章自行声明 init 时尊重作者配置，否则注入页面选择的样式。
	if !hasMermaidInit(source) {
		if normalizedTheme == MermaidThemeModern {
			source = mermaidModernInit + "\n" + source
		} else {
			source = mermaidHandDrawnInit + "\n" + source
		}
	}
	payload, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: source})
	if err != nil {
		return "", fmt.Errorf("编码 Mermaid 源码: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	if digest == "" {
		sum := sha256.Sum256([]byte(normalizedTheme + "\x00" + source))
		digest = hex.EncodeToString(sum[:])
	}
	if len(digest) > 16 {
		digest = digest[:16]
	}
	return mermaidInkEndpoint + encoded + "?v=" + digest, nil
}

func hasMermaidInit(source string) bool {
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, "%%{init:")
	}
	return false
}
