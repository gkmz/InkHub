package hugo

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Factory 构建 Hugo 标准 Taxonomy Provider。
type Factory struct{}

var _ contracts.TaxonomyProviderFactory = (*Factory)(nil)

// NewFactory 创建 Hugo Taxonomy Provider 工厂。
func NewFactory() *Factory { return &Factory{} }

// Type 返回 Hugo Provider 类型。
func (*Factory) Type() contracts.ProviderType { return contracts.ProviderHugo }

// Descriptor 返回 Hugo taxonomy 工厂能力。
func (*Factory) Descriptor() contracts.TaxonomyDescriptor { return (&Provider{}).Descriptor() }

// Build 从 Hugo Publish 实例配置中提取 taxonomy 所需字段。
func (*Factory) Build(_ context.Context, ref contracts.ProviderRef, view contracts.ConfigView, _ contracts.SecretResolver) (contracts.TaxonomyProvider, error) {
	if ref.ID == "" || ref.Type != contracts.ProviderHugo {
		return nil, fmt.Errorf("Hugo Taxonomy Provider 引用无效")
	}
	var raw map[string]any
	if err := json.Unmarshal(view.Data, &raw); err != nil {
		return nil, fmt.Errorf("解析 Hugo Provider 配置: %w", err)
	}
	root, _ := raw["root"].(string)
	contentDir, _ := raw["content_dir"].(string)
	absolute, err := filepath.Abs(root)
	if err != nil || !authorizedPath(absolute, view.AllowedRoots) {
		return nil, fmt.Errorf("Hugo 根目录不在授权范围内")
	}
	return New(Config{ProviderID: ref.ID, Root: absolute, ContentDir: contentDir})
}

func authorizedPath(path string, roots []string) bool {
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
