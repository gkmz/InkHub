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
	OperationID      string
	Article          article.Article
	Body             string
	ResourceRefs     []ResourceRef
	ContentHash      string
	TemplateRef      *TemplateRef
	ExpectedRevision string
	PreviewOnly      bool
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
	PreviewURL       string
	ExpiresAt        *time.Time
}

// DeliveryResult 保存一次幂等渠道交付结果。
type DeliveryResult struct {
	State            string
	ProviderRevision string
	Location         string
	ConfirmRequired  bool
}
