package registry

import (
	"context"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestRegistryRegistersAndBuildsTypedAIProvider(t *testing.T) {
	t.Parallel()

	factory := fakeAIFactory{
		descriptor: contracts.AIDescriptor{Descriptor: contracts.Descriptor{
			Type:         contracts.ProviderOpenAI,
			DisplayName:  "OpenAI Compatible",
			Version:      "1",
			Capabilities: []contracts.Capability{contracts.CapabilityStructuredOutput},
		}},
	}
	registry := New(nil)
	if err := registry.RegisterAI(factory); err != nil {
		t.Fatalf("注册 AI Provider: %v", err)
	}

	descriptor, err := registry.Descriptor(contracts.ProviderOpenAI)
	if err != nil {
		t.Fatalf("读取 Descriptor: %v", err)
	}
	if descriptor.DisplayName != "OpenAI Compatible" || descriptor.Type != contracts.ProviderOpenAI {
		t.Fatalf("Descriptor 不匹配: %+v", descriptor)
	}

	provider, err := registry.BuildAI(context.Background(), contracts.ProviderRef{
		ID:   "provider_ai",
		Type: contracts.ProviderOpenAI,
	}, contracts.ConfigView{})
	if err != nil {
		t.Fatalf("构建 AI Provider: %v", err)
	}
	if provider.Descriptor().Type != contracts.ProviderOpenAI {
		t.Fatalf("构建了错误类型的 Provider: %q", provider.Descriptor().Type)
	}
}

func TestRegistryRejectsDuplicateAndUnknownProviderTypes(t *testing.T) {
	t.Parallel()

	factory := fakeAIFactory{descriptor: contracts.AIDescriptor{Descriptor: contracts.Descriptor{
		Type:        contracts.ProviderOpenAI,
		DisplayName: "OpenAI Compatible",
		Version:     "1",
	}}}
	registry := New(nil)
	if err := registry.RegisterAI(factory); err != nil {
		t.Fatalf("首次注册 AI Provider: %v", err)
	}
	if err := registry.RegisterAI(factory); err == nil {
		t.Fatal("重复注册应失败")
	}
	if _, err := registry.Descriptor(contracts.ProviderType("missing")); err == nil {
		t.Fatal("读取未知 Provider Descriptor 应失败")
	}
	if _, err := registry.BuildAI(context.Background(), contracts.ProviderRef{
		ID: "provider_missing", Type: contracts.ProviderType("missing"),
	}, contracts.ConfigView{}); err == nil {
		t.Fatal("构建未知 AI Provider 应失败")
	}
}

func TestRegistryBuildsSourceAndPublishProviders(t *testing.T) {
	t.Parallel()

	registry := New(nil)
	if err := registry.RegisterSource(fakeSourceFactory{}); err != nil {
		t.Fatalf("注册 Source Provider: %v", err)
	}
	if err := registry.RegisterPublish(fakePublishFactory{}); err != nil {
		t.Fatalf("注册 Publish Provider: %v", err)
	}
	source, err := registry.BuildSource(context.Background(), contracts.ProviderRef{ID: "source", Type: contracts.ProviderObsidian}, contracts.ConfigView{})
	if err != nil || source.Descriptor().Type != contracts.ProviderObsidian {
		t.Fatalf("构建 Source Provider: provider=%v err=%v", source, err)
	}
	publish, err := registry.BuildPublish(context.Background(), contracts.ProviderRef{ID: "publish", Type: contracts.ProviderHugo}, contracts.ConfigView{})
	if err != nil || publish.Descriptor().Type != contracts.ProviderHugo {
		t.Fatalf("构建 Publish Provider: provider=%v err=%v", publish, err)
	}
}

func TestRegistryBuildsTaxonomyProvider(t *testing.T) {
	t.Parallel()
	runtime := New(nil)
	if err := runtime.RegisterTaxonomy(fakeTaxonomyFactory{}); err != nil {
		t.Fatalf("注册 Taxonomy Provider: %v", err)
	}
	provider, err := runtime.BuildTaxonomy(context.Background(), contracts.ProviderRef{ID: "taxonomy", Type: contracts.ProviderHugo}, contracts.ConfigView{})
	if err != nil || provider.Descriptor().Type != contracts.ProviderHugo {
		t.Fatalf("构建 Taxonomy Provider: provider=%v err=%v", provider, err)
	}
}

type fakeAIFactory struct {
	descriptor contracts.AIDescriptor
}

func (f fakeAIFactory) Type() contracts.ProviderType { return f.descriptor.Type }

func (f fakeAIFactory) Descriptor() contracts.AIDescriptor { return f.descriptor }

func (f fakeAIFactory) Build(
	context.Context,
	contracts.ProviderRef,
	contracts.ConfigView,
	contracts.SecretResolver,
) (contracts.AIProvider, error) {
	return fakeAIProvider{descriptor: f.descriptor}, nil
}

type fakeAIProvider struct {
	descriptor contracts.AIDescriptor
}

func (p fakeAIProvider) Descriptor() contracts.AIDescriptor { return p.descriptor }

func (fakeAIProvider) Validate(context.Context) error { return nil }

func (fakeAIProvider) Generate(context.Context, contracts.AIRequest) (contracts.AIResponse, error) {
	return contracts.AIResponse{}, nil
}

type fakeSourceFactory struct{}

func (fakeSourceFactory) Type() contracts.ProviderType { return contracts.ProviderObsidian }
func (fakeSourceFactory) Descriptor() contracts.SourceDescriptor {
	return contracts.SourceDescriptor{Descriptor: contracts.Descriptor{Type: contracts.ProviderObsidian}}
}
func (fakeSourceFactory) Build(context.Context, contracts.ProviderRef, contracts.ConfigView, contracts.SecretResolver) (contracts.SourceProvider, error) {
	return fakeSourceProvider{}, nil
}

type fakeSourceProvider struct{}

func (fakeSourceProvider) Descriptor() contracts.SourceDescriptor {
	return contracts.SourceDescriptor{Descriptor: contracts.Descriptor{Type: contracts.ProviderObsidian}}
}
func (fakeSourceProvider) Validate(context.Context) error { return nil }
func (fakeSourceProvider) Scan(context.Context, contracts.ScanCursor) (contracts.ScanResult, error) {
	return contracts.ScanResult{}, nil
}
func (fakeSourceProvider) Read(context.Context, contracts.SourceRef) (contracts.SourceDocument, error) {
	return contracts.SourceDocument{}, nil
}
func (fakeSourceProvider) WriteMetadata(context.Context, contracts.MetadataWriteCommand) (contracts.SourceDocument, error) {
	return contracts.SourceDocument{}, nil
}
func (fakeSourceProvider) Watch(context.Context, chan<- contracts.SourceChange) error { return nil }

type fakePublishFactory struct{}

func (fakePublishFactory) Type() contracts.ProviderType { return contracts.ProviderHugo }
func (fakePublishFactory) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{Descriptor: contracts.Descriptor{Type: contracts.ProviderHugo}}
}
func (fakePublishFactory) Build(context.Context, contracts.ProviderRef, contracts.ConfigView, contracts.SecretResolver) (contracts.PublishProvider, error) {
	return fakePublishProvider{}, nil
}

type fakePublishProvider struct{}

func (fakePublishProvider) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{Descriptor: contracts.Descriptor{Type: contracts.ProviderHugo}}
}
func (fakePublishProvider) Validate(context.Context) error { return nil }
func (fakePublishProvider) Preflight(context.Context, contracts.PublishInput) (contracts.PreflightResult, error) {
	return contracts.PreflightResult{}, nil
}
func (fakePublishProvider) Prepare(context.Context, contracts.PublishInput) (contracts.PreparedArtifact, error) {
	return contracts.PreparedArtifact{}, nil
}
func (fakePublishProvider) Deliver(context.Context, contracts.PreparedArtifact) (contracts.DeliveryResult, error) {
	return contracts.DeliveryResult{}, nil
}

type fakeTaxonomyFactory struct{}

func (fakeTaxonomyFactory) Type() contracts.ProviderType { return contracts.ProviderHugo }
func (fakeTaxonomyFactory) Descriptor() contracts.TaxonomyDescriptor {
	return contracts.TaxonomyDescriptor{Descriptor: contracts.Descriptor{Type: contracts.ProviderHugo}}
}
func (fakeTaxonomyFactory) Build(context.Context, contracts.ProviderRef, contracts.ConfigView, contracts.SecretResolver) (contracts.TaxonomyProvider, error) {
	return fakeTaxonomyProvider{}, nil
}

type fakeTaxonomyProvider struct{}

func (fakeTaxonomyProvider) Descriptor() contracts.TaxonomyDescriptor {
	return contracts.TaxonomyDescriptor{Descriptor: contracts.Descriptor{Type: contracts.ProviderHugo}}
}
func (fakeTaxonomyProvider) Validate(context.Context) error { return nil }
func (fakeTaxonomyProvider) Discover(context.Context, contracts.TaxonomyCursor) (contracts.TaxonomySnapshot, error) {
	return contracts.TaxonomySnapshot{}, nil
}
func (fakeTaxonomyProvider) PlanChange(context.Context, contracts.TaxonomyCommand) (contracts.TaxonomyChangeSet, error) {
	return contracts.TaxonomyChangeSet{}, nil
}
func (fakeTaxonomyProvider) ApplyChange(context.Context, contracts.TaxonomyChangeSet) (contracts.TaxonomySnapshot, error) {
	return contracts.TaxonomySnapshot{}, nil
}
func (fakeTaxonomyProvider) Watch(context.Context, chan<- contracts.TaxonomyChange) error { return nil }
