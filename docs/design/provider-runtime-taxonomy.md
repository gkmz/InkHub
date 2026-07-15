# Provider Runtime、Taxonomy 与模板目标设计

> 本文档定义 InkHub 从 MVP 内置实现演进到可扩展 Provider 主链路的边界。

## 1. 目标

- Application 只依赖 Provider 契约和统一 Runtime，不直接构造 Obsidian、Hugo、微信等具体实现。
- Hugo 标准配置、文章 frontmatter 和 taxonomy term 页面是 Hugo taxonomy 的权威来源。
- SQLite 持久化 taxonomy 投影、使用统计和同步状态，启动时无需重复全量扫描。
- 模板资源通过明确的目标和格式选择 Renderer，不再默认等同于微信公众号模板。
- MVP 继续采用编译期注册，不引入动态插件、脚本执行或第三方进程内代码。

## 2. Provider Runtime

Registry 是内置 Provider 工厂的唯一注册点，同时实现 `ProviderRuntime`：

```go
type ProviderRuntime interface {
    BuildSource(context.Context, ProviderRef, ConfigView) (SourceProvider, error)
    BuildAI(context.Context, ProviderRef, ConfigView) (AIProvider, error)
    BuildPublish(context.Context, ProviderRef, ConfigView) (PublishProvider, error)
    BuildTaxonomy(context.Context, ProviderRef, ConfigView) (TaxonomyProvider, error)
}
```

bootstrap 只负责注册内置工厂和装配依赖。Application 从持久化实例读取 `provider_type`、配置、授权根目录和 Secret 引用，再交给 Runtime 构建。任何业务用例不得直接 import 具体 Provider 包，也不得按 `provider_type` 分支。

发布渠道差异通过 Descriptor 表达：`DeliveryMode` 取 `automatic`、`manual_confirmation` 或 `prepare_only`。Application 始终调用 `Preflight` 和 `Prepare`；仅 `automatic` 调用 `Deliver`，其他模式保存待人工确认的产物。

## 3. Taxonomy Provider

Taxonomy 生命周期不同于文章发布，因此使用独立契约：

```go
type TaxonomyProvider interface {
    Descriptor() TaxonomyDescriptor
    Validate(context.Context) error
    Discover(context.Context, TaxonomyCursor) (TaxonomySnapshot, error)
    PlanChange(context.Context, TaxonomyCommand) (TaxonomyChangeSet, error)
    ApplyChange(context.Context, TaxonomyChangeSet) (TaxonomySnapshot, error)
    Watch(context.Context, chan<- TaxonomyChange) error
}
```

`TaxonomySnapshot` 包含 provider instance、revision、完整性标记、term 列表和诊断。Term 使用 `(kind, key)` 作为渠道内稳定标识，展示名与规范名分离；kind 至少支持 category、tag 和自定义 taxonomy 名称，不把 Hugo 的自定义 taxonomy 压缩成固定枚举。

`PlanChange` 只生成可审阅变更，`ApplyChange` 必须校验 `ExpectedRevision`，防止覆盖用户在 Hugo 中的外部修改。

## 4. Hugo 标准实现

`HugoStandardTaxonomyProvider` 按以下顺序发现数据：

1. 解析 `hugo.toml`、`hugo.yaml`、`hugo.yml` 或 `hugo.json` 中的 `taxonomies`；未配置时采用 Hugo 默认 `category/categories` 和 `tag/tags`。
2. 扫描 `content` 下文章 frontmatter，按配置的 plural key 汇总 term 和使用次数。
3. 读取 `content/<plural>/<term>/_index.md` 或对应 branch bundle，补充 title、description、aliases 等元数据。
4. 对以上输入计算确定性 revision；输入未变化时允许返回未修改结果。

Hugo 文件仍是权威来源。SQLite 只保存最近成功的完整快照；发现失败不得清空旧快照，而是记录失败状态和诊断。旧 `data/taxonomy.yaml` 不再参与标准发现或发布校验，也不自动删除。

## 5. SQLite 投影

新增 taxonomy snapshot 元数据，至少记录：provider instance、revision、同步状态、是否完整、最近成功时间、最近尝试时间和脱敏错误。`taxonomy_terms` 记录外部 key、kind、name、canonical name、元数据、usage count 和 source revision。

刷新采用单事务替换当前 Provider 的完整 term 集合：先写新 snapshot 和 terms，再删除该 Provider 中已消失的 terms。刷新失败回滚事务并保留上次成功数据。文章索引更新后可重算关联与使用次数，但不能改变外部权威 term。

## 6. 模板目标

保留现有模板包的不可变版本、摘要、资源许可和安全安装机制。Manifest 增加：

- `target`：稳定目标，例如 `wechat-html`。
- `format`：入口格式，例如 `css`。
- `renderer`：所需 Renderer 契约标识。
- `compatibility`：目标 Provider 类型和版本约束。

`TemplateRef` 同时携带 target；Publish Provider 在预检阶段拒绝目标不兼容的模板。首个 Renderer 仍是微信 HTML，Hugo 不因本次抽象获得主题模板功能。

## 7. 兼容与错误处理

- migration 只新增表和字段，不重建用户数据表。
- 已安装微信模板迁移为 `target=wechat-html`、`format=css`、`renderer=wechat-html-v1`。
- Runtime 对未知、禁用或类型不匹配的实例返回结构化配置错误。
- taxonomy 配置不可解析时保留缓存，并在设置和诊断页面显示可操作错误。
- 本轮不实现动态插件、远程 taxonomy、多值文章 category UI 或 Hugo 主题管理。

## 8. 验收标准

- 扫描和发布主链路不直接 import 具体 Provider。
- 新增编译期 Provider 只需实现工厂并在 bootstrap 注册，不修改 Application 用例。
- Hugo fixture 可从配置、frontmatter 和 term 页面发现 taxonomy，并生成稳定 revision。
- SQLite 在重启后立即返回最近成功快照，失败刷新不会丢失旧数据。
- 微信模板兼容性由 target/renderer 校验，旧内置模板完成兼容迁移。
- 全量 Go 测试、前端测试和构建通过，关键边界具备中文注释和公开方法文档注释。
