package contracts

import (
	"context"
	"encoding/json"
)

// ProviderType 标识 Provider 的稳定编译期类型。
type ProviderType string

const (
	ProviderObsidian ProviderType = "obsidian"
	ProviderOpenAI   ProviderType = "openai-compatible"
	ProviderHugo     ProviderType = "hugo"
	ProviderWeChat   ProviderType = "wechat"
)

// ProviderRef 标识一个已配置的 Provider 实例。
type ProviderRef struct {
	ID   string
	Type ProviderType
}

// ErrorCategory 是 Provider 错误的稳定分类。
type ErrorCategory string

const (
	ErrorValidation           ErrorCategory = "validation"
	ErrorConflict             ErrorCategory = "conflict"
	ErrorNotFound             ErrorCategory = "not_found"
	ErrorUnauthorizedResource ErrorCategory = "unauthorized_resource"
	ErrorDependency           ErrorCategory = "dependency_unavailable"
	ErrorTemporary            ErrorCategory = "temporary"
	ErrorPermanent            ErrorCategory = "permanent"
	ErrorInternal             ErrorCategory = "internal"
)

// ProviderError 是 Transport 和任务系统可识别的脱敏错误。
type ProviderError struct {
	Code      string
	Category  ErrorCategory
	Message   string
	Retryable bool
	Field     string
	Cause     error
}

// Error 返回可安全展示的说明，不泄露底层响应或 Secret。
func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Unwrap 保留错误链，供 errors.Is 和 errors.As 使用。
func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Capability 描述 Provider 可被 Application 使用的固定能力。
type Capability string

const (
	CapabilityScan               Capability = "scan"
	CapabilityRead               Capability = "read"
	CapabilityWriteMetadata      Capability = "write_metadata"
	CapabilityWatch              Capability = "watch"
	CapabilityStructuredOutput   Capability = "structured_output"
	CapabilityPreview            Capability = "preview"
	CapabilityDraft              Capability = "draft"
	CapabilityDirectPublish      Capability = "direct_publish"
	CapabilityImages             Capability = "images"
	CapabilityTaxonomy           Capability = "taxonomy"
	CapabilityCanonical          Capability = "canonical"
	CapabilityManualConfirmation Capability = "manual_confirmation"
)

// Descriptor 描述 Provider 的身份、版本和能力，不包含 Secret。
type Descriptor struct {
	Type         ProviderType
	DisplayName  string
	Version      string
	ConfigSchema string
	Capabilities []Capability
	SecretKeys   []string
	SupportedOS  []string
}

// ConfigView 提供已解析、已脱敏的实例配置。
type ConfigView struct {
	SchemaVersion string
	Data          json.RawMessage
	AllowedRoots  []string
	SecretRefs    map[string]string
}

// ProviderRuntime 根据持久化实例构建已注册的类型化 Provider。
type ProviderRuntime interface {
	BuildSource(ctx context.Context, ref ProviderRef, config ConfigView) (SourceProvider, error)
	BuildAI(ctx context.Context, ref ProviderRef, config ConfigView) (AIProvider, error)
	BuildPublish(ctx context.Context, ref ProviderRef, config ConfigView) (PublishProvider, error)
	BuildTaxonomy(ctx context.Context, ref ProviderRef, config ConfigView) (TaxonomyProvider, error)
}

// SecretResolver 只按引用读取 Provider 已声明的 Secret。
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (SecretValue, error)
}

// SecretValue 只允许在单次 Provider 调用期间存在。
type SecretValue struct {
	Bytes []byte
}
