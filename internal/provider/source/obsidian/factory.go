package obsidian

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

const obsidianConfigSchema = `{"type":"object","additionalProperties":false,"required":["root"],"properties":{"root":{"type":"string","minLength":1},"content_roots":{"type":"array","items":{"type":"string"}},"ignored_folders":{"type":"array","items":{"type":"string"}},"ignored_file_names":{"type":"array","items":{"type":"string"}}}}`

// Factory 从持久化配置构建 Obsidian Source Provider。
type Factory struct{}

var _ contracts.SourceProviderFactory = (*Factory)(nil)

// NewFactory 创建 Obsidian Source Provider 工厂。
func NewFactory() *Factory { return &Factory{} }

// Type 返回稳定的 Obsidian Provider 类型。
func (*Factory) Type() contracts.ProviderType { return contracts.ProviderObsidian }

// Descriptor 返回 Obsidian 支持的格式、配置和能力。
func (*Factory) Descriptor() contracts.SourceDescriptor {
	return contracts.SourceDescriptor{Descriptor: contracts.Descriptor{
		Type: contracts.ProviderObsidian, DisplayName: "Obsidian", Version: "1", ConfigSchema: obsidianConfigSchema,
		Capabilities: []contracts.Capability{contracts.CapabilityScan, contracts.CapabilityRead, contracts.CapabilityWriteMetadata, contracts.CapabilityWatch, contracts.CapabilityImages},
	}, Formats: []string{"markdown", "obsidian-markdown"}}
}

// Build 严格解析配置，并确保 Vault 位于已授权路径中。
func (f *Factory) Build(_ context.Context, ref contracts.ProviderRef, view contracts.ConfigView, _ contracts.SecretResolver) (contracts.SourceProvider, error) {
	if ref.ID == "" || ref.Type != contracts.ProviderObsidian {
		return nil, fmt.Errorf("Obsidian Provider 引用无效")
	}
	var raw struct {
		Root             string   `json:"root"`
		ContentRoots     []string `json:"content_roots,omitempty"`
		IgnoredFolders   []string `json:"ignored_folders,omitempty"`
		IgnoredFileNames []string `json:"ignored_file_names,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(view.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析 Obsidian Provider 配置: %w", err)
	}
	root, err := filepath.Abs(raw.Root)
	if err != nil || !authorizedRoot(root, view.AllowedRoots) {
		return nil, fmt.Errorf("Obsidian Vault 不在授权范围内")
	}
	return New(Config{SourceID: ref.ID, Root: root, ContentRoots: raw.ContentRoots, IgnoredFolders: raw.IgnoredFolders, IgnoredFileNames: raw.IgnoredFileNames})
}

func authorizedRoot(path string, roots []string) bool {
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
