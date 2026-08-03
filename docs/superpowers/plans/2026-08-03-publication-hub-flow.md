# 审核中心与独立发布渠道 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将文章审核页收敛为元数据审核中心，并提供 Hugo、微信、小红书三个互不阻塞、可恢复、可查看历史的独立发布页面。

**Architecture:** 保留内容库到 `/articles/:id` 的主入口，将审核页改为只处理元数据、检查、AI 建议和审核状态；新增共享的渠道导航与状态摘要组件，三个渠道页面通过统一导航互相切换。Hugo 现有预览任务移动到独立页面，并扩展安全错误摘要，让失败状态携带阶段、代码、原因和动作。

**Tech Stack:** Go HTTP transport and application services, SQLite-backed jobs and publication history, React + TypeScript + Vitest, Vite embedded assets.

## Global Constraints

- 内容库始终进入文章审核中心；不新增独立主导航项。
- 审核通过后三个渠道立即可见，但不自动跳转、不互相阻塞。
- 已成功渠道保留历史；文章更新只标记受影响产物过期，不强制重新生成 AI 建议。
- 页面不得将内部绝对路径、job ID 或 provider 私有数据展示给用户。
- 关键代码使用中文注释，公开 Go 方法有文档注释；提交使用 Conventional Commits。
- 保持现有 API 和数据库兼容，优先在现有 `ArticleDetail`、工作流和历史接口上扩展。

---

### Task 1: 建立共享渠道导航与状态摘要

**Files:**
- Create: `web/app/src/components/PublicationChannelNav.tsx`
- Create: `web/app/src/components/PublicationChannelNav.test.tsx`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- `PublicationChannelNav` consumes `{ article: Pick<ArticleDetail, "id" | "review_state" | "hugo_state" | "wechat_state" | "xiaohongshu_state">; active: PublicationChannel; onNavigate: (path: string) => void }`.
- It produces four accessible actions: `审核`, `Hugo`, `微信`, `小红书`; each action resolves to `/articles/:id`, `/articles/:id/hugo`, `/articles/:id/wechat`, or `/articles/:id/xiaohongshu`.
- Add `PublicationDisplayState` and `PublicationChannelSummary` types for display-only normalization; do not change persisted state values.

- [x] **Step 1: Write the failing component tests**

Add tests that render an approved article and assert all three channels are visible, the active channel is marked, and clicking `小红书` navigates to `/articles/a1/xiaohongshu`. Add a second test for an unapproved article asserting channel actions are disabled with `审核通过后可用`.

Run:

```bash
cd web/app
npm test -- --run src/components/PublicationChannelNav.test.tsx
```

Expected: FAIL because the component and display-state helper do not exist.

- [x] **Step 2: Add display-state normalization**

In `web/app/src/api/types.ts`, add:

```ts
export type PublicationDisplayState = "blocked" | "not_configured" | "ready" | "running" | "completed" | "failed" | "stale";
export interface PublicationChannelSummary {
  channel: PublicationChannel;
  label: string;
  state: PublicationDisplayState;
  rawState: string;
  actionLabel: string;
}
```

Implement `normalizePublicationState(rawState, reviewState, configured)` in `PublicationChannelNav.tsx`, mapping known Chinese states (`已同步`, `已准备`, `已复制`, `已确认`, `失败`, `处理中`, `需要重新处理`, `尚未准备`) and using `blocked` when `review_state !== "已通过"`.

- [x] **Step 3: Implement the shared navigation**

Render a compact `nav` with one button per channel. The channel button must include visible state text, an icon, `aria-current` for the active channel, and `title`/disabled behavior for blocked channels. `审核` always routes back to the article review center. Use existing Lucide icons and keep the component independent of channel-specific execution details.

- [x] **Step 4: Add responsive styles**

Add `.publication-channel-nav`, `.publication-channel-item`, and state modifiers to `web/app/src/styles/app.css`. Use a single row on desktop and a wrapping row on narrow screens; no horizontal overflow and no nested card styling.

- [x] **Step 5: Run the focused tests**

```bash
cd web/app
npm test -- --run src/components/PublicationChannelNav.test.tsx
```

Expected: PASS.

---

### Task 2: Refactor the article page into a review-only center

**Files:**
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/components/PublicationTrack.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- `ArticlePage` keeps `onNavigate` and existing metadata/AI interfaces, but no longer imports or renders `HugoPublishFlow`.
- The review command bar exposes only `审核通过` before approval and `进入发布渠道` after approval; it never chooses Hugo as an implicit next step.
- `PublicationTrack` remains a read-only summary and delegates channel navigation to `PublicationChannelNav`.

- [x] **Step 1: Add failing workflow tests**

Extend `article-workflow.test.tsx` with tests that mock a pending-review article, assert no element with accessible name `Hugo 发布` exists, click `审核通过`, and then assert `同步到 Hugo`, `发布到微信`, and `发布到小红书` are visible. Add a second test for an article with `hugo_state=已同步` asserting 微信 and小红书 remain directly available.

Run:

```bash
cd web/app
npm test -- --run src/pages/article/article-workflow.test.tsx
```

Expected: FAIL because the current page still renders the inline flow and exposes only one primary next action.

- [x] **Step 2: Remove inline Hugo workflow state and rendering**

Delete `showHugoFlow`, its recovery `useEffect`, `historyRefreshKey`, and the `<HugoPublishFlow>` render from `ArticlePage`. Keep `PublicationHistory` as a collapsed read-only section.

- [x] **Step 3: Add the review-center publication section**

After `PublicationTrack` and before the metadata form, render the shared `PublicationChannelNav` inside a `publication-center` section. The section must show `先完成审核` while pending and `请选择需要处理的渠道` after approval. Before approval the channel buttons remain visible but disabled.

- [x] **Step 4: Simplify the primary review command**

Set `primary` to `审核通过` while pending review and to a non-submitting `查看发布渠道` action after approval. Do not mutate `hugo_state` in the review callback; use the backend review response or reload as the source of truth.

- [x] **Step 5: Run focused frontend tests and typecheck**

```bash
cd web/app
npm test -- --run src/pages/article/article-workflow.test.tsx
npm run typecheck
```

Expected: PASS with no inline Hugo section rendered.

---

### Task 3: Move Hugo execution into a standalone channel page and add cross-channel entry points

**Files:**
- Create: `web/app/src/pages/hugo/HugoPage.tsx`
- Create: `web/app/src/pages/hugo/HugoPage.test.tsx`
- Modify: `web/app/src/app.tsx`
- Modify: `web/app/src/pages/wechat-preview/WeChatPreviewPage.tsx`
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx`
- Modify: `web/app/src/components/HugoPublishFlow.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- `HugoPage({ articleID, onNavigate })` loads `ArticleDetail`, renders article title/review state, `PublicationChannelNav active="hugo"`, and the existing `HugoPublishFlow` as the main content.
- `HugoPublishFlow` keeps its existing props but its `onPublished` callback reloads Hugo page data instead of mutating the review page.
- WeChat and Xiaohongshu pages render `PublicationChannelNav` with their current article object and route to the other two channels.

- [x] **Step 1: Add failing route and page tests**

Add tests that render `HugoPage`, assert the channel navigation is present, and assert the page does not render an article metadata form. Add a route assertion that `/articles/a1/hugo` resolves to `HugoPage`.

Run:

```bash
cd web/app
npm test -- --run src/pages/hugo/HugoPage.test.tsx
```

Expected: FAIL because the route and page do not exist.

- [x] **Step 2: Implement `HugoPage`**

Load the article and publication workflow in one page. Render a top toolbar with `返回审核`, article title, current review state, and the shared channel navigation. Render `HugoPublishFlow` in a first-viewport main section with the current content hash. Add a collapsed `PublicationHistory` below the flow. On completion reload the article and workflow.

- [x] **Step 3: Register `/hugo` in the router**

Change the article route matcher in `web/app/src/app.tsx` from `(/wechat|/xiaohongshu)?` to `(/hugo|/wechat|/xiaohongshu)?`. Route `/hugo` to `HugoPage`; keep the existing two routes otherwise unchanged.

- [x] **Step 4: Add cross-channel navigation to WeChat and Xiaohongshu**

Render `PublicationChannelNav` in each page header below the article identity. Keep existing content generation and manual confirmation semantics. Navigation remains usable while a channel task is running and does not cancel or reset local state.

- [x] **Step 5: Update Hugo flow layout and run tests**

Remove the assumption that the flow is nested in a review sidebar. Add standalone styles for `.hugo-page`, `.hugo-page-main`, and `.channel-page-header`.

```bash
cd web/app
npm test -- --run src/components/HugoPublishFlow.test.tsx src/pages/hugo/HugoPage.test.tsx src/pages/wechat-preview/WeChatPlan.test.tsx src/pages/xiaohongshu/xiaohongshuAdapter.test.ts
npm run typecheck
```

---

### Task 4: Preserve and expose actionable Hugo failure diagnostics

**Files:**
- Modify: `internal/app/publication/hugo_preview.go`
- Modify: `internal/app/bootstrap/hugo_preview_runner.go`
- Modify: `internal/transport/http/hugo_preview.go`
- Modify: `internal/app/bootstrap/hugo_preview_runner_test.go`
- Modify: `internal/transport/http/hugo_preview_test.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/components/HugoPublishFlow.tsx`
- Modify: `web/app/src/components/HugoPublishFlow.test.tsx`

**Interfaces:**
- Add `PublicationFailure` with `stage`, `code`, `message`, `action`, and `retryable` to the safe Hugo preview/workflow view.
- The runner persists a stable provider error code and safe message; the application view derives stage, action, and user retryability from the job kind and error code so failed jobs remain recoverable without a result payload.
- The UI displays diagnostics from the API and falls back to the current generic message when older responses omit the new fields.

- [x] **Step 1: Add a failing backend test for stage-aware failure**

Extend `TestHugoPreviewJobPersistsTerminalFailureEvent` to assert the failed job and event use the first blocking diagnostic's stable code and safe message. Add an application-service test asserting `PreviewView.Failure` derives `stage: "preflight"`, a user-facing `action`, and UI retryability from the persisted job. Add a transport fixture with a failed view and assert the JSON response contains all five failure fields.

Run:

```bash
go test ./internal/app/bootstrap ./internal/transport/http
```

Expected: FAIL because the publication view and event payload do not expose failure metadata.

- [x] **Step 2: Add Go failure view types and stage propagation**

In `internal/app/publication/hugo_preview.go`, add:

```go
type PublicationFailure struct {
    Stage string `json:"stage"`
    Code string `json:"code"`
    Message string `json:"message"`
    Action string `json:"action"`
    Retryable bool `json:"retryable"`
}
```

Attach `*PublicationFailure` to `PreviewView` and the recovered workflow view. In `handlePreview`, when `Preflight` returns non-ready diagnostics, return a `contracts.ProviderError` containing the first blocking diagnostic code and safe message with runner retry disabled. In `HugoPreviewService.Find`, derive `PublicationFailure` from `job.Kind`, `job.ErrorCode`, and `job.ErrorMessage`; map known codes such as `source.image_unresolved`, `hugo.article_invalid`, and `hugo.operation_invalid` to explicit actions. This keeps failure detail durable even though failed jobs intentionally have no `result_json`.

- [x] **Step 3: Serialize safe failure fields**

Update `safeHugoPreviewView` and `safeWorkflowView` to include a nested `failure` object only when present. Do not include provider paths, staging paths, job payloads, or stack traces.

- [x] **Step 4: Render actionable failure UI**

Extend `HugoPreviewView`, `RecoveredHugoPreviewView`, and `PublicationWorkflowView` in TypeScript. In `HugoPublishFlow`, show a persistent error block with `失败阶段`, `原因`, and `下一步`; provide `重新生成预览` when `retryable` is true. Keep the existing toast for transient request errors.

- [x] **Step 5: Run backend and focused frontend tests**

```bash
go test ./internal/app/bootstrap ./internal/transport/http ./internal/app/publication
cd web/app
npm test -- --run src/components/HugoPublishFlow.test.tsx
npm run typecheck
```

Expected: PASS, with the failed preview test exposing preflight diagnostics.

---

### Task 5: Full regression, embedded assets, and quality review

**Files:**
- Modify: `web/dist/**` (generated by the existing build)
- Modify: `docs/superpowers/specs/2026-08-03-publication-hub-flow-design.md` only if implementation decisions require clarification

- [x] **Step 1: Run the complete frontend suite and production build**

```bash
cd web/app
npm test -- --run
npm run typecheck
npm run lint
npm run build
```

Expected: all Vitest files pass, TypeScript and lint exit 0, and `web/dist/index.html` references the newly built asset.

- [x] **Step 2: Run the complete Go suite**

```bash
go test ./...
```

Expected: all packages pass.

- [x] **Step 3: Verify the user workflow against the running app**

Restart the local InkHub server so embedded assets are refreshed, then verify:

1. 内容库 → 文章 opens review center.
2. Review center has no inline Hugo directory selector.
3. After approval, Hugo, WeChat, and Xiaohongshu entries are independently available.
4. `/hugo`, `/wechat`, and `/xiaohongshu` each expose the other channel entries.
5. A failed Hugo preflight shows stage, reason, and retry action.

- [x] **Step 4: Review diff and status**

```bash
git diff --check
git status --short
git diff --stat
```

Confirm only the design, plan, frontend, backend, tests, and generated embedded assets changed; do not include runtime logs or unrelated user files.

- [x] **Step 5: Commit the completed implementation**

```bash
git add internal web/app web/dist docs/superpowers/plans/2026-08-03-publication-hub-flow.md
git commit -m "feat: separate review and publication channels"
```
