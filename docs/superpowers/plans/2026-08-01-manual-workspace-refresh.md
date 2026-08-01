# 手动刷新工作区实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为工作台和内容库提供重新扫描工作区并刷新当前数据的手动入口，并将最新已就绪文章置于工作台首组。

**Architecture:** 在运行期 HTTP Handler 增加受同源校验保护的同步扫描端点，复用现有 Source 构建和 `workspace.ScanWorkspace` 索引逻辑。React 页面各自触发扫描请求，成功后递增已有 reload key 或重新读取工作台数据；排序继续由后端 SQL 和 DashboardView 分组顺序提供。

**Tech Stack:** Go、SQLite、net/http、React、TypeScript、Vitest、Lucide。

## Global Constraints

- 缺少或非法 `publish.status` 仍按草稿处理。
- 关键代码使用中文注释，公开方法保留文档注释。
- 不修改用户已有的 E2E 文件改动。
- 不引入文件监听器、定时轮询或新的数据库表。

---

### Task 1: 增加工作区刷新 API

**Files:**
- Modify: `internal/transport/http/runtime.go`
- Test: `internal/transport/http/runtime_test.go`

**Interfaces:**
- Produces `POST /api/v1/workspace/refresh` with `{indexed, failed}` response.

- [ ] **Step 1: Write the failing HTTP test**

在 `runtime_test.go` 增加测试：创建一个带 `publish.status: draft` 的 Markdown，初始化工作区后将文件改成 `publish.status: ready`，调用 `POST /api/v1/workspace/refresh`，断言 `200`、`indexed` 大于 0，并从数据库断言文章 `content_stage` 为 `ready`。

- [ ] **Step 2: Run the focused test**

Run `go test ./internal/transport/http -run TestRuntimeHandlerRefreshesWorkspace -count=1`。
Expected: FAIL，因为刷新路由尚不存在。

- [ ] **Step 3: Implement the endpoint**

在 `ServeHTTP` 增加 `POST /api/v1/workspace/refresh` 分支；新增带中文文档注释的 `refreshWorkspace` 方法，读取最近工作区的 `sources.id` 和 `workspace_id`，调用现有 `h.scanWorkspace(request.Context(), sourceID, workspaceID, nil)`，错误使用 `mapError`，成功返回 `map[string]int{"indexed": report.Indexed, "failed": report.Failed}`。端点必须经过 `validateWriteRequest`。

- [ ] **Step 4: Run the focused test**

Run `go test ./internal/transport/http -run TestRuntimeHandlerRefreshesWorkspace -count=1`。
Expected: PASS。

### Task 2: 调整工作台分组顺序

**Files:**
- Create: `web/app/src/pages/workspace/dashboard-page.test.tsx`
- Modify: `web/app/src/pages/workspace/DashboardPage.tsx`
- Modify: `internal/app/bootstrap/dashboard_api.go`
- Test: `web/app/src/pages/workspace/dashboard-page.test.tsx`
- Test: `internal/app/bootstrap/dashboard_api_test.go`

**Interfaces:**
- Keeps `DashboardView` field names unchanged; only response group ordering changes.

- [ ] **Step 1: Write the failing ordering test**

断言 Dashboard 页面渲染的第一个 `section` 标题为“最新已就绪”，并断言后端 DashboardView 序列化或页面消费顺序不会把草稿放入任何分组。

- [ ] **Step 2: Run the focused frontend test**

Run `cd web/app && npm test -- --run src/pages/workspace/dashboard-page.test.tsx`。
Expected: FAIL，因为当前页面 sections 将“处理失败”放在第一位。

- [ ] **Step 3: Implement the order change**

将前端 `sections` 数组改为 `latest_ready`、`failed`、`changed`、`needs_review`、`ready_to_publish`、`recently_handled`；同步检查后端分组构造顺序及测试断言，使服务端和页面保持同一产品顺序。不得在页面新增二次排序。

- [ ] **Step 4: Run focused tests**

Run `go test ./internal/app/bootstrap -run TestDashboard -count=1` and `cd web/app && npm test -- --run src/pages/workspace/dashboard-page.test.tsx`。
Expected: PASS。

### Task 3: 接入前端刷新交互

**Files:**
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/pages/workspace/DashboardPage.tsx`
- Modify: `web/app/src/pages/library/LibraryPage.tsx`
- Modify: `web/app/src/styles/app.css`
- Test: `web/app/src/pages/workspace/dashboard-page.test.tsx`
- Test: `web/app/src/pages/library/library-page.test.tsx`

**Interfaces:**
- Produces `refreshWorkspace(): Promise<{ indexed: number; failed: number }>`.

- [ ] **Step 1: Write failing UI tests**

在两个页面测试中 mock `/workspace/refresh`，点击名称为“刷新工作区”的按钮，断言请求方法为 `POST`，按钮在 Promise 未完成期间显示“正在扫描…”并禁用，完成后页面重新请求 dashboard 或 articles 并出现“内容库已更新”提示。

- [ ] **Step 2: Run focused frontend tests**

Run `cd web/app && npm test -- --run src/pages/workspace/dashboard-page.test.tsx src/pages/library/library-page.test.tsx`。
Expected: FAIL，因为客户端和按钮尚不存在。

- [ ] **Step 3: Implement the client and pages**

在 `client.ts` 增加带文档注释的 `refreshWorkspace` POST 方法。工作台增加刷新按钮、`refreshing` 状态和 `reloadKey`，请求成功后重新调用 `getDashboard` 并用 Toast 报告索引数量；内容库复用已有 `reloadKey` 触发请求，保留现有筛选状态，并在成功后提示。按钮使用 `RefreshCw` 图标，扫描失败时提示错误且不清空原数据。为刷新按钮增加稳定的 flex 样式和移动端布局规则。

- [ ] **Step 4: Run focused frontend tests**

Run `cd web/app && npm test -- --run src/pages/workspace/dashboard-page.test.tsx src/pages/library/library-page.test.tsx`。
Expected: PASS。

### Task 4: 全量验证和文档更新

**Files:**
- Modify: `README.md` or relevant product documentation only if the manual refresh behavior is not already described.

- [ ] **Step 1: Run backend verification**

Run `go test -race ./...`, `go vet ./...`, and `go build ./cmd/inkhub`。
Expected: all commands pass.

- [ ] **Step 2: Run frontend verification**

Run `cd web/app && npm run typecheck`, `npm run lint`, and `npm test -- --run`。
Expected: all commands pass.

- [ ] **Step 3: Review repository state**

Run `git diff --check`, `git diff --stat`, and `git status --short`; verify only the refresh implementation, plan/spec documentation, and pre-existing user E2E changes are present.
