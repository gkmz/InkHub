package contracts

import (
	"context"
	"encoding/json"
)

// AIProvider 执行结构化智能分析，不修改文章和审核状态。
type AIProvider interface {
	Descriptor() AIDescriptor
	Validate(ctx context.Context) error
	Generate(ctx context.Context, request AIRequest) (AIResponse, error)
}

// AIDescriptor 描述 AI Provider 的模型和输入输出限制。
type AIDescriptor struct {
	Descriptor
	Models        []string
	MaxInputBytes int64
	OutputSchema  string
}

// AITask 标识 AI 分析任务类型。
type AITask string

const (
	AITaskMetadata    AITask = "metadata"
	AITaskSEO         AITask = "seo"
	AITaskXiaohongshu AITask = "xiaohongshu"
)

// AIRequest 是经过 Application 隐私裁剪后的结构化请求。
type AIRequest struct {
	Task             AITask
	Article          ArticleInput
	Taxonomy         TaxonomyContext
	OutputSchema     string
	InputContentHash string
	AllowBody        bool
}

// ArticleInput 是允许发送给 AI 的文章字段，不包含路径和 Secret。
type ArticleInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Body        string   `json:"body,omitempty"`
	Tags        []string `json:"tags"`
	Keywords    []string `json:"keywords"`
	Category    string   `json:"category"`
	Series      string   `json:"series"`
	Slug        string   `json:"slug"`
}

// TaxonomyContext 提供 AI 排序所需的现有分类候选。
type TaxonomyContext struct {
	Categories []string `json:"categories"`
	Series     []string `json:"series"`
	Tags       []string `json:"tags"`
}

// AIResponse 保存经过 Provider 校验的结构化建议。
type AIResponse struct {
	InputContentHash string
	Model            string
	Suggestions      []Suggestion
}

// Suggestion 描述一个可单独采纳的字段建议。
type Suggestion struct {
	Field      string
	Value      json.RawMessage
	Rationale  string
	Confidence float64
	NewTerm    bool
}

// AIProviderFactory 构建一个类型安全的 AI Provider 实例。
type AIProviderFactory interface {
	Type() ProviderType
	Descriptor() AIDescriptor
	Build(ctx context.Context, ref ProviderRef, config ConfigView, secrets SecretResolver) (AIProvider, error)
}
