# 发布任务恢复与统一历史实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为文章页增加服务端权威的 Hugo 任务恢复、真实进度和 Hugo/微信统一发布历史。

**Architecture:** 现有 `jobs` 继续保存运行状态，`publications` 保存渠道当前投影，`publication_events` 保存成功与最终失败事实。Application 新增只读 Workflow/Timeline 服务；HTTP 只暴露文章级安全 DTO；React 组件刷新时从文章级接口恢复，不在浏览器持久化 Job ID。

**Tech Stack:** Go、SQLite、React、TypeScript、Vitest、Testing Library、Playwright、Vite。

## Global Constraints

- 所有设计、计划、交互和开发文档使用中文。
- 每个新增行为先写失败测试并实际确认 RED，再写最小实现。
- 每个功能点完成后执行 reflection；不通过时先修复，再进入下一功能点。
- 关键代码使用中文注释；Go 公开类型和方法使用中文文档注释。
- 连续开发模式下功能点之间不提交；所有回归通过后只做一次 Conventional Commit 聚合提交。
- HTTP 不返回 content hash、Job ID、数据库 ID、Vault/Hugo/staging 绝对路径、Secret 或原始 `result_json`。
- 当前工作流只匹配最近工作区、请求文章、当前内容版本和启用的 Hugo Provider。
- 历史成功与最终失败均来自 `publication_events`；Job 不直接进入历史。
- 微信现有准备、复制和人工确认行为保持兼容。

---

### Task 1: 发布任务最终失败事件与确定性任务重排

**Files:**
- Modify: `internal/app/job/runner.go`
- Modify: `internal/app/job/runner_test.go`
- Modify: `internal/storage/sqlite/repository/job.go`
- Modify: `internal/storage/sqlite/repository/job_test.go`
- Modify: `internal/app/bootstrap/publication_runner.go`
- Modify: `internal/app/bootstrap/hugo_preview_runner.go`
- Modify: `internal/app/bootstrap/hugo_preview_runner_test.go`
- Modify: `internal/app/publication/hugo_preview.go`
- Modify: `internal/app/publication/hugo_preview_test.go`
- Modify: `internal/storage/sqlite/repository/publication.go`

**Interfaces:**
- Produces: `HandlerOptions.OnTerminalFailure func(context.Context, domainjob.Job, Failure) error`。
- Produces: `JobRepository.RequeueFailed(ctx context.Context, id, workspaceID, kind string, now time.Time) (domainjob.Job, error)`。
- Modifies: `HugoPreviewJobStore` 增加 `RequeueFailed(ctx context.Context, id, workspaceID, kind string, now time.Time) (domainjob.Job, error)`，Application 不依赖 SQLite 具体类型。
- Produces: 最终失败时 `PublicationRecord.State=failed` 和 attempt 级幂等 `PublicationEvent`。

- [ ] **Step 1: 写 Runner 最终失败回调测试**

覆盖可重试错误的中间失败不调用回调、耗尽重试后只调用一次、不可重试错误立即调用一次；回调接收稳定错误码、安全消息和当前 attempt。

- [ ] **Step 2: 确认 Runner RED**

Run: `go test ./internal/app/job -run 'TestRunnerCallsTerminalFailure|TestRunnerSkipsFailureEventWhileRetrying'`

Expected: FAIL，原因是 `HandlerOptions` 没有终态失败回调。

- [ ] **Step 3: 实现终态失败回调**

在 `Runner.RunOne` 判断不再 Retry 后，先持久化 Job failed，再调用回调；回调失败记录结构化日志并返回错误，但不得把 Job 改回 running。失败对象只包含：

```go
type Failure struct {
    Code    string
    Message string
    Attempt int
}
```

- [ ] **Step 4: 写失败 Job 重排测试并确认 RED**

断言只有匹配 `id + workspaceID + kind + failed` 的任务能重排为 queued；清空 `started_at`、`finished_at` 和当前错误，保留 attempts、payload、result 与确定性 ID；queued/running/succeeded 或身份错配拒绝。

Run: `go test ./internal/storage/sqlite/repository -run TestJobRepositoryRequeuesOnlyMatchingFailedJob`

Expected: FAIL，原因是 `RequeueFailed` 不存在。

- [ ] **Step 5: 实现失败重排与 Application 复用**

`HugoPreviewService.Queue` 遇到身份匹配的 failed Preview 时调用依赖重排；`Confirm` 遇到身份匹配的 failed Deliver 时，在重新校验文章版本、Artifact 有效期和 manifest 后重排同一 Job。重复点击只返回一个 queued/running Job。

- [ ] **Step 6: 保存终态失败 Publication Event**

Bootstrap 为 `publication`、`hugo_preview`、`hugo_deliver`、`wechat_prepare` 注册失败回调。Event ID 使用：

```text
event + publication_id + failed + job_id + attempt
```

Event payload 只保存 `channel`、`error_code` 和安全 `message`。失败投影保留尝试使用的 content hash；数据库写入继续使用同一事务。

- [ ] **Step 7: 验证功能点并 reflection**

Run:

```bash
go test ./internal/app/job ./internal/app/publication ./internal/app/bootstrap ./internal/storage/sqlite/repository
git diff --check
```

检查自动重试无历史噪声、同一 attempt 不重复事件、失败重排不产生第二个 Deliver ID、微信兼容。

---

### Task 2: 当前 Hugo 工作流只读服务

**Files:**
- Create: `internal/app/publication/workflow.go`
- Create: `internal/app/publication/workflow_test.go`
- Modify: `internal/storage/sqlite/repository/job.go`
- Modify: `internal/storage/sqlite/repository/job_test.go`
- Create: `internal/app/bootstrap/publication_workflow_api.go`

**Interfaces:**
- Produces: `WorkflowResolver.ResolveWorkflowArticle(ctx, articleID) (WorkflowArticle, error)`。
- Produces: `WorkflowJobStore.FindCurrentHugoJobs(ctx, workspaceID, articleID, providerID, contentHash string) (HugoJobSnapshot, error)`。
- Produces: `PublicationWorkflowService.Find(ctx, articleID) (WorkflowView, error)`。

- [ ] **Step 1: 写工作流优先级与隔离失败测试**

测试数据同时包含其他工作区、其他文章、其他 Provider、旧 hash、当前 Preview 和当前 Deliver；断言只选当前身份，Deliver 优先于 Preview，旧版本不恢复。

- [ ] **Step 2: 确认 Workflow RED**

Run: `go test ./internal/app/publication -run TestPublicationWorkflow`

Expected: FAIL，原因是 Workflow 服务和模型不存在。

- [ ] **Step 3: 实现 Job 查询与安全阶段映射**

Repository 使用 `json_extract(payload_json, '$.article_id')`、Provider ID 和 hash 过滤，只返回有限 Job 字段。Application 按设计中的进度区间映射中文阶段：

```go
type DeliveryJobView struct {
    State    string
    Progress int
    Stage    string
    Error    string
}
```

不得把 payload/result 暴露到 `WorkflowView`。

- [ ] **Step 4: 复用 HugoPreviewService 安全视图**

Preview succeeded 时调用现有 `Find`；queued/running/failed 时只构造状态和阶段。过期由现有安全视图派生。Deliver succeeded 与 Publication 当前 hash 一致时返回 published。

- [ ] **Step 5: 实现 Bootstrap 当前工作区 Resolver**

查询必须连接最近工作区、文章和启用 Hugo Provider；文章不存在、软删除或跨工作区均返回未找到。Application 不接收客户端 Provider/Workspace/hash。

- [ ] **Step 6: 验证功能点并 reflection**

Run:

```bash
go test ./internal/app/publication ./internal/storage/sqlite/repository ./internal/app/bootstrap
git diff --check
```

检查无任务空状态、失败消息脱敏、Deliver/Preview 优先级、旧内容过滤、绝对路径不进入编码视图。

---

### Task 3: 统一发布历史 Repository 与 Application 时间线

**Files:**
- Modify: `internal/storage/sqlite/repository/publication.go`
- Modify: `internal/storage/sqlite/repository/publication_test.go`
- Create: `internal/app/publication/history.go`
- Create: `internal/app/publication/history_test.go`
- Modify: `internal/app/bootstrap/publication_workflow_api.go`

**Interfaces:**
- Produces: `PublicationRepository.ListEvents(ctx, workspaceID, articleID string, cursor EventCursor, limit int) (EventPage, error)`。
- Produces: `PublicationWorkflowService.History(ctx, articleID, cursor string, limit int) (HistoryPage, error)`。

- [ ] **Step 1: 写稳定分页与渠道合并失败测试**

插入 Hugo published/failed 与微信 prepared/copied/confirmed Event，其中两个时间相同；断言按 `created_at DESC, id DESC` 稳定排序，limit+1 生成 next cursor，第二页无重复和遗漏。

- [ ] **Step 2: 确认 History RED**

Run: `go test ./internal/storage/sqlite/repository ./internal/app/publication -run 'TestPublicationHistory|TestPublicationRepositoryListsEvents'`

Expected: FAIL，原因是 Event 查询和 History 服务不存在。

- [ ] **Step 3: 实现 Event keyset 查询**

Repository 连接 `publications` 与 `provider_instances`，限定 workspace/article，返回 Provider type、event type、payload 和时间。cursor 使用时间与 Event ID；limit 范围由 Application 限制为 1-50。

- [ ] **Step 4: 实现不透明 cursor 与自然语言映射**

cursor 编码内容包含 workspace、article、createdAt、eventID，并使用现有稳定 JSON/Base64 模式；跨文章 cursor 返回 `ErrHistoryCursorInvalid`。映射：

```text
hugo/published  → 已同步到 Hugo
hugo/failed     → Hugo 同步失败
wechat/prepared → 微信内容已准备
wechat/copied   → 微信内容已复制
wechat/confirmed→ 已确认保存微信草稿
wechat/failed   → 微信内容处理失败
```

未知或损坏 payload 使用安全通用详情。

- [ ] **Step 5: 验证功能点并 reflection**

Run:

```bash
go test ./internal/storage/sqlite/repository ./internal/app/publication ./internal/app/bootstrap
git diff --check
```

检查相同时间排序、非法 cursor、跨工作区、未知事件、失败 payload 脱敏和成功无重复。

---

### Task 4: 文章级 Workflow 与 History HTTP API

**Files:**
- Create: `internal/transport/http/publication_workflow.go`
- Create: `internal/transport/http/publication_workflow_test.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/app/bootstrap/bootstrap.go`

**Interfaces:**
- Adds: `GET /api/v1/articles/{id}/publication-workflow`。
- Adds: `GET /api/v1/articles/{id}/publication-history?cursor=&limit=`。
- Consumes: Task 2/3 的最小 `PublicationWorkflowAPI`。

- [ ] **Step 1: 写 HTTP 安全与分页失败测试**

覆盖空 Workflow、Ready Preview、Delivering、History next cursor、limit 边界、非法 cursor、其他工作区未找到。fixture 使用 `/secret/hugo` 和 `/secret/staging`，断言响应不包含这些值、hash、job ID 或 `result_json`。

- [ ] **Step 2: 确认 HTTP RED**

Run: `go test ./internal/transport/http -run TestPublicationWorkflow`

Expected: FAIL，原因是 RuntimeOptions、路由和 Handler 不存在。

- [ ] **Step 3: 实现最小 API 与安全 DTO**

Transport 只解析 article ID、cursor、limit，并逐字段构造 snake_case 响应。Workflow 无任务返回 `200`；非法 limit 返回 `400 request.invalid`；非法 cursor 返回 `400 request.cursor_invalid`；未找到返回现有统一 404。

- [ ] **Step 4: 装配 Bootstrap**

`RuntimeOptions` 注入 Task 2/3 API，不允许 HTTP 直接查询 Job `result_json` 或 Provider 配置。保留通用 `/jobs/{id}` 的有限字段响应。

- [ ] **Step 5: 验证功能点并 reflection**

Run:

```bash
go test ./internal/transport/http ./internal/app/bootstrap
git diff --check
```

检查路由顺序不被通用 article detail 捕获、GET 无写副作用、错误文案中文、所有内部标识脱敏。

---

### Task 5: React 工作流恢复与统一发布历史

**Files:**
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/components/HugoPublishFlow.tsx`
- Modify: `web/app/src/components/HugoPublishFlow.test.tsx`
- Create: `web/app/src/components/PublicationHistory.tsx`
- Create: `web/app/src/components/PublicationHistory.test.tsx`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/styles/app.css`
- Modify: `web/app/vite.config.ts`
- Modify: `web/app/e2e/workflows.spec.ts`

**Interfaces:**
- Adds: `getPublicationWorkflow(articleID, signal)`。
- Adds: `getPublicationHistory(articleID, cursor?, signal?)`。
- Adds: `PublicationHistory({ articleID, refreshKey })`。

- [ ] **Step 1: 写 Hugo 刷新恢复失败测试**

覆盖 Preparing 自动轮询、Ready 直接显示 Artifact 与确认、Delivering 显示真实进度且无确认按钮、Expired 重新生成、Failed 错误与重试、组件卸载后不再请求。

- [ ] **Step 2: 确认 Hugo 组件 RED**

Run: `npm test -- --run src/components/HugoPublishFlow.test.tsx`

Expected: FAIL，原因是组件仍直接加载 Section，无法读取文章级 Workflow。

- [ ] **Step 3: 重构 HugoPublishFlow 状态机**

首次请求 Workflow；只在无当前流程时加载 Section。所有 timer 保存到 ref，文章变化或卸载时 clearTimeout + AbortController.abort。Deliver succeeded 调用 `onPublished`；失败操作使用全局 Toast 和页面内持久状态。

- [ ] **Step 4: 写统一历史失败测试**

覆盖默认折叠、展开加载 Hugo/微信自然语言项、加载更多稳定追加、失败保留已有项并显示重新加载、refreshKey 变化重载第一页。

- [ ] **Step 5: 确认 History 组件 RED**

Run: `npm test -- --run src/components/PublicationHistory.test.tsx`

Expected: FAIL，原因是组件和 API 不存在。

- [ ] **Step 6: 实现 History 与 ArticlePage 集成**

历史使用 `<details>` 或等价可访问 disclosure，默认关闭；条目为无框时间线。ArticlePage 在发布成功后刷新文章并增加 `historyRefreshKey`，不能在客户端插入伪历史。

- [ ] **Step 7: 更新 Demo API 与 E2E fixture**

Demo 提供 Preparing→Ready、Delivering→Published 的可重复响应和至少一条 Hugo/微信历史；E2E 刷新 Ready 页面后仍显示确认，发布成功后历史出现新事件。

- [ ] **Step 8: 验证功能点并 reflection**

Run:

```bash
cd web/app
npm test -- --run
npm run typecheck
npm run lint
```

检查重复点击、定时器清理、空状态、错误反馈、移动发布标签、长路径换行、无嵌套卡片和无内部 ID。

---

### Task 6: 文档、服务重启、真实页面与整体回归

**Files:**
- Modify: `docs/design/interactions.md`
- Modify: `docs/design/data-model.md`
- Modify: `.codex/HANDOFF.md`
- Generated: `web/dist/*`

**Interfaces:**
- Validates: Task 1-5 跨层组合行为。
- Produces: 一次聚合 Conventional Commit。

- [ ] **Step 1: 更新中文设计与交接文档**

记录文章级恢复、统一 History DTO、失败 Event、failed Job 显式重排、分页和安全边界。不得把专项设计改成英文摘要。

- [ ] **Step 2: 执行整体 reflection**

逐项检查：当前工作区隔离、旧 hash、Preview/Deliver 优先级、Artifact 过期、失败重排、attempt 事件幂等、服务重启、微信回归、cursor、绝对路径泄露、定时器、重复点击、移动端遮挡、日志诊断。

- [ ] **Step 3: 修复 reflection 问题**

每个行为问题先补失败测试并确认 RED，再做最小实现并运行对应 GREEN 测试。纯文案或注释修正执行格式检查。

- [ ] **Step 4: 验证服务重启恢复**

使用临时 SQLite、Vault 和 Hugo fixture：让 `hugo_preview`/`hugo_deliver` 进入 running，重启 Runner 后执行 Recover，断言可重试任务重新 queued、同一 Artifact 不重复写入、History 不产生中间重试事件。

- [ ] **Step 5: 真实页面验证**

启动 Vite Demo 服务，使用浏览器在 1440×1000 和 390×844 验证 Ready 刷新恢复、Delivering 进度、发布成功历史、加载更多、失败反馈、无 console error 和 `scrollWidth === clientWidth`。

- [ ] **Step 6: 完整验证**

Run:

```bash
cd web/app
npm test -- --run
npm run typecheck
npm run lint
npm run build
npx playwright test e2e/workflows.spec.ts --workers=1
cd ../..
go test ./...
go vet ./...
git diff --check
```

Expected: 全部 PASS。

- [ ] **Step 7: 复查并聚合提交**

查看 `git diff` 与 `git status --short`，排除 test-results、trace、临时数据库和 fixture，只暂存本计划涉及文件。提交：

```bash
git commit -m "feat(publication): recover workflows and history"
```
