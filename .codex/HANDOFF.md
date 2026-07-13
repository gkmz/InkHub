# Codex Handoff

## Current Goal

在独立的 InkHub 项目中，按已确认的 PRD 和架构设计，开发一个本地优先、简单易用的开源内容工作台：

```text
Obsidian Vault → InkHub 审核/AI/SEO/Taxonomy → Hugo + 微信公众号
```

## Constraints

- 产品名：`InkHub`，CLI 和仓库命名使用 `inkhub`。
- MVP 只实现：Obsidian + Hugo + 微信公众号。
- 架构必须提供三类标准 Provider：Source、AI、Publish。
- MVP Provider：Obsidian Source、OpenAI-compatible AI、Hugo Publish、WeChat Publish。
- 微信公众号只能做模板渲染、图片处理、HTML 复制和人工草稿确认，不依赖不可用的自动发布接口。
- Hugo 内容转换、page bundle、图片复制、路径重写、taxonomy 校验和构建必须内建，不依赖旧脚本。
- Hugo `data/taxonomy.yaml` 是 MVP taxonomy 唯一权威来源，SQLite 只存缓存和统计。
- 稳定文章 ID 在 MVP 中写入 Obsidian Markdown frontmatter，不使用 sidecar 或仅数据库身份。
- 正文始终保存在 Obsidian Vault，SQLite 只保存索引、配置、状态、历史和任务。
- 设计目标是简单、精美、易用，避免 CMS 化、复杂大屏、动态插件市场和多余设置。
- 代码规范：后端 Go；关键代码中文注释；公开方法中文文档注释；Conventional Commits。

## Done

- 已完成产品定位和需求讨论。
- 已创建并审核 PRD：`docs/PRD.md` 1.2。
- 已创建架构设计：`docs/design/architecture.md` 1.1。
- 已创建并完成 reflection 审查：
  - `docs/design/data-model.md`。
  - `docs/design/provider-contracts.md`。
  - `docs/design/wechat-template-spec.md`。
  - `docs/design/interactions.md`。
  - `docs/plans/mvp-implementation-plan.md`。
- 架构设计包含：
  - 模块化单体和分层依赖。
  - Core、Application、Domain、Infrastructure、Transport 边界。
  - 三类 Provider Port 和 MVP 实现。
  - SQLite、任务、文件监听、原子写入和 Hugo staging。
  - AI、Hugo、微信完整数据流。
  - 微信模板安全、版本和仓库分发原则。
  - 现有旧代码的迁移表。
  - 测试架构和架构验收标准。
- 已确认现有旧代码只作为行为参考：Markdown 渲染、代码高亮、WikiLink、callout、图片上传、Mermaid 和微信复制行为可迁移；旧 Handler、JSON 状态、路径 ID、平台列表和外部导入脚本不作为新架构基础。

## Next

任务 1 至任务 9 已完成实现、代码审查、真实 Hugo 集成测试和全量验证。下一步按实施计划执行任务 10：微信模板引擎与 Publish Provider。继续遵循每个功能先写失败测试、实现后 reflection、公开方法中文文档注释和关键代码中文注释的质量门禁；每个任务完成后立即提交。

## Important Decisions

- InkHub 是全新产品，不承诺兼容旧 `markdown-preview` 的代码、JSON、API 或 CLI 参数。
- MVP Release 1 = 开发阶段 A + 开发阶段 B，并通过 PRD 第 32 章全部验收标准。
- MVP 每个工作区只能有一个 Hugo 和一个微信 Provider 实例；数据模型为后续多实例预留 ID，但 UI 不开放多实例。
- MVP 只支持固定的 Obsidian frontmatter 标准：`id`、`title`、`description`、`tags`、`keywords` 和 `publish.category/series/slug/cover`。
- AI 只推荐，不直接覆盖；新 tag 必须人工准入并受控写回 Hugo taxonomy。
- 内链建议移到 Release 1 之后。
- 微信模板从 MVP 起标准化、可校验、可安装、可分享；`InkHub Default` 和 `InkHub Minimal` 走同一模板链路。
- 现有 Hugo 导入脚本不作为 InkHub 运行依赖；Hugo 逻辑归入内部 Hugo Publish Provider。
- 不建立动态插件系统，Provider 先编译期注册。

## Files

- `/Users/hank/workspace/mine/InkHub/docs/PRD.md`
- `/Users/hank/workspace/mine/InkHub/docs/design/architecture.md`
- 旧行为参考：`old/markdown/processor.go`、`old/services/mermaid.go`、`old/services/uploader.go`、`old/services/publisher.go`、`old/scanner/scanner.go`、`old/models/*`、`old/handlers/*`、`old/web/*`

## Verification

- 已对 PRD 和架构文档运行 `git diff --check`，无空白错误。
- 文档仅新增 `docs/`，尚未提交 Git。
- 当前项目实际路径：`/Users/hank/workspace/mine/InkHub`。
- 旧项目原始代码仍在仓库中，尚未开始迁移实现。

## Risks / Open Questions

- SQLite 具体驱动尚未确定；优先选择不依赖 CGO 的实现。
- Hugo staging 的完整站点构建策略需要在 Provider 设计中通过临时 fixture 验证。
- 微信 CSS 允许列表和内联结果需要用真实公众号后台粘贴验证。
- Keychain 在 macOS/Linux/Windows 的统一实现需要在平台层确定降级策略。
- 模板索引默认使用 `https://raw.githubusercontent.com/gkmz/InkHub/main/templates/index.json`；MVP 由当前仓库 PR 和 CI 维护。

## Fresh Session Prompt

请先阅读项目指令文件、`docs/PRD.md`、`docs/design/architecture.md`、四份专项设计、`docs/plans/mvp-implementation-plan.md` 和本 handoff。不要依赖旧聊天历史；开始前先运行 `git status --short`，确认当前项目是 `/Users/hank/workspace/mine/InkHub`，并从实施计划 Task 1 开始。
