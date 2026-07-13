package hugo

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

const hugoConfigSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["root","staging_root"],
  "properties":{
    "root":{"type":"string","minLength":1},
    "staging_root":{"type":"string","minLength":1},
    "section":{"type":"string"},
    "base_url":{"type":"string"},
    "artifact_ttl_seconds":{"type":"integer","minimum":1}
  }
}`

// Factory 从已授权配置构建 Hugo Publish Provider。
type Factory struct {
	builder Builder
}

var _ contracts.PublishProviderFactory = (*Factory)(nil)

// NewFactory 创建 Hugo Provider 工厂。
func NewFactory(builder Builder) *Factory { return &Factory{builder: builder} }

// Type 返回稳定的 Hugo Provider 类型。
func (f *Factory) Type() contracts.ProviderType { return contracts.ProviderHugo }

// Descriptor 返回 Hugo 工厂的配置 schema 和能力声明。
func (f *Factory) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{Descriptor: contracts.Descriptor{
		Type: contracts.ProviderHugo, DisplayName: "Hugo", Version: "1", ConfigSchema: hugoConfigSchema,
		Capabilities: []contracts.Capability{
			contracts.CapabilityPreview, contracts.CapabilityTaxonomy, contracts.CapabilityCanonical,
		},
	}}
}

// Build 严格解码配置并校验 Hugo 与 staging 路径授权。
func (f *Factory) Build(_ context.Context, ref contracts.ProviderRef, view contracts.ConfigView, _ contracts.SecretResolver) (contracts.PublishProvider, error) {
	if ref.ID == "" || ref.Type != contracts.ProviderHugo {
		return nil, providerError("hugo.config_invalid", "Hugo Provider 引用无效", contracts.ErrorValidation, false, nil)
	}
	var raw struct {
		Root               string `json:"root"`
		StagingRoot        string `json:"staging_root"`
		Section            string `json:"section,omitempty"`
		BaseURL            string `json:"base_url,omitempty"`
		ArtifactTTLSeconds int64  `json:"artifact_ttl_seconds,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(view.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, providerError("hugo.config_invalid", "Hugo Provider 配置无效", contracts.ErrorValidation, false, err)
	}
	root, err := filepath.Abs(raw.Root)
	if err != nil {
		return nil, providerError("hugo.config_invalid", "Hugo 根目录无效", contracts.ErrorValidation, false, err)
	}
	staging, err := filepath.Abs(raw.StagingRoot)
	if err != nil {
		return nil, providerError("hugo.config_invalid", "Hugo staging 目录无效", contracts.ErrorValidation, false, err)
	}
	if !authorizedPath(root, view.AllowedRoots) || !authorizedPath(staging, view.AllowedRoots) {
		return nil, providerError("hugo.path_unauthorized", "Hugo 路径不在授权范围内", contracts.ErrorUnauthorizedResource, false, nil)
	}
	provider, err := New(Config{
		Root: root, StagingRoot: staging, Section: raw.Section, BaseURL: raw.BaseURL,
		ArtifactTTL: time.Duration(raw.ArtifactTTLSeconds) * time.Second,
	}, f.builder)
	if err != nil {
		return nil, providerError("hugo.config_invalid", "Hugo Provider 配置无效", contracts.ErrorValidation, false, err)
	}
	return provider, nil
}

func authorizedPath(path string, roots []string) bool {
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err == nil && withinOrEqual(path, absolute) {
			return true
		}
	}
	return false
}
