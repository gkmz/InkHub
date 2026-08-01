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

// MermaidInkRenderer 使用 mermaid.ink 将图表源码转换为公开 HTTPS 图片。
//
// 微信图文只能稳定接收可访问图片，因此这里把源码编码进 URL；相同源码
// 会得到相同地址，并附带摘要版本参数，方便 CDN 缓存失效。
type MermaidInkRenderer struct{}

// NewMermaidInkRenderer 创建默认 Mermaid 图片转换器。
func NewMermaidInkRenderer() MermaidRenderer {
	return MermaidInkRenderer{}
}

// Render 返回 Mermaid 图表的公开图片地址，不在本地执行浏览器或脚本。
func (MermaidInkRenderer) Render(ctx context.Context, source, digest string) (string, error) {
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
	payload, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: source})
	if err != nil {
		return "", fmt.Errorf("编码 Mermaid 源码: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	if digest == "" {
		sum := sha256.Sum256([]byte(source))
		digest = hex.EncodeToString(sum[:])
	}
	if len(digest) > 16 {
		digest = digest[:16]
	}
	return mermaidInkEndpoint + encoded + "?v=" + digest, nil
}
