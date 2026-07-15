// Package registry 管理编译期注册的类型化 Provider 工厂。
package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Registry 只管理 Provider 类型、工厂和能力发现，不保存业务状态。
type Registry struct {
	mu       sync.RWMutex
	secrets  contracts.SecretResolver
	source   map[contracts.ProviderType]contracts.SourceProviderFactory
	ai       map[contracts.ProviderType]contracts.AIProviderFactory
	publish  map[contracts.ProviderType]contracts.PublishProviderFactory
	taxonomy map[contracts.ProviderType]contracts.TaxonomyProviderFactory
}

// SupportsTaxonomy 返回指定类型是否注册了 Taxonomy Provider 工厂。
func (r *Registry) SupportsTaxonomy(providerType contracts.ProviderType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.taxonomy[providerType]
	return exists
}

// New 创建空的 Provider Registry。
func New(secrets contracts.SecretResolver) *Registry {
	return &Registry{
		secrets:  secrets,
		source:   make(map[contracts.ProviderType]contracts.SourceProviderFactory),
		ai:       make(map[contracts.ProviderType]contracts.AIProviderFactory),
		publish:  make(map[contracts.ProviderType]contracts.PublishProviderFactory),
		taxonomy: make(map[contracts.ProviderType]contracts.TaxonomyProviderFactory),
	}
}

// RegisterSource 注册一个编译期 Source Provider 工厂。
func (r *Registry) RegisterSource(factory contracts.SourceProviderFactory) error {
	if factory == nil || factory.Type() == "" || factory.Descriptor().Type != factory.Type() {
		return fmt.Errorf("Source Provider 工厂或 Descriptor 无效")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.source[factory.Type()]; exists {
		return fmt.Errorf("Source Provider 已注册: %s", factory.Type())
	}
	r.source[factory.Type()] = factory
	return nil
}

// RegisterAI 注册一个编译期 AI Provider 工厂。
func (r *Registry) RegisterAI(factory contracts.AIProviderFactory) error {
	if factory == nil || factory.Type() == "" {
		return fmt.Errorf("AI Provider 工厂或类型为空")
	}
	descriptor := factory.Descriptor()
	if descriptor.Type != factory.Type() {
		return fmt.Errorf("AI Provider 类型与 Descriptor 不一致")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.ai[factory.Type()]; exists {
		return fmt.Errorf("AI Provider 已注册: %s", factory.Type())
	}
	r.ai[factory.Type()] = factory
	return nil
}

// Descriptor 返回已注册 Provider 的稳定描述。
func (r *Registry) Descriptor(providerType contracts.ProviderType) (contracts.Descriptor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if factory, exists := r.ai[providerType]; exists {
		return factory.Descriptor().Descriptor, nil
	}
	if factory, exists := r.source[providerType]; exists {
		return factory.Descriptor().Descriptor, nil
	}
	if factory, exists := r.publish[providerType]; exists {
		return factory.Descriptor().Descriptor, nil
	}
	if factory, exists := r.taxonomy[providerType]; exists {
		return factory.Descriptor().Descriptor, nil
	}
	return contracts.Descriptor{}, fmt.Errorf("未知 Provider 类型: %s", providerType)
}

// BuildSource 根据实例引用构建对应类型的 Source Provider。
func (r *Registry) BuildSource(ctx context.Context, ref contracts.ProviderRef, config contracts.ConfigView) (contracts.SourceProvider, error) {
	if ref.ID == "" || ref.Type == "" {
		return nil, fmt.Errorf("Provider 实例 ID 或类型为空")
	}
	r.mu.RLock()
	factory, exists := r.source[ref.Type]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("未知 Source Provider 类型: %s", ref.Type)
	}
	return factory.Build(ctx, ref, config, r.secrets)
}

// BuildAI 根据实例引用构建对应类型的 AI Provider。
func (r *Registry) BuildAI(ctx context.Context, ref contracts.ProviderRef, config contracts.ConfigView) (contracts.AIProvider, error) {
	if ref.ID == "" || ref.Type == "" {
		return nil, fmt.Errorf("Provider 实例 ID 或类型为空")
	}
	r.mu.RLock()
	factory, exists := r.ai[ref.Type]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("未知 AI Provider 类型: %s", ref.Type)
	}
	return factory.Build(ctx, ref, config, r.secrets)
}

// RegisterPublish 注册一个编译期 Publish Provider 工厂。
func (r *Registry) RegisterPublish(factory contracts.PublishProviderFactory) error {
	if factory == nil || factory.Type() == "" || factory.Descriptor().Type != factory.Type() {
		return fmt.Errorf("Publish Provider 工厂或 Descriptor 无效")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.publish[factory.Type()]; exists {
		return fmt.Errorf("Publish Provider 已注册: %s", factory.Type())
	}
	r.publish[factory.Type()] = factory
	return nil
}

// BuildPublish 根据实例引用构建对应类型的 Publish Provider。
func (r *Registry) BuildPublish(ctx context.Context, ref contracts.ProviderRef, config contracts.ConfigView) (contracts.PublishProvider, error) {
	if ref.ID == "" || ref.Type == "" {
		return nil, fmt.Errorf("Provider 实例 ID 或类型为空")
	}
	r.mu.RLock()
	factory, exists := r.publish[ref.Type]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("未知 Publish Provider 类型: %s", ref.Type)
	}
	return factory.Build(ctx, ref, config, r.secrets)
}

// RegisterTaxonomy 注册一个编译期 Taxonomy Provider 工厂。
func (r *Registry) RegisterTaxonomy(factory contracts.TaxonomyProviderFactory) error {
	if factory == nil || factory.Type() == "" || factory.Descriptor().Type != factory.Type() {
		return fmt.Errorf("Taxonomy Provider 工厂或 Descriptor 无效")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.taxonomy[factory.Type()]; exists {
		return fmt.Errorf("Taxonomy Provider 已注册: %s", factory.Type())
	}
	r.taxonomy[factory.Type()] = factory
	return nil
}

// BuildTaxonomy 根据实例引用构建对应类型的 Taxonomy Provider。
func (r *Registry) BuildTaxonomy(ctx context.Context, ref contracts.ProviderRef, config contracts.ConfigView) (contracts.TaxonomyProvider, error) {
	if ref.ID == "" || ref.Type == "" {
		return nil, fmt.Errorf("Provider 实例 ID 或类型为空")
	}
	r.mu.RLock()
	factory, exists := r.taxonomy[ref.Type]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("未知 Taxonomy Provider 类型: %s", ref.Type)
	}
	return factory.Build(ctx, ref, config, r.secrets)
}
