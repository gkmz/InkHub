# Codex Handoff

## Current Goal

在独立的 InkHub 项目中，按已确认的 PRD 和架构设计，开发一个本地优先、简单易用的开源内容工作台：

```text
Obsidian Vault → InkHub 审核/AI/SEO/Taxonomy → Hugo + 微信公众号
```

## Constraints

- 产品名：`InkHub`，CLI 和仓库命名使用 `inkhub`。
- MVP 只实现：Obsidian + Hugo + 微信公众号。
- 架构必须提供四类标准 Provider：Source、AI、Publish、Taxonomy。
- MVP Provider：Obsidian Source、OpenAI-compatible AI、Hugo Publish、WeChat Publish。
- 微信公众号只能做模板渲染、图片处理、HTML 复制和人工草稿确认，不依赖不可用的自动发布接口。
- Hugo 内容转换、page bundle、图片复制、路径重写、taxonomy 校验和构建必须内建，不依赖旧脚本。
- Hugo 标准配置、文章 frontmatter 和 taxonomy term 页面是权威来源；SQLite 持久化最近成功快照、同步状态和统计。
- 稳定文章 ID 在 MVP 中写入 Obsidian Markdown frontmatter，不使用 sidecar 或仅数据库身份。
- 正文始终保存在 Obsidian Vault，SQLite 只保存索引、配置、状态、历史和任务。
- 设计目标是简单、精美、易用，避免 CMS 化、复杂大屏、动态插件市场和多余设置。
- 代码规范：后端 Go；关键代码中文注释；公开方法中文文档注释；Conventional Commits。

## Done

- 已完成产品定位和需求讨论。
- 已创建并审核 PRD：`docs/PRD.md` 1.6。
- 已创建架构设计：`docs/design/architecture.md` 1.2。
- 已创建并完成 reflection 审查：
  - `docs/design/data-model.md`。
  - `docs/design/provider-contracts.md`。
  - `docs/design/wechat-template-spec.md`。
  - `docs/design/interactions.md`。
  - `docs/plans/mvp-implementation-plan.md`。
- 架构设计包含：
  - 模块化单体和分层依赖。
  - Core、Application、Domain、Infrastructure、Transport 边界。
  - Source、AI、Publish、Taxonomy 四类 Provider Port 和 MVP 实现。
  - SQLite、任务、文件监听、原子写入和 Hugo staging。
  - AI、Hugo、微信完整数据流。
  - 微信模板安全、版本和仓库分发原则。
  - 现有旧代码的迁移表。
  - 测试架构和架构验收标准。
- 已确认现有旧代码只作为行为参考：Markdown 渲染、代码高亮、WikiLink、callout、图片上传、Mermaid 和微信复制行为可迁移；旧 Handler、JSON 状态、路径 ID、平台列表和外部导入脚本不作为新架构基础。

## Next

Hugo Section 发现、真实 Artifact 预览、用户确认和同 Artifact 原子交付闭环已经完成。下一步按 PRD 优先级补齐页面刷新后的运行任务恢复与发布历史，或推进微信图片处理的真实渠道验证；Tags 是多值集合，继续保持独立组件，不复用单值 taxonomy 字段。继续遵循每个功能先写失败测试、实现后 reflection、公开方法中文文档注释和关键代码中文注释的质量门禁。

## Important Decisions

- InkHub 是全新产品，不承诺兼容旧 `markdown-preview` 的代码、JSON、API 或 CLI 参数。
- MVP Release 1 = 开发阶段 A + 开发阶段 B，并通过 PRD 第 32 章全部验收标准。
- MVP 每个工作区只能有一个 Hugo 和一个微信 Provider 实例；数据模型为后续多实例预留 ID，但 UI 不开放多实例。
- MVP 只支持固定的 Obsidian frontmatter 标准：`id`、`title`、`description`、`tags`、`keywords` 和 `publish.category/series/slug/cover`。
- AI 只生成候选，不直接覆盖；用户逐项采用 Tag 后仍需保存文章，新 Tag 随文章发布和 taxonomy 刷新自然入库。
- 内链建议移到 Release 1 之后。
- 模板使用 target/format/renderer/compatibility 通用模型；MVP 的 `InkHub Default` 和 `InkHub Minimal` 目标均为 `wechat-html`。
- 现有 Hugo 导入脚本不作为 InkHub 运行依赖；Hugo 逻辑归入内部 Hugo Publish Provider。
- 不建立动态插件系统，Provider 先编译期注册。
- Application 和 Transport 的文章运行链路通过 Provider Runtime 构建 Source/Publish/Taxonomy；具体工厂只在 bootstrap 注册。
- `markdown-folder` 只读 Source 用于验证扩展契约，MVP UI 暂不开放配置入口。
- 类目管理页面从 SQLite 快照即时展示 categories/tags；手工刷新调用 Taxonomy Service，新建 category 必须先预览 Hugo 原生 term page，再按 revision 确认应用。
- 文章审核页 Category/Series 通过共用的单值字段从 Taxonomy Provider 快照选择；快照外旧值明确保留，同页创建 term 后只回填发起字段的草稿，仍需用户保存文章。
- 文章 Tags 使用独立的可创建多选器，从 SQLite taxonomy 投影查询候选和文章数量；AI 使用现有候选生成结构化建议，使用次数由 Application 从快照补充，不能信任模型输出。
- OpenAI-compatible 设置已接入系统 Secret Store；API Key 不进入 SQLite、HTTP 响应或日志。
- Hugo 新文章从已扫描的一级 Section 中选择目标；已有文章按 `source_id` 锁定原 Section，本阶段不支持跨 Section 移动或目录管理。
- Hugo 发布使用 `hugo_preview` 与 `hugo_deliver` 两类确定性任务：Preview 只写 staging 并保存完整 Artifact，确认重新校验后 Deliver 同一个 Artifact；HTTP 只返回相对目标和文件摘要。

## Files

- `/Users/hank/workspace/mine/InkHub/docs/PRD.md`
- `/Users/hank/workspace/mine/InkHub/docs/design/architecture.md`
- 旧行为参考：`old/markdown/processor.go`、`old/services/mermaid.go`、`old/services/uploader.go`、`old/services/publisher.go`、`old/scanner/scanner.go`、`old/models/*`、`old/handlers/*`、`old/web/*`

## Verification

- Provider Runtime、Hugo 标准 taxonomy、SQLite 快照、类目查询/刷新/预览/应用 API、类目管理页面、文章 Category/Series/Tags 编辑、AI Tag 建议、模板 target 和第二 Source 均有单元测试。
- 每阶段已运行全量 `go test ./...`、`go vet ./...` 和相关 race 测试。
- 当前项目实际路径：`/Users/hank/workspace/mine/InkHub`。

## Risks / Open Questions

- 微信 CSS 允许列表和内联结果需要用真实公众号后台粘贴验证。
- Keychain 在 macOS/Linux/Windows 的统一实现需要在平台层确定降级策略。
- 模板索引默认使用 `https://raw.githubusercontent.com/gkmz/InkHub/main/templates/index.json`；MVP 由当前仓库 PR 和 CI 维护。

## Fresh Session Prompt

请先阅读项目指令文件、`docs/PRD.md`、`docs/design/architecture.md`、专项设计和本 handoff。开始前运行 `git status --short`，确认当前项目是 `/Users/hank/workspace/mine/InkHub`；Provider 抽象、类目管理、文章元数据与 AI Tag、Hugo 发布预览确认闭环已完成，下一步按 PRD 优先级补齐任务恢复与发布历史，或推进微信真实渠道验证。
