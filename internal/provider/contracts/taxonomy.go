package contracts

import "context"

// TaxonomyProvider 负责发现、规划、应用和监听发布平台的分类体系。
type TaxonomyProvider interface {
	Descriptor() TaxonomyDescriptor
	Validate(ctx context.Context) error
	Discover(ctx context.Context, cursor TaxonomyCursor) (TaxonomySnapshot, error)
	PlanChange(ctx context.Context, command TaxonomyCommand) (TaxonomyChangeSet, error)
	ApplyChange(ctx context.Context, change TaxonomyChangeSet) (TaxonomySnapshot, error)
	Watch(ctx context.Context, changes chan<- TaxonomyChange) error
}

// TaxonomyDescriptor 描述 Provider 支持的原生 taxonomy 与变更能力。
type TaxonomyDescriptor struct {
	Descriptor
	Writable bool
}

// TaxonomyProviderFactory 构建一个类型安全的 Taxonomy Provider 实例。
type TaxonomyProviderFactory interface {
	Type() ProviderType
	Descriptor() TaxonomyDescriptor
	Build(ctx context.Context, ref ProviderRef, config ConfigView, secrets SecretResolver) (TaxonomyProvider, error)
}

// TaxonomyCursor 标识上一次成功发现的权威 revision。
type TaxonomyCursor struct {
	Revision string
}

// TaxonomyTerm 是发布平台 taxonomy term 的标准投影。
type TaxonomyTerm struct {
	Kind          string
	Key           string
	Name          string
	CanonicalName string
	UsageCount    int
	Metadata      map[string]string
}

// TaxonomySnapshot 是一次完整或未变化的 taxonomy 发现结果。
type TaxonomySnapshot struct {
	ProviderRef ProviderRef
	Revision    string
	Terms       []TaxonomyTerm
	Complete    bool
	Unchanged   bool
	Diagnostics []Diagnostic
}

// TaxonomyCommandKind 描述受控 term 变更类型。
type TaxonomyCommandKind string

const (
	TaxonomyCreateTerm TaxonomyCommandKind = "create_term"
	TaxonomyUpdateTerm TaxonomyCommandKind = "update_term"
	TaxonomyDeleteTerm TaxonomyCommandKind = "delete_term"
)

// TaxonomyCommand 描述用户确认前需要规划的 term 变更。
type TaxonomyCommand struct {
	Kind             TaxonomyCommandKind
	Term             TaxonomyTerm
	ExpectedRevision string
}

// TaxonomyFileChange 描述一个 Provider 原生文件的前后内容。
type TaxonomyFileChange struct {
	RelativePath string
	Before       string
	After        string
}

// TaxonomyChangeSet 是可审阅且带乐观并发 revision 的变更计划。
type TaxonomyChangeSet struct {
	ProviderRef      ProviderRef
	ExpectedRevision string
	Files            []TaxonomyFileChange
}

// TaxonomyChange 表示外部 taxonomy 资源发生变化，需要重新发现。
type TaxonomyChange struct {
	Revision string
	Path     string
}
