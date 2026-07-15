// Package markdown 实现只读的普通 Markdown Folder Source Provider。
package markdown

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Factory 从授权目录配置构建 Markdown Folder Provider。
type Factory struct{}

var _ contracts.SourceProviderFactory = (*Factory)(nil)

// NewFactory 创建 Markdown Folder Provider 工厂。
func NewFactory() *Factory { return &Factory{} }

// Type 返回稳定的 Markdown Folder Provider 类型。
func (*Factory) Type() contracts.ProviderType { return contracts.ProviderMarkdownFolder }

// Descriptor 返回普通 Markdown 只读能力。
func (*Factory) Descriptor() contracts.SourceDescriptor {
	return contracts.SourceDescriptor{Descriptor: contracts.Descriptor{
		Type: contracts.ProviderMarkdownFolder, DisplayName: "Markdown Folder", Version: "1",
		Capabilities: []contracts.Capability{contracts.CapabilityScan, contracts.CapabilityRead, contracts.CapabilityWatch},
	}, Formats: []string{"markdown"}}
}

// Build 解析配置并校验根目录位于授权范围内。
func (*Factory) Build(_ context.Context, ref contracts.ProviderRef, view contracts.ConfigView, _ contracts.SecretResolver) (contracts.SourceProvider, error) {
	if ref.ID == "" || ref.Type != contracts.ProviderMarkdownFolder {
		return nil, fmt.Errorf("Markdown Folder Provider 引用无效")
	}
	var raw struct {
		Root             string   `json:"root"`
		ContentRoots     []string `json:"content_roots,omitempty"`
		IgnoredFolders   []string `json:"ignored_folders,omitempty"`
		IgnoredFileNames []string `json:"ignored_file_names,omitempty"`
	}
	if err := json.Unmarshal(view.Data, &raw); err != nil {
		return nil, fmt.Errorf("解析 Markdown Folder 配置: %w", err)
	}
	root, err := filepath.Abs(raw.Root)
	if err != nil || !authorized(root, view.AllowedRoots) {
		return nil, fmt.Errorf("Markdown Folder 不在授权范围内")
	}
	return New(Config{SourceID: ref.ID, Root: root, ContentRoots: raw.ContentRoots, IgnoredFolders: raw.IgnoredFolders, IgnoredFileNames: raw.IgnoredFileNames})
}

func authorized(path string, roots []string) bool {
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
