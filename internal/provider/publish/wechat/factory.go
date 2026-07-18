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

const wechatConfigSchema = `{"type":"object","additionalProperties":false,"required":["staging_root"],"properties":{"staging_root":{"type":"string","minLength":1},"artifact_ttl_seconds":{"type":"integer","minimum":1},"variables":{"type":"object"},"template":{"type":"string"},"github_owner":{"type":"string"},"github_repository":{"type":"string"},"github_branch":{"type":"string"},"github_prefix":{"type":"string"},"github_secret_ref":{"type":"string"}}}`

// AssetUploaderConfig 是微信 Provider 交给装配层的非敏感上传配置。
type AssetUploaderConfig struct {
	Owner      string
	Repository string
	Branch     string
	Prefix     string
}

// AssetUploaderBuilder 使用瞬时 Secret 创建具体图片上传器。
type AssetUploaderBuilder func(context.Context, AssetUploaderConfig, []byte) (AssetUploader, error)

// Factory 使用已注入的平台能力构建微信 Provider。
type Factory struct {
	templates TemplateLoader
	uploader  AssetUploader
	builder   AssetUploaderBuilder
	clipboard Clipboard
	mermaid   MermaidRenderer
}

// NewFactoryWithUploaderBuilder 创建按 Provider 实例动态装配图片上传器的工厂。
func NewFactoryWithUploaderBuilder(templates TemplateLoader, clipboard Clipboard, builder AssetUploaderBuilder, mermaid ...MermaidRenderer) *Factory {
	factory := NewFactory(templates, nil, clipboard, mermaid...)
	factory.builder = builder
	return factory
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
	}, DeliveryMode: contracts.DeliveryPrepareOnly}
}

// Build 严格解码配置并校验 staging 授权范围。
func (f *Factory) Build(ctx context.Context, ref contracts.ProviderRef, view contracts.ConfigView, secrets contracts.SecretResolver) (contracts.PublishProvider, error) {
	if ref.ID == "" || ref.Type != contracts.ProviderWeChat {
		return nil, providerError("wechat.config_invalid", "微信 Provider 引用无效", contracts.ErrorValidation, nil)
	}
	var raw struct {
		StagingRoot        string         `json:"staging_root"`
		ArtifactTTLSeconds int64          `json:"artifact_ttl_seconds,omitempty"`
		Variables          map[string]any `json:"variables,omitempty"`
		Template           string         `json:"template,omitempty"`
		GitHubOwner        string         `json:"github_owner,omitempty"`
		GitHubRepository   string         `json:"github_repository,omitempty"`
		GitHubBranch       string         `json:"github_branch,omitempty"`
		GitHubPrefix       string         `json:"github_prefix,omitempty"`
		GitHubSecretRef    string         `json:"github_secret_ref,omitempty"`
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
	uploader := f.uploader
	hasGitHub := raw.GitHubOwner != "" || raw.GitHubRepository != "" || raw.GitHubSecretRef != ""
	if hasGitHub {
		if raw.GitHubOwner == "" || raw.GitHubRepository == "" || raw.GitHubSecretRef == "" || f.builder == nil || secrets == nil {
			return nil, providerError("wechat.image_hosting_invalid", "微信图片仓库配置不完整", contracts.ErrorValidation, nil)
		}
		secret, err := secrets.Resolve(ctx, raw.GitHubSecretRef)
		if err != nil || len(secret.Bytes) == 0 {
			return nil, providerError("wechat.image_hosting_secret_missing", "微信图片仓库 Token 未配置", contracts.ErrorValidation, nil)
		}
		uploader, err = f.builder(ctx, AssetUploaderConfig{Owner: raw.GitHubOwner, Repository: raw.GitHubRepository, Branch: raw.GitHubBranch, Prefix: raw.GitHubPrefix}, secret.Bytes)
		for index := range secret.Bytes {
			secret.Bytes[index] = 0
		}
		if err != nil {
			return nil, providerError("wechat.image_hosting_invalid", "微信图片仓库配置无效", contracts.ErrorValidation, err)
		}
	}
	return New(Config{StagingRoot: staging, ArtifactTTL: time.Duration(raw.ArtifactTTLSeconds) * time.Second, Variables: raw.Variables, Mermaid: f.mermaid}, f.templates, uploader, f.clipboard)
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
