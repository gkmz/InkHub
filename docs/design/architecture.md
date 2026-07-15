# InkHub 技术架构设计

> 本文档定义 InkHub MVP Release 1 的总体架构、模块边界、运行机制和现有代码迁移策略。

## 1. 文档信息

| 项目 | 内容 |
| --- | --- |
| 文档版本 | 1.2 |
| 对应 PRD | `docs/PRD.md` 1.5 |
| 目标版本 | InkHub MVP Release 1 |
| 目标读者 | 核心开发者、架构维护者、Provider 贡献者 |
| 技术栈 | Go、SQLite、本地 Web UI、嵌入式静态资源 |

### 1.1 目的与范围

本文档用于指导开发者完成以下工作：

- 建立 InkHub 新代码骨架和依赖方向。
- 实现 Core、Repository、后台任务和 Provider。
- 将现有微信渲染、图片和 Mermaid 能力迁入新架构。
- 保证 Obsidian、Hugo、AI 和微信流程可测试、可恢复、可扩展。

本文档不定义完整数据库字段、HTTP API 清单、微信模板 JSON Schema 和视觉稿。它们由后续专项设计负责。

## 2. 架构目标与约束

### 2.1 业务闭环

```text
Obsidian 写作
  → 扫描与审核
  → AI 元数据和 SEO 建议
  → Taxonomy 校验
  → Hugo 同步与预览
  → 微信模板渲染与复制
  → 渠道状态和版本追踪
```

### 2.2 质量目标

1. **简单部署**：单个 Go 二进制启动，前端资源和 migration 内嵌。
2. **本地优先**：正文不进入数据库，服务默认只监听本机。
3. **职责清晰**：Core 不包含具体内容源、模型和渠道逻辑。
4. **失败可恢复**：文件写入和 Hugo 同步失败后保留原内容。
5. **幂等**：同一文章重复同步更新同一 Hugo bundle。
6. **可测试**：Domain 不依赖 Gin、SQLite、真实文件系统或远程服务。
7. **可扩展**：新增 Provider 不修改无关 Provider 和核心状态机。
8. **体验克制**：数据库、hash、任务队列等实现概念不直接暴露给用户。

### 2.3 MVP 约束

- 内容源只支持 Obsidian Vault。
- 每个工作区只允许一个 Obsidian、一个 Hugo 和一个微信 Provider 实例。
- AI 采用 OpenAI-compatible API。
- Hugo CLI 是站点构建的权威实现。
- 微信以格式化复制和人工草稿确认为终点。
- Hugo 标准配置、文章 frontmatter 和 taxonomy term 页面是 taxonomy 权威来源；SQLite 保存持久化投影。
- Provider 编译进主程序，不实现动态插件。
- 微信模板是无执行代码的标准资源包。

## 3. 核心架构决策

### 3.1 模块化单体

InkHub 采用 Go 模块化单体，不采用微服务。

原因：

- 产品运行在本机，不需要独立扩缩容。
- 文件系统、SQLite 和本地 UI 共享生命周期和一致性边界。
- 单进程更容易实现单文件分发、文章级互斥和优雅关闭。
- 清晰的包边界足以支持 Provider 扩展。

MVP 禁止引入消息中间件、服务发现、分布式锁、内部远程 RPC 和通用工作流引擎。

### 3.2 分层依赖

```text
Transport
    ↓
Application
    ↓
Domain
    ↑
Infrastructure / Provider
```

- `Transport` 处理 HTTP、CLI 和序列化。
- `Application` 编排用例、事务、任务和补偿。
- `Domain` 定义文章、审核、taxonomy、发布和模板规则。
- `Infrastructure` 实现 SQLite、文件、进程、Secret 和操作系统能力。
- `Provider` 适配 Obsidian、AI、Hugo 和微信。

依赖只能指向内层。Domain 禁止导入 Gin、SQLite 驱动、Goldmark、Hugo 命令和具体 Provider。

### 3.3 SQLite 与内容源

SQLite 保存工作区、文章索引、审核状态、Provider 实例、发布记录、AI 建议、模板安装和任务。

SQLite 不保存 Markdown 正文。Obsidian Vault 始终是正文唯一来源，Hugo 内容是发布副本。

### 3.4 编译期 Provider

MVP 内置：

- Obsidian Source Provider。
- OpenAI-compatible AI Provider。
- Hugo Publish Provider。
- WeChat Publish Provider。

Provider 注册表只管理类型、工厂和能力。业务状态和调用时机由 Application 决定。

### 3.5 本地 Web UI

InkHub 运行本地 HTTP Server，前端资源嵌入二进制。Transport API 与页面实现解耦；未来是否使用 React 不影响 Application 和 Domain。

### 3.6 外部工具边界

- Hugo 构建调用官方 `hugo` 命令。
- Mermaid 通过内部抽象调用嵌入实现或受控外部进程。
- Hugo 内容转换、资源复制、路径重写和 taxonomy 校验由 InkHub 内部实现。
- 所有进程调用使用参数数组和受控工作目录，不拼接 shell 字符串。

## 4. 系统上下文

```text
┌──────────────────┐
│  Obsidian Vault  │
│ Markdown + Assets│
└────────┬─────────┘
         │ read / controlled write
         ▼
┌──────────────────────────────────────────────┐
│                   InkHub                    │
│ Core · SQLite · Providers · Local Web UI    │
└──────┬──────────┬──────────┬──────────┬─────┘
       │          │          │          │
       ▼          ▼          ▼          ▼
    AI API    Hugo Project  GitHub    Template
                           Image Repo Repository
                  │
                  ▼
             Hugo Preview

InkHub → Clipboard → 微信公众号后台（人工粘贴与确认）
```

| 外部系统 | 职责 | InkHub 边界 |
| --- | --- | --- |
| Obsidian Vault | 正文和源资源 | 读取 Markdown，受控写回标准 frontmatter |
| AI API | 结构化建议 | 不写文件，不管理审核状态 |
| Hugo Project | 博客与 taxonomy | 原子同步 bundle，调用 Hugo 构建 |
| GitHub Image Repo | 微信图片托管 | 上传用户确认的文章资源 |
| Template Repository | 模板索引和模板包 | 下载、校验、安装，不执行代码 |
| 微信公众号后台 | 草稿和发布 | InkHub 复制 HTML，用户确认结果 |

## 5. 分层职责

### 5.1 Transport

负责：

- 解析 HTTP 和 CLI 输入。
- 执行协议层格式校验。
- 调用 Application Use Case。
- 映射错误为页面或 JSON 响应。
- 提供任务进度、预览和文件下载。

禁止直接操作 SQLite、文章文件和 Provider，也禁止在 Handler 中实现审核或发布规则。

### 5.2 Application

负责：

- 编排完整用户用例。
- 控制短数据库事务。
- 获取文章和 Provider 互斥锁。
- 创建后台任务。
- 调用 Domain Service 和 Port。
- 记录发布事件和失败补偿。

主要用例：

- `InitializeWorkspace`
- `ScanWorkspace`
- `ReviewArticle`
- `GenerateMetadataSuggestions`
- `AcceptMetadataSuggestion`
- `ApproveNewTag`
- `PublishToHugo`
- `PrepareForWeChat`
- `ConfirmWeChatDraft`
- `InstallWeChatTemplate`

### 5.3 Domain

负责：

- 标准文章模型和 content hash。
- 审核状态机。
- Taxonomy 与 tag 准入规则。
- SEO 检查结果。
- 发布状态和过期判定。
- Provider 能力描述。
- 模板 manifest 和版本兼容规则。
- 任务状态和错误分类。

Domain 只使用 Go 标准类型和自身 Value Object。

### 5.4 Infrastructure

负责：

- SQLite Repository 和 migration。
- 原子文件读写和文件监听。
- 外部进程和 HTTP Client。
- Keychain、环境变量和系统目录。
- 剪贴板、浏览器打开和日志。

### 5.5 Provider

Provider 适配特定内容源、AI 服务或发布渠道。Provider 可以使用 Infrastructure，但不能：

- 绕过 Application 修改状态。
- 依赖 HTTP Handler。
- 访问配置未授权的目录。
- 将渠道字段写入标准文章模型。

## 6. 核心领域模块

### 6.1 Workspace

- 表示一个 Vault 及其 AI、Hugo、微信配置。
- 管理最近使用时间和启用能力。
- 校验路径和 MVP 单实例约束。
- 保存默认微信模板。

### 6.2 Article

- 表示标准元数据和源文件定位。
- 校验稳定 ID、标题、description、tags 和 publish 字段。
- 计算规范化 content hash。
- 识别文件移动、删除和重复 ID。

Article 不包含 Hugo bundle、微信 HTML 或 AI 原始响应。

### 6.3 Editorial

- 管理草稿、待补充、待审核、审核通过和内容变化状态。
- 汇总元数据、Markdown、图片和 SEO 检查。
- 判定是否允许调用目标 Publish Provider。
- 在影响输出的内容变化后使审核失效。

### 6.4 Taxonomy

- 通过 Taxonomy Provider 解析发布平台的标准 taxonomy 资源。
- Hugo 实现读取站点配置、文章 frontmatter 和 taxonomy term 页面。
- 管理 category、series、aliases、核心 tag 和低频豁免。
- 规范化和统计 tags。
- 生成受控 YAML 变更。

SQLite 中的 taxonomy 数据是可重建的持久化投影，用于启动展示、同步状态和使用统计；发现失败时保留最近成功快照。

### 6.5 Content Checker

- 执行通用 Markdown 和 SEO 检查。
- 输出 `blocking`、`recommended`、`optional`、`passed`。
- 允许 Publish Provider 增加渠道检查器。
- 只返回发现，不直接修改文章。

### 6.6 Publication

- 管理文章到 Provider 实例的处理记录。
- 保存使用的 content hash。
- 判定渠道是否 outdated。
- 记录 prepared、copied、confirmed、published、failed 事件。

状态通过事件和最新记录计算，不再使用单一 `published` 布尔值。

### 6.7 Template

- 解析和校验微信模板 manifest。
- 校验标准版本、模板版本和兼容范围。
- 校验 CSS 允许列表、资源清单和 SHA-256。
- 安装、更新、回滚和选择模板。
- 将模板变量转换为受控 CSS 值。

### 6.8 Job

- 表示扫描、AI、taxonomy、Hugo 和微信任务。
- 管理 queued、running、succeeded、failed、cancelled。
- 保存有限进度、重试次数和错误摘要。
- 重启后恢复可重试任务。

### 6.9 Secret

- 使用逻辑 Secret Key 引用敏感信息。
- 从 Keychain 或环境变量读取实际值。
- 禁止 Secret 进入 SQLite、日志、API 和诊断包。

## 7. Provider 契约

### 7.1 通用要求

所有 Provider 必须：

- 有稳定 Type、实例 ID 和 Descriptor。
- 声明能力并提供配置校验。
- 接收 `context.Context`。
- 返回结构化错误，不退出进程。
- 不持有全局可变状态。
- 支持 Fake 和契约测试。
- 只访问配置授权资源。

Provider 不创建数据库事务。

### 7.2 Source Provider

```go
// SourceProvider 负责读取和受控写回内容源。
type SourceProvider interface {
	Descriptor() SourceDescriptor
	Validate(ctx context.Context) error
	Scan(ctx context.Context, cursor ScanCursor) (ScanResult, error)
	Read(ctx context.Context, ref SourceRef) (SourceDocument, error)
	WriteMetadata(ctx context.Context, cmd MetadataWriteCommand) (SourceDocument, error)
	Watch(ctx context.Context, changes chan<- SourceChange) error
}
```

Obsidian 实现负责：

- 校验 `.obsidian` 和附件设置。
- 扫描 Markdown 和解析固定 frontmatter。
- 解析 WikiLink、callout 和本地资源引用。
- 原子写回标准 frontmatter。
- 监听文件变化。

它不负责 HTML、AI 和 Hugo 转换。

普通 Markdown 文件夹与 Obsidian Vault 使用同一 `SourceProvider` 契约，但作为不同实现注册。通用的路径授权、文件遍历、fingerprint、监听和标准 Markdown/frontmatter 能力下沉到共享组件；Obsidian Provider 通过组合共享组件增加 `.obsidian` 配置、WikiLink、嵌入语法和 callout 适配，不在单个 Provider 中使用 mode 分支。

### 7.3 AI Provider

```go
// AIProvider 执行结构化智能分析任务。
type AIProvider interface {
	Descriptor() AIDescriptor
	Validate(ctx context.Context) error
	Generate(ctx context.Context, request AIRequest) (AIResponse, error)
}
```

`AIRequest` 包含任务类型、输入范围、taxonomy 候选和输出 schema 版本。返回结果先做结构校验，再进入 Domain 规则校验。

OpenAI-compatible 实现只负责请求、响应和错误映射，不决定建议是否采用，也不写文章。

### 7.4 Publish Provider

```go
// PublishProvider 将标准文章准备或交付到目标渠道。
type PublishProvider interface {
	Descriptor() PublishDescriptor
	Validate(ctx context.Context) error
	Preflight(ctx context.Context, input PublishInput) (PreflightResult, error)
	Prepare(ctx context.Context, input PublishInput) (PreparedArtifact, error)
	Deliver(ctx context.Context, artifact PreparedArtifact) (DeliveryResult, error)
}
```

Descriptor 可以声明：

- `preview`
- `draft`
- `direct_publish`
- `images`
- `taxonomy`
- `canonical`
- `manual_confirmation`

Application 根据能力显示动作，不通过 Provider 类型字符串堆积分支。

### 7.5 Hugo Provider 组件

- Hugo 配置探测器。
- Frontmatter 映射器。
- Obsidian-to-Hugo 转换器。
- Page Bundle 规划器。
- 资源复制器。
- Taxonomy 校验器。
- 临时站点构建器。
- Hugo Process Runner。
- 原子替换与恢复器。

### 7.6 WeChat Provider 组件

- Markdown HTML 渲染器。
- Mermaid 转换器。
- 图片收集和上传器。
- 微信模板渲染器。
- CSS 内联器和 HTML 清理器。
- 预览 Artifact。
- 剪贴板交付器。

Default、Minimal 和第三方模板必须走同一加载、校验和渲染路径。

## 8. 关键用例数据流

### 8.1 初始化工作区

```text
Validate Vault
  → Detect attachments
  → Detect Hugo and taxonomy
  → Validate optional AI and WeChat config
  → Create workspace transaction
  → Register Provider instances
  → Enqueue initial scan
  → Save recent workspace
```

可选 Provider 失败不阻止工作区创建，但对应能力标记为未配置。

### 8.2 增量扫描

```text
Receive file event
  → Source Provider reads file
  → Parse metadata
  → Compute content hash
  → Resolve stable ID
  → Upsert article index
  → Recompute editorial freshness
  → Mark publications outdated
  → Refresh taxonomy statistics
```

重复 ID 不覆盖文章，冲突文章阻止审核和发布。

### 8.3 AI 建议

```text
Load article and taxonomy
  → Build privacy-filtered request
  → Call AI Provider
  → Validate response schema
  → Apply domain rules
  → Save suggestion with content hash
  → Return field-level diff
```

接受建议时再次校验 content hash，防止旧建议覆盖新文章。

### 8.4 新 Tag 准入

```text
Build taxonomy change
  → Show provider-native diff
  → Validate expected revision
  → Apply through Taxonomy Provider
  → Rediscover authoritative taxonomy
  → Write article frontmatter
  → Persist refreshed snapshot
```

Taxonomy 和文章无法组成文件系统事务。先写权威 taxonomy；文章写入失败时保留新词并允许重试。

### 8.5 Hugo 同步

```text
Acquire article/provider lock
  → Verify approval and content hash
  → Run preflight
  → Build bundle in staging
  → Build temporary Hugo site
  → Backup current bundle
  → Atomically replace bundle
  → Verify real site build
  → Restore backup on failure
  → Record result and event
```

临时工作区与目标项目尽量位于同一文件系统。具体 staging 算法由 Hugo Provider 设计定义。

### 8.6 微信准备与确认

```text
Acquire lock
  → Verify content hash
  → Resolve template and variables
  → Render Markdown and Mermaid
  → Confirm image list
  → Upload missing images
  → Inline and sanitize styles
  → Save prepared artifact metadata
  → Preview
  → Copy on user action
  → Confirm draft on user action
```

准备、复制和确认是三个独立事件，均记录同一 content hash。

## 9. 运行时架构

### 9.1 组件

```text
InkHub Process
├── CLI Bootstrap
├── Explicit Dependency Wiring
├── HTTP Server
├── Application Services
├── Job Runner
├── File Watch Manager
├── Provider Registry
├── SQLite Connection Pool
└── Shutdown Coordinator
```

依赖使用显式 Go 构造函数装配，不引入重型 DI 框架。

### 9.2 启动顺序

1. 解析最小 CLI 参数。
2. 确定数据目录。
3. 初始化脱敏日志。
4. 获取单实例锁。
5. 打开 SQLite。
6. 备份并执行 migration。
7. 加载工作区和 Provider 配置。
8. 注册 Provider 工厂。
9. 恢复未完成任务。
10. 启动 Job Runner 和文件监听。
11. 启动仅监听本机的 HTTP Server。
12. 按配置打开浏览器。

Migration 失败时进入只读恢复提示，不启动写任务。

### 9.3 单实例

同一数据目录只允许一个写实例。第二个进程发现健康服务时打开现有 UI 并退出；过期锁只能在安全检查后回收。

### 9.4 Job Runner

- 使用 SQLite 持久化任务和少量本地 worker。
- 同一文章与同一 Provider 的发布任务串行。
- 连续扫描事件可以合并。
- AI 和网络任务支持超时和有限重试。
- 文件替换不盲目自动重试。
- 详细日志写轮换文件，通过 job ID 关联。

### 9.5 文件监听

- 监听 Vault Markdown、附件和 taxonomy。
- 忽略 InkHub 临时文件。
- 合并短时间重复事件。
- 监听失败时回退到定时增量扫描。
- 应用写回后重新读取文件作为最终事实。

### 9.6 优雅关闭

1. 停止接受新写请求。
2. 取消可取消的 AI、扫描和网络任务。
3. 等待原子文件操作完成。
4. 停止监听并持久化任务状态。
5. 关闭 HTTP 和数据库。
6. 释放单实例锁。

## 10. 持久化边界

### 10.1 Repository

Application 使用领域 Repository：

- `WorkspaceRepository`
- `ArticleRepository`
- `EditorialRepository`
- `TaxonomyCacheRepository`
- `ProviderInstanceRepository`
- `PublicationRepository`
- `SuggestionRepository`
- `TemplateRepository`
- `JobRepository`

Repository 不向上暴露 SQL row 或 `map[string]any`。

### 10.2 事务

- 用例明确声明事务边界。
- 网络调用和 Hugo 构建不占用数据库写事务。
- 外部操作前保存任务意图，完成后短事务保存结果。
- Publication 和 Event 在同一事务保存。
- 文件与 SQLite 使用 staging、幂等和补偿保证最终一致。

### 10.3 数据分类

可重建：文章索引、taxonomy 缓存、tag 统计。

不可完全重建：审核记录、AI 采纳、发布历史、微信确认、工作区和 Provider 配置。备份优先保护后者。

## 11. 配置与 Secret

| 来源 | 内容 |
| --- | --- |
| CLI | host、port、workspace、data-dir 等本次覆盖 |
| 环境变量 | Secret 和自动化覆盖 |
| `.inkhub/config.yaml` | 可共享规则和排除规则 |
| SQLite | 本机路径、最近工作区、UI 偏好和实例配置 |
| 默认值 | 安全默认行为 |

优先级：CLI > 环境变量 > 工作区配置 > SQLite > 默认值。

```go
// SecretStore 安全保存和读取敏感配置。
type SecretStore interface {
	Get(ctx context.Context, key SecretKey) (SecretValue, error)
	Set(ctx context.Context, key SecretKey, value SecretValue) error
	Delete(ctx context.Context, key SecretKey) error
}
```

优先使用系统 Keychain；不可用时使用环境变量，不使用明文 SQLite 兜底。

可以热更新 taxonomy、微信模板和 UI 偏好。Vault、Hugo、AI 和图片仓库变化需要重建 Provider。数据目录、host 和 port 需要重启。

## 12. 文件系统架构

macOS 默认目录：

```text
~/Library/Application Support/InkHub/
├── inkhub.db
├── backups/
├── logs/
├── templates/
├── jobs/
├── staging/
└── locks/
```

### 12.1 路径授权

- Obsidian Provider：Vault 根目录。
- Hugo Provider：Hugo 根目录和 staging。
- WeChat Provider：Vault 资源和模板目录。

所有路径先规范化、解析符号链接，并验证位于允许根目录内。

### 12.2 原子写入

1. 在目标同目录创建临时内容。
2. 写入并关闭。
3. 校验格式和必要字段。
4. 保留权限。
5. 原子 rename。
6. 失败时删除临时内容并保留原文件。

### 12.3 Hugo Staging

- 创建反映真实站点的临时工作区。
- 将新 bundle 写入临时站点。
- 执行 taxonomy 校验和 Hugo build。
- 成功后备份并替换真实 bundle。
- 在真实站点快速构建确认。
- 失败时恢复备份。
- staging 使用 job ID，异常退出后可恢复和清理。

## 13. HTTP 与 CLI

### 13.1 HTTP

- 长任务返回 job ID，不保持请求直到完成。
- 错误包含稳定 code、用户消息和字段问题。
- 除设置和诊断外，不返回绝对路径。
- 列表支持分页、筛选和稳定排序。
- Transport 只调用 Application。

页面主入口只有工作台、内容库、标签治理和设置。文章详情、任务、模板和历史作为子路由。

### 13.2 CLI

```text
inkhub
inkhub init
inkhub doctor
inkhub scan
inkhub db backup
inkhub template init
inkhub template validate
```

CLI 与 Web UI 调用相同 Use Case，不复制业务逻辑。

## 14. 错误与日志

### 14.1 错误分类

- `validation`
- `conflict`
- `not_found`
- `unauthorized_resource`
- `dependency_unavailable`
- `temporary`
- `permanent`
- `internal`

用户错误说明发生了什么、失败步骤、能否重试和下一步。内部日志包含 code、job、article、workspace、instance ID、错误链和耗时。

日志禁止记录正文、Secret、完整 AI 请求和剪贴板 HTML。

### 14.2 诊断包

可以包含版本、Provider 状态、脱敏配置、schema 版本、失败任务日志和路径可用性，不包含用户正文和 Secret。

## 15. 安全架构

- 默认监听 `127.0.0.1`，禁用跨域，写接口校验同源。
- 文件访问执行根目录约束。
- 不用通用静态路由暴露 Vault。
- 模板解包防止 Zip Slip。
- 模板禁止 JavaScript、可执行文件、`@import` 和未声明资源。
- 模板校验 manifest、允许属性、文件清单和 SHA-256。
- AI Provider 不得扩大用户允许的正文发送范围。
- 外部 HTTP 设置超时、大小限制和重定向策略。

## 16. 分发与跨平台

二进制嵌入：

- Web 资源。
- SQLite migration。
- Default 和 Minimal 微信模板。
- 标准预览 Markdown。
- Schema 和默认配置。

macOS 是首要验收平台；Linux 支持本地服务和 CLI；Windows 在 Release 1 前完成基础验证。

平台差异封装为 `SystemPaths`、`DirectoryPicker`、`SecretStore`、`Clipboard`、`BrowserOpener`、`FileIdentity` 和 `ProcessRunner`。

`inkhub doctor` 检查 Hugo、路径、SQLite、AI、GitHub 图片、模板、端口和权限。未配置 AI 或 Hugo 不阻止启动。

## 17. 测试架构

### 17.1 层次

1. Domain 单元测试。
2. Application 使用 Fake Repository 和 Fake Provider。
3. Infrastructure 使用临时 SQLite 和临时目录。
4. Provider 契约测试和黄金文件。
5. 少量端到端关键流程。

### 17.2 必测内容

Domain：

- 标准文章、content hash、审核状态。
- 发布过期判定。
- Tag alias、数量和准入。
- 模板版本兼容和错误分类。

Obsidian 黄金文件：

- Frontmatter、WikiLink、callout、图片、中文路径、重复 ID。
- 写回元数据后正文和无关字段不变。

Hugo 集成：

- 创建和重复同步 bundle。
- 图片复制和重名。
- Taxonomy 和构建失败。
- 替换失败恢复。

微信模板：

- Default 和 Minimal 同链路渲染。
- 标准元素样式、CSS 内联和属性拒绝。
- 模板变量边界和更新回滚。
- 输出不含脚本和禁止资源。

端到端：

1. 创建工作区并扫描 Vault。
2. AI 建议与逐项接受。
3. 审核文章。
4. 同步 Hugo 并预览。
5. 切换模板并复制微信内容。
6. 修改文章后渠道过期。

## 18. 推荐代码结构

```text
InkHub/
├── cmd/inkhub/main.go
├── internal/
│   ├── app/
│   │   ├── bootstrap/
│   │   ├── workspace/
│   │   ├── editorial/
│   │   ├── taxonomy/
│   │   ├── publication/
│   │   └── template/
│   ├── domain/
│   │   ├── article/
│   │   ├── editorial/
│   │   ├── taxonomy/
│   │   ├── publication/
│   │   ├── template/
│   │   └── job/
│   ├── provider/
│   │   ├── registry/
│   │   ├── source/obsidian/
│   │   ├── ai/openai/
│   │   └── publish/{hugo,wechat}/
│   ├── content/{markdown,assets,mermaid}/
│   ├── storage/sqlite/{migrations,repository}/
│   ├── platform/{filesystem,process,secrets,clipboard,browser}/
│   └── transport/{http,cli}/
├── web/{static,templates}/
├── templates/wechat/{inkhub-default,inkhub-minimal}/
├── docs/{PRD.md,design}/
├── testdata/{obsidian,hugo,templates}/
├── go.mod
└── README.md
```

### 18.1 包规则

- 一个包只表达一个职责。
- 不建立 `utils`、`helpers`、`common` 大杂烩。
- Domain 不导入外层。
- Provider 不互相导入。
- Transport 只调用 Application。
- SQL 只存在于 SQLite Infrastructure。
- 文件写入集中经过受控 Platform API。
- 公开方法有明确中文文档注释。
- 关键业务和补偿流程有简短中文注释。

## 19. 现有代码迁移

### 19.1 策略

采用新骨架旁路迁移，不在旧 Handler、Service 和 JSON 状态上继续增加 InkHub 功能。

1. 建立新 `cmd/inkhub` 和 `internal`。
2. 旧入口暂时保持可构建。
3. 为待迁移行为补特征测试或黄金文件。
4. 迁移到新模块。
5. 新入口验收后删除对应旧入口。
6. 最后统一 module、README 和 CI。

不创建长期 `legacy` 副本，Git 历史保存旧实现。

### 19.2 迁移表

| 现有模块 | 处理 | 新归属 |
| --- | --- | --- |
| `old/markdown/processor.go` | 拆分行为并补测试 | `internal/content/markdown` |
| `old/markdown/chroma_style.go` | 迁移 | Markdown 渲染与代码主题 |
| `old/services/mermaid.go` | 抽象依赖后迁移 | `internal/content/mermaid` |
| `old/services/uploader.go` | 改为接口和 GitHub 实现 | WeChat asset uploader |
| `old/services/publisher.go` | 提取图片解析行为，删除整体结构 | WeChat Provider |
| `old/web/static/css/wechat.css` | 改为标准模板 | `inkhub-default` |
| 微信复制 JavaScript | 提取交互 | 新发布页 |
| `old/scanner/scanner.go` | 重写 | Obsidian Source Provider |
| `old/models/article.go` | 替换 | Domain Article |
| `old/models/status.go` | 替换 | Editorial / Publication |
| `old/services/status.go` | 删除 JSON 实现 | SQLite Repository |
| `old/services/platform.go` | 删除平台列表 | Provider Registry |
| `old/config/config.go` | 重写 | Bootstrap Config + SecretStore |
| `old/handlers/*` | 重写为薄 Transport | `internal/transport/http` |
| `old/server/server.go` | 重写装配，保留 Gin | HTTP Transport |
| `old/main.go` | 替换 | `cmd/inkhub/main.go` |
| `old/config/*.json` | 删除 | SQLite 和实例配置 |
| `old/REFACTORING.md` | 删除或归档 | 本文档替代 |

### 19.3 保留与放弃

保留经过验证的行为：微信样式、代码高亮、WikiLink、callout、图片上传、Mermaid 和复制流程。

不保留旧包结构、路径 ID、JSON 状态、平台列表、旧 API 和 `markdown-preview` 命名。

### 19.4 迁移纪律

- 每个功能点结束时项目可构建、测试通过。
- 新旧实现不同时写同一状态。
- 新入口接管前不删除旧能力。
- 接管后删除旧入口，避免双轨维护。
- 不混入无关格式化和大范围重命名。

## 20. 演进边界

Release 1 之后可以增加新 Source Provider、多实例、新 AI Provider、更多 Publish Provider和自主 taxonomy。

只有同时满足以下条件才评估进程外插件：

- 至少三个 Provider 由核心仓库外维护。
- 编译期贡献明显阻碍发布。
- 接口经过多个真实实现并稳定。
- 可以定义版本、权限、隔离和恢复协议。

MVP 明确不采用微服务、消息中间件、外部数据库、动态模板代码、通用工作流引擎、Service Locator 和重型 DI。

## 21. 架构验收标准

- Domain 无需 Gin 和 SQLite即可测试。
- Transport 不直接依赖 SQLite、文件系统和具体 Provider。
- 四个 Provider 均通过标准 Port 接入。
- Migration 可在空库和已有 schema 执行。
- 单实例锁、优雅关闭和发布互斥可验证。
- Markdown 与 taxonomy 原子写入可验证。
- Hugo 失败恢复可验证。
- Default 和 Minimal 模板走同一标准链路。
- Secret 不进入数据库、日志和 API。
- 新代码不依赖旧 JSON 和外部导入脚本。
- 迁移期间项目持续可构建。

## 22. 后续详细设计

1. `docs/design/data-model.md`：文章模型、SQLite schema、状态机和 migration。
2. `docs/design/provider-contracts.md`：Provider 完整 Go 契约和错误模型。
3. `docs/design/wechat-template-spec.md`：manifest、CSS 允许列表和仓库协议。
4. `docs/design/interactions.md`：初始化、审核、治理和发布交互。
5. `docs/plans/mvp-implementation-plan.md`：开发、验证和回滚计划。

后续文档不得反转本文定义的依赖方向。必须调整时，先更新本文并记录原因。
