package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

const wechatConfigSchema = `{"type":"object","additionalProperties":false,"required":["staging_root"],"properties":{"staging_root":{"type":"string","minLength":1},"artifact_ttl_seconds":{"type":"integer","minimum":1},"variables":{"type":"object"}}}`

// Factory 使用已注入的平台能力构建微信 Provider。
type Factory struct {
	templates TemplateLoader
	uploader  AssetUploader
	clipboard Clipboard
	mermaid   MermaidRenderer
}

var _ contracts.PublishProviderFactory = (*Factory)(nil)

// NewFactory 创建微信 Provider 工厂。
func NewFactory(templates TemplateLoader, uploader AssetUploader, clipboard Clipboard, mermaid ...MermaidRenderer) *Factory {
	var renderer MermaidRenderer
	if len(mermaid) > 0 {
		renderer = mermaid[0]
	}
	return &Factory{templates: templates, uploader: uploader, clipboard: clipboard, mermaid: renderer}
}

// Type 返回稳定的微信 Provider 类型。
func (f *Factory) Type() contracts.ProviderType { return contracts.ProviderWeChat }

// Descriptor 返回微信工厂配置和人工确认能力。
func (f *Factory) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{Descriptor: contracts.Descriptor{
		Type: contracts.ProviderWeChat, DisplayName: "微信公众号", Version: "1", ConfigSchema: wechatConfigSchema,
		Capabilities: []contracts.Capability{contracts.CapabilityPreview, contracts.CapabilityImages, contracts.CapabilityManualConfirmation},
	}}
}

// Build 严格解码配置并校验 staging 授权范围。
func (f *Factory) Build(_ context.Context, ref contracts.ProviderRef, view contracts.ConfigView, _ contracts.SecretResolver) (contracts.PublishProvider, error) {
	if ref.ID == "" || ref.Type != contracts.ProviderWeChat {
		return nil, providerError("wechat.config_invalid", "微信 Provider 引用无效", contracts.ErrorValidation, nil)
	}
	var raw struct {
		StagingRoot        string         `json:"staging_root"`
		ArtifactTTLSeconds int64          `json:"artifact_ttl_seconds,omitempty"`
		Variables          map[string]any `json:"variables,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(view.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, providerError("wechat.config_invalid", "微信 Provider 配置无效", contracts.ErrorValidation, err)
	}
	staging, err := filepath.Abs(raw.StagingRoot)
	if err != nil || !authorizedWechatPath(staging, view.AllowedRoots) {
		return nil, providerError("wechat.path_unauthorized", "微信 staging 不在授权范围内", contracts.ErrorUnauthorizedResource, err)
	}
	return New(Config{StagingRoot: staging, ArtifactTTL: time.Duration(raw.ArtifactTTLSeconds) * time.Second, Variables: raw.Variables, Mermaid: f.mermaid}, f.templates, f.uploader, f.clipboard)
}

func authorizedWechatPath(path string, roots []string) bool {
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(absolute, path)
		if err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))) {
			return true
		}
	}
	return false
}
