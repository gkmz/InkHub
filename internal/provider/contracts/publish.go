package contracts

import (
	"context"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
)

// PublishProvider 负责渠道预检、转换、预览和交付。
type PublishProvider interface {
	Descriptor() PublishDescriptor
	Validate(ctx context.Context) error
	Preflight(ctx context.Context, input PublishInput) (PreflightResult, error)
	Prepare(ctx context.Context, input PublishInput) (PreparedArtifact, error)
	Deliver(ctx context.Context, artifact PreparedArtifact) (DeliveryResult, error)
}

// PublishDescriptor 描述发布渠道的通用能力。
type PublishDescriptor struct {
	Descriptor
	DeliveryMode DeliveryMode
}

// DeliveryMode 描述产物生成后的标准交付策略。
type DeliveryMode string

const (
	// DeliveryAutomatic 表示 Application 应继续执行自动交付。
	DeliveryAutomatic DeliveryMode = "automatic"
	// DeliveryManualConfirmation 表示交付后仍需用户在外部平台确认。
	DeliveryManualConfirmation DeliveryMode = "manual_confirmation"
	// DeliveryPrepareOnly 表示 Provider 只生成产物，不由后台任务自动交付。
	DeliveryPrepareOnly DeliveryMode = "prepare_only"
)

// PublishProviderFactory 构建一个类型安全的 Publish Provider 实例。
type PublishProviderFactory interface {
	Type() ProviderType
	Descriptor() PublishDescriptor
	Build(ctx context.Context, ref ProviderRef, config ConfigView, secrets SecretResolver) (PublishProvider, error)
}

// PublishInput 是发布渠道接收的标准文章快照。
type PublishInput struct {
	OperationID  string
	Article      article.Article
	Body         string
	ResourceRefs []ResourceRef
	Diagnostics  []Diagnostic
	ContentHash  string
	TemplateRef  *TemplateRef
	// MermaidTheme 固定当前发布任务使用的图表样式，避免异步执行时读取到变化后的配置。
	MermaidTheme     string
	ExpectedRevision string
	PreviewOnly      bool
	TargetSection    string
	TargetDirectory  string
}

// TemplateRef 标识已通过模板校验的不可变模板版本。
type TemplateRef struct {
	ID      string
	Version string
	Digest  string
	Target  string
}

// PreflightResult 保存不产生副作用的渠道检查结果。
type PreflightResult struct {
	Diagnostics []Diagnostic
	Ready       bool
}

// PreparedArtifact 是有明确内容版本和有效期的渠道产物。
type PreparedArtifact struct {
	OperationID      string
	ProviderRevision string
	ContentHash      string
	Location         string
	TargetPath       string
	// PreviousTargetPath 是命名规则升级时需要在新目标交付成功后清理的旧 Bundle 路径。
	PreviousTargetPath string `json:"previous_target_path,omitempty"`
	PreviewURL         string
	ExpiresAt          *time.Time
	TargetRelativePath string
	Change             string
	Files              []ArtifactFile
}

// ArtifactFile 描述已准备产物中的一个可审阅文件。
type ArtifactFile struct {
	RelativePath string
	MediaType    string
	SHA256       string
	Size         int64
}

// PublishSection 是 Publish Provider 暴露的受控一级目标目录。
type PublishSection struct {
	Name         string
	ArticleCount int
	Directories  []PublishDirectory
}

// PublishDirectory 描述 Section 下用于承载 Page Bundle 的分类目录。
type PublishDirectory struct {
	Path         string
	ArticleCount int
}

// SectionDiscovery 返回可选 Section 和已有文章的锁定目标。
type SectionDiscovery struct {
	Sections          []PublishSection
	ExistingSection   string
	ExistingDirectory string
	ExistingTarget    string
	SelectionLocked   bool
}

// SectionAwarePublishProvider 为需要受控目标目录的发布渠道提供发现能力。
type SectionAwarePublishProvider interface {
	PublishProvider
	DiscoverSections(ctx context.Context, sourceID string) (SectionDiscovery, error)
}

// PreparedArtifactValidator 在交付前验证持久化 Artifact 仍属于当前 Provider 实例且未被篡改。
type PreparedArtifactValidator interface {
	ValidatePreparedArtifact(ctx context.Context, artifact PreparedArtifact) error
}

// DeliveryResult 保存一次幂等渠道交付结果。
type DeliveryResult struct {
	State            string
	ProviderRevision string
	Location         string
	ConfirmRequired  bool
}
