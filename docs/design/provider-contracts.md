# InkHub Provider 契约设计

> 对应 PRD 1.4、架构设计 1.1 和 `data-model.md`，面向 MVP Release 1。

## 1. 设计目标

Provider 是 InkHub 连接内容源、AI 服务和发布渠道的边界。Provider 负责外部系统适配，不负责数据库事务、审核状态、任务编排或用户确认。

所有 Provider 必须：

- 有稳定的 `Type`、实例 ID 和 Descriptor。
- 接收 `context.Context`，尊重取消和超时。
- 只访问实例配置授权的资源。
- 返回结构化错误，不退出进程，不打印 Secret。
- 不持有跨请求的可变全局状态。
- 可以被 Fake 替换，并通过契约测试。
- 不创建或持有 SQLite 事务。

## 2. 通用类型

```go
// ProviderType 标识 Provider 的稳定编译期类型。
type ProviderType string

const (
    ProviderObsidian ProviderType = "obsidian"
    ProviderOpenAI   ProviderType = "openai-compatible"
    ProviderHugo     ProviderType = "hugo"
    ProviderWeChat   ProviderType = "wechat"
)

// ProviderRef 标识一个已注册的 Provider 实例。
type ProviderRef struct {
    ID   string
    Type ProviderType
}

// ProviderError 是 Transport、Job Runner 和 Doctor 可识别的错误。
type ProviderError struct {
    Code      string
    Category  ErrorCategory
    Message   string
    Retryable bool
    Field     string
    Cause     error
}

// Error 返回可安全展示的错误说明，不展开底层 Cause。
func (e *ProviderError) Error() string { return e.Message }

// Unwrap 保留错误链，供 errors.Is 和 errors.As 判断。
func (e *ProviderError) Unwrap() error { return e.Cause }

type ErrorCategory string

const (
    ErrorValidation           ErrorCategory = "validation"
    ErrorConflict             ErrorCategory = "conflict"
    ErrorNotFound             ErrorCategory = "not_found"
    ErrorUnauthorizedResource ErrorCategory = "unauthorized_resource"
    ErrorDependency           ErrorCategory = "dependency_unavailable"
    ErrorTemporary            ErrorCategory = "temporary"
    ErrorPermanent            ErrorCategory = "permanent"
    ErrorInternal              ErrorCategory = "internal"
)
```

`Message` 是可展示给用户的简短说明；堆栈和外部响应只进入脱敏日志。`Code` 必须稳定，例如 `obsidian.frontmatter_invalid`、`hugo.build_failed`、`wechat.template_invalid`，不能直接使用第三方 HTTP 状态或命令输出。

## 3. Descriptor、配置和能力

```go
// Descriptor 描述 Provider 的身份、版本和能力，不包含 Secret。
type Descriptor struct {
    Type             ProviderType
    DisplayName      string
    Version          string
    ConfigSchema     string
    Capabilities     []Capability
    SecretKeys       []string
    SupportedOS      []string
}

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

// ConfigView 提供已解析、已脱敏的实例配置。
type ConfigView struct {
    SchemaVersion string
    Data          json.RawMessage
    AllowedRoots  []string
    SecretRefs    map[string]string
}

// SecretResolver 只按引用读取当前 Provider 已声明的 Secret。
type SecretResolver interface {
    Resolve(ctx context.Context, ref string) (SecretValue, error)
}

// SecretValue 只能在 Provider 调用期间存在，不可序列化或记录日志。
type SecretValue struct {
    Bytes []byte
}

// ConfigValidator 校验配置和授权路径，不执行长任务。
type ConfigValidator interface {
    ValidateConfig(ctx context.Context, config ConfigView) *ProviderError
}

```

Provider 配置分为三类：

- 可共享规则写入 `.inkhub/config.yaml`。
- 本机路径、开关和 UI 偏好写入 SQLite。
- API Key、Token 等只保存为 `SecretStore` 引用，Provider 通过受控接口读取，不接受明文 Secret 进入 `ConfigView`。

`ConfigView.Data` 必须先通过 Descriptor 指定的 JSON Schema，再解码为具体 Provider 的配置结构。`AllowedRoots` 必须是平台层完成绝对化、符号链接解析和授权校验后的路径。Descriptor 的能力必须使用固定枚举，Application 根据能力显示操作，不根据 Provider 类型堆叠业务分支。

## 4. Source Provider

### 4.1 接口

```go
// SourceProvider 负责发现、读取、监听和受控写回内容源。
type SourceProvider interface {
    // Descriptor 返回稳定的类型信息和能力声明。
    Descriptor() SourceDescriptor
    // Validate 检查实例配置、授权路径和必要依赖。
    Validate(ctx context.Context) error
    // Scan 分页发现内容源中的文章变化。
    Scan(ctx context.Context, cursor ScanCursor) (ScanResult, error)
    // Read 读取并解析一篇源文章。
    Read(ctx context.Context, ref SourceRef) (SourceDocument, error)
    // WriteMetadata 以乐观并发和原子替换方式写回标准元数据。
    WriteMetadata(ctx context.Context, cmd MetadataWriteCommand) (SourceDocument, error)
    // Watch 持续报告文件变化，直到 context 取消。
    Watch(ctx context.Context, changes chan<- SourceChange) error
}
```

```go
type SourceDescriptor struct {
    Descriptor
    Formats []string
}

type ScanCursor struct {
    Fingerprint string
    RelativePath string
}

type ScanResult struct {
    Documents []SourceDocumentRef
    Next       *ScanCursor
    Complete   bool
}

type SourceDocumentRef struct {
    Ref             SourceRef
    RelativePath    string
    StableID        string
    SourceFingerprint string
    Deleted         bool
}

type SourceRef struct {
    SourceID     string
    RelativePath string
    StableID     string
}

type SourceDocument struct {
    Ref             SourceRef
    Article         Article
    Body            string
    RawFrontmatter  string
    ResourceRefs    []ResourceRef
    Diagnostics     []Diagnostic
}

type ResourceRef struct {
    Original string
    Resolved string
    Kind     string
}

type MetadataWriteCommand struct {
    Ref              SourceRef
    ExpectedFingerprint string
    Patch            MetadataPatch
}

// MetadataPatch 使用指针区分“不修改”和“写入空值”。
type MetadataPatch struct {
    StableID   *string
    Title      *string
    Description *string
    Tags       *[]string
    Keywords   *[]string
    Category   *string
    Series     *string
    Slug       *string
    Cover      *string
}

type SourceChange struct {
    Kind   SourceChangeKind
    Ref    SourceRef
    Error  *ProviderError
}

type SourceChangeKind string

const (
    SourceCreated SourceChangeKind = "created"
    SourceModified SourceChangeKind = "modified"
    SourceMoved SourceChangeKind = "moved"
    SourceDeleted SourceChangeKind = "deleted"
    SourceRescanRequired SourceChangeKind = "rescan_required"
)
```

`WriteMetadata` 必须执行期望指纹检查、字段级写回、格式校验、临时文件写入和原子替换；不允许重排无关 frontmatter 或改写正文。`Watch` 只发送文件变化事件，扫描和状态更新由 Application 完成。

`SourceRef` 必须包含 `SourceID`，并至少包含 `StableID` 或 `RelativePath` 之一；两者同时存在但指向不同文件时返回 `conflict`。Scan cursor 只对同一 Source 实例和配置版本有效，配置变化后必须丢弃 cursor 并全量扫描。

### 4.2 Obsidian 实现边界

Obsidian Provider：

- 校验 Vault 根目录和 `.obsidian` 设置。
- 扫描 `.md` 文件并解析固定 frontmatter，包括 `keywords`。
- 解析 WikiLink、callout 和本地资源引用，但不生成渠道 HTML。
- 生成或受控写回稳定 ID及标准元数据。
- 监听新增、修改、移动和删除。

它不负责 AI、SEO 决策、Hugo frontmatter 或微信样式。

Obsidian 与普通 Markdown Folder 是 `SourceProvider` 的两个实现。二者组合相同的文件夹扫描、授权路径、监听和标准 Markdown 组件；Obsidian 实现只增加 Vault 配置与方言适配。MVP 只注册 Obsidian，Markdown Folder Provider 保留为后续实现，不通过 `mode` 参数混入 Obsidian Provider。

## 5. AI Provider

### 5.1 接口和结构化协议

```go
// AIProvider 执行结构化智能分析，不修改文章和审核状态。
type AIProvider interface {
    // Descriptor 返回稳定的类型信息和结构化输出能力。
    Descriptor() AIDescriptor
    // Validate 检查实例配置、Secret 和服务可用性。
    Validate(ctx context.Context) error
    // Generate 根据指定 schema 生成结构化建议。
    Generate(ctx context.Context, request AIRequest) (AIResponse, error)
}

type AIDescriptor struct {
    Descriptor
    Models          []string
    MaxInputBytes   int64
    OutputSchema    string
}

type AIRequest struct {
    Task              AITask
    Article           ArticleInput
    Taxonomy          TaxonomyContext
    OutputSchema      string
    InputContentHash  string
    AllowBody         bool
}

// ArticleInput 是按隐私策略裁剪后的 AI 输入，不包含文件路径和 Secret。
type ArticleInput struct {
    Title       string
    Description string
    Body        string
    Tags        []string
    Keywords    []string
    Category    string
    Series      string
    Slug        string
}

type AITask string

const (
    AITaskMetadata AITask = "metadata"
    AITaskSEO      AITask = "seo"
)

type AIResponse struct {
    InputContentHash string
    Model            string
    Suggestions      []Suggestion
    Raw              json.RawMessage
}

type Suggestion struct {
    Field       string
    Value       json.RawMessage
    Rationale   string
    Confidence  float64
    NewTerm     bool
}
```

`ArticleInput` 由 Application 按隐私设置构建；Provider 不得自行读取 Vault。返回结果必须先通过 JSON Schema 和 Domain 规则校验，再写入 `ai_suggestions`。`Raw` 只用于本次响应校验，完成后必须丢弃，不写数据库或日志。未知 tag 只能标记 `NewTerm`，不得直接写入文章或 taxonomy。`AllowBody=false` 时请求只能包含标题、元数据和明确允许的摘要片段。

### 5.2 OpenAI-compatible 实现

OpenAI-compatible Provider 只负责：构造兼容的 HTTP 请求、设置超时和大小限制、解析结构化响应、映射 HTTP/网络/限流错误。它不决定建议是否接受、不生成发布状态、不记录完整 prompt 或响应到日志。

## 6. Publish Provider

### 6.1 通用接口

```go
// PublishProvider 负责渠道预检、转换、预览和交付。
type PublishProvider interface {
    // Descriptor 返回稳定的渠道能力声明。
    Descriptor() PublishDescriptor
    // Validate 检查渠道配置、授权路径和外部依赖。
    Validate(ctx context.Context) error
    // Preflight 只检查输入和目标，不产生外部副作用。
    Preflight(ctx context.Context, input PublishInput) (PreflightResult, error)
    // Prepare 生成可预览、可交付且有期限的 artifact。
    Prepare(ctx context.Context, input PublishInput) (PreparedArtifact, error)
    // Deliver 将已准备 artifact 幂等交付到渠道目标。
    Deliver(ctx context.Context, artifact PreparedArtifact) (DeliveryResult, error)
}

type PublishDescriptor struct {
    Descriptor
}

type PublishInput struct {
    OperationID      string
    Article          Article
    Body             string
    ResourceRefs     []ResourceRef
    ContentHash      string
    TemplateRef      *TemplateRef
    ExpectedRevision string
    PreviewOnly      bool
}

// TemplateRef 标识已通过模板校验的模板版本。
type TemplateRef struct {
    ID      string
    Version string
    Digest  string
}

type PreflightResult struct {
    Checks       []CheckResult
    ChangeSummary ChangeSummary
    Target       TargetDescription
}

type CheckResult struct {
    Code     string
    Severity CheckSeverity
    Message  string
}

type CheckSeverity string

const (
    CheckBlocking    CheckSeverity = "blocking"
    CheckRecommended CheckSeverity = "recommended"
    CheckOptional    CheckSeverity = "optional"
    CheckPassed      CheckSeverity = "passed"
)

type ChangeSummary struct {
    Added    []string
    Updated  []string
    Removed  []string
}

type TargetDescription struct {
    Path string
    URL  string
}

type ArtifactFile struct {
    RelativePath string
    MediaType    string
    SHA256       string
    Size         int64
}

type PreparedArtifact struct {
    ArtifactID   string
    ContentHash  string
    ProviderType ProviderType
    OperationID  string
    ExpiresAt    time.Time
    PreviewURL   string
    Files        []ArtifactFile
    Metadata     map[string]string
}

type DeliveryResult struct {
    State           DeliveryState
    ProviderRevision string
    ConfirmRequired bool
    TargetURL       string
    Message         string
}

type DeliveryState string

const (
    DeliveryCopied    DeliveryState = "copied"
    DeliveryPublished DeliveryState = "published"
)
```

`OperationID` 由 Job ID、Provider instance ID 和 content hash 确定性生成，是 Prepare/Deliver 的幂等键。相同 OperationID 的重复调用必须返回同一结果或继续未完成步骤，不得制造重复 bundle、上传或渠道记录。

Provider 不更新 `publications`。Application 在 `Prepare`、`Deliver` 返回后，以实际使用的 `ContentHash` 和 Provider revision 写入 Publication/Event。`PreparedArtifact` 只能引用 Provider 自己的 job staging 目录，不能携带用户输入的任意绝对路径。Provider 必须在成功、失败、取消和启动恢复时提供幂等清理；超过 `ExpiresAt` 且没有运行中任务引用的 artifact 可以删除。

### 6.2 Hugo Publish Provider

输入为标准 Article、正文和资源引用；输出为 Hugo staging bundle、目标路径、预览地址和构建诊断。Provider 内部完成：

1. Hugo 根目录和 section 探测。
2. Obsidian Markdown 到 Hugo 内容的转换。
3. page bundle 和图片路径规划。
4. taxonomy 校验和 Hugo frontmatter 映射。
5. `Prepare` 在临时站点完成转换、taxonomy 校验和 Hugo build，只生成可预览 artifact。
6. `Deliver` 原子替换真实 bundle，并执行真实站点快速构建；失败时恢复原 bundle。

构建或替换任一步失败都返回结构化错误并保留原 bundle。重复同步以稳定 `source_id`/Article ID 定位同一 bundle，不创建重复页面。Provider 不执行 Git commit、push 或部署。

### 6.3 WeChat Publish Provider

输入为标准 Article、Markdown 正文、资源引用和已校验的 TemplateRef；输出为预览 HTML、复制所需 artifact 和图片处理结果。Provider 内部完成 Markdown 渲染、代码高亮、Mermaid 转换、图片上传、模板渲染、CSS 内联和 HTML 清理。图片上传按内容摘要生成目标键，相同 OperationID 重试不得重复上传；部分失败不生成可交付 artifact。

能力声明包含 `preview`、`images` 和 `manual_confirmation`，不包含 `direct_publish`。`Deliver` 只执行剪贴板交付并返回 `copied`；用户确认草稿由 Application 单独记录为 `confirmed`，复制成功不等于发布成功。

## 7. Provider Registry

```go
// ProviderRegistry 管理编译期注册的类型化 Provider 工厂。
type ProviderRegistry interface {
    RegisterSource(factory SourceProviderFactory) error
    RegisterAI(factory AIProviderFactory) error
    RegisterPublish(factory PublishProviderFactory) error
    Descriptor(providerType ProviderType) (Descriptor, error)
    BuildSource(ctx context.Context, ref ProviderRef, config ConfigView) (SourceProvider, error)
    BuildAI(ctx context.Context, ref ProviderRef, config ConfigView) (AIProvider, error)
    BuildPublish(ctx context.Context, ref ProviderRef, config ConfigView) (PublishProvider, error)
}

type SourceProviderFactory interface {
    Type() ProviderType
    Descriptor() SourceDescriptor
    Build(ctx context.Context, ref ProviderRef, config ConfigView, secrets SecretResolver) (SourceProvider, error)
}

type AIProviderFactory interface {
    Type() ProviderType
    Descriptor() AIDescriptor
    Build(ctx context.Context, ref ProviderRef, config ConfigView, secrets SecretResolver) (AIProvider, error)
}

type PublishProviderFactory interface {
    Type() ProviderType
    Descriptor() PublishDescriptor
    Build(ctx context.Context, ref ProviderRef, config ConfigView, secrets SecretResolver) (PublishProvider, error)
}
```

MVP 编译期注册四个实现：Obsidian、OpenAI-compatible、Hugo 和 WeChat。不实现动态插件、运行时代码下载或 Provider 之间互相调用。Registry 只负责类型、工厂和能力发现，不负责业务状态。

## 8. 错误、取消和安全

- 配置错误返回 `validation`，不创建后台任务。
- 路径越权返回 `unauthorized_resource`，不尝试修正路径。
- 外部服务超时、限流和临时不可用返回 `temporary` 并允许 Job Runner 重试。
- 解析失败、模板不兼容、Hugo 构建失败按具体步骤返回 `permanent` 或 `validation`。
- `context.Canceled` 和 `context.DeadlineExceeded` 保留可识别原因，不当作成功。
- 超时由 Application 按操作类型设置；Provider 不移除或延长调用方 deadline。
- Provider 日志只能包含 workspace、article、job、instance ID、稳定错误码和耗时。
- HTTP 请求设置连接/读取超时、响应大小上限和受控重定向；外部命令使用参数数组和授权工作目录。
- `ProviderError.Cause` 不经过 Transport 序列化；`Message`、`Field` 和 `Code` 必须经过敏感信息检查。

## 9. 契约测试

每个实现必须通过对应接口的共享测试套件：

- Descriptor 稳定、能力声明与实际行为一致。
- `Validate` 能发现缺失配置、无权限路径和不可用依赖。
- 取消 context 后不继续写文件、不提交交付结果。
- 错误包含稳定 code、分类和 Retryable，且不包含 Secret 或正文。
- 重复调用相同输入不会产生额外副作用，或明确返回冲突。
- Provider 不创建数据库事务、不访问未授权路径。

额外测试：

- Obsidian：frontmatter、`keywords`、WikiLink、callout、中文路径、重复 ID、原子写回。
- OpenAI-compatible：结构化响应、无效 JSON、超时、限流、大小限制和隐私裁剪。
- Hugo：重复 bundle、taxonomy/build 失败、staging 恢复和预览。
- WeChat：Default/Minimal 模板、CSS 允许列表、无脚本输出、图片/Mermaid、复制与确认分离。
