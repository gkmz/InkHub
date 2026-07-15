# 设置反馈与内容库分页实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让目录规则一眼可见、所有设置操作有明确反馈，并让内容库可以稳定加载全部文章。

**Architecture:** React 顶层 `ToastProvider` 统一管理操作反馈，业务组件通过 `useToast` 触发消息。文章列表使用后端 keyset cursor 分页，cursor 编解码与 SQL 查询边界独立测试，前端按页追加并在查询条件变化时重置。

**Tech Stack:** React、TypeScript、Vitest、Go、SQLite、URL-safe Base64

---

### Task 1: 全局操作提示组件

**Files:**
- Create: `web/app/src/components/ToastProvider.tsx`
- Create: `web/app/src/components/ToastProvider.test.tsx`
- Modify: `web/app/src/app.tsx`
- Modify: `web/app/src/styles/app.css`

- [ ] **Step 1: 写失败测试**

测试组件调用 `useToast().show({ kind: "info", message: "功能尚未开放" })` 后，页面出现 `role="status"` 的提示，并可点击关闭按钮移除。

- [ ] **Step 2: 运行测试确认 RED**

Run: `npm test -- --run src/components/ToastProvider.test.tsx`
Expected: FAIL，原因是 `ToastProvider` 尚不存在。

- [ ] **Step 3: 实现最小组件**

实现 `ToastProvider`、`useToast`、消息队列和关闭动作；每条消息使用稳定 ID，success/info 使用 status，error 使用 alert，自动关闭计时器在卸载时清理。

- [ ] **Step 4: 顶层装配并验证 GREEN**

在 `app.tsx` 的应用根部装配 Provider，运行同一测试确认通过。

### Task 2: 目录规则摘要和设置按钮反馈

**Files:**
- Modify: `web/app/src/components/ContentScopePicker.tsx`
- Modify: `web/app/src/components/ContentScopePicker.test.tsx`
- Modify: `web/app/src/components/SecretField.tsx`
- Modify: `web/app/src/pages/settings/SettingsPage.tsx`
- Modify: `web/app/src/pages/settings/settings.test.tsx`
- Modify: `web/app/src/styles/app.css`

- [ ] **Step 1: 写目录摘要失败测试**

传入 `contentRoots=["Areas"]` 和 `ignoredFolders=["Areas/私人记录"]`，断言“当前管理”“当前排除”区域直接展示两个路径；点击删除管理目录后，`onChange` 收到三个空规则中相应的有效组合。

- [ ] **Step 2: 写未开放操作失败测试**

点击“保存 AI 设置”“保存发布设置”和 Secret 删除图标，断言均显示具体的“尚未开放”全局提示，而不是静默无效果。

- [ ] **Step 3: 运行相关测试确认 RED**

Run: `npm test -- --run src/components/ContentScopePicker.test.tsx src/pages/settings/settings.test.tsx`
Expected: FAIL，原因是摘要与提示事件尚不存在。

- [ ] **Step 4: 实现摘要与事件**

为当前规则增加紧凑列表和删除图标；`SecretField` 接收 `onDelete`；设置页通过 `useToast` 为未开放操作显示 info 提示。

- [ ] **Step 5: 运行相关测试确认 GREEN**

重复 Step 3 命令，确认全部通过。

### Task 3: 后端稳定 cursor 分页

**Files:**
- Create: `internal/app/bootstrap/article_cursor.go`
- Create: `internal/app/bootstrap/article_cursor_test.go`
- Modify: `internal/app/bootstrap/runtime_api.go`
- Modify: `internal/app/bootstrap/publication_test.go`

- [ ] **Step 1: 写 cursor 编解码失败测试**

测试 `{ModifiedAt: "2026-07-15T10:00:00Z", ID: "article_1"}` 可往返，损坏 Base64、空字段和超过 1024 字符的 cursor 被拒绝。

- [ ] **Step 2: 写跨页失败测试**

插入 3 篇具有稳定时间和 ID 的文章，以 limit=2 查询第一页应返回 2 篇和 cursor，再用 cursor 查询只返回剩余 1 篇且没有 cursor。

- [ ] **Step 3: 运行测试确认 RED**

Run: `go test ./internal/app/bootstrap -run 'TestArticleCursor|TestListArticlesPaginates'`
Expected: FAIL，原因是 cursor 函数缺失且 `ListArticles` 忽略 cursor。

- [ ] **Step 4: 实现 cursor 与 keyset SQL**

使用 `base64.RawURLEncoding` 编码 JSON；查询按 `COALESCE(source_mtime, updated_at) DESC, id DESC` 排序，cursor 条件为时间更早或同时间 ID 更小，读取 `limit+1` 并用末项生成下一页 cursor。

- [ ] **Step 5: 运行后端测试确认 GREEN**

重复 Step 3 命令，并运行 `go test ./internal/transport/http ./internal/app/bootstrap`。

### Task 4: 内容库加载更多

**Files:**
- Modify: `web/app/src/pages/library/LibraryPage.tsx`
- Modify: `web/app/src/app.test.tsx`
- Modify: `web/app/src/styles/app.css`

- [ ] **Step 1: 写追加分页失败测试**

mock 第一页返回 2 篇和 `next_cursor`，第二页返回 1 篇；点击“加载更多”后断言 3 篇都保留、第二次请求携带 cursor，按钮在末页消失。

- [ ] **Step 2: 写分页失败保留测试**

第二页请求失败时断言第一页文章仍在，并出现 error Toast。

- [ ] **Step 3: 运行测试确认 RED**

Run: `npm test -- --run src/app.test.tsx`
Expected: FAIL，原因是页面未保存 cursor、没有加载更多按钮。

- [ ] **Step 4: 实现追加和重置**

保存 `nextCursor` 与 `loadingMore`；查询或状态变化重置列表；加载更多追加并按文章 ID 去重；失败时调用 Toast。

- [ ] **Step 5: 运行测试确认 GREEN**

重复 Step 3 命令确认通过。

### Task 5: Reflection、构建与整体回归

**Files:**
- Modify: `web/dist/index.html`
- Replace: `web/dist/assets/index-*.js`
- Replace if changed: `web/dist/assets/index-*.css`

- [ ] **Step 1: 前端全量验证**

Run: `npm test -- --run && npm run typecheck && npm run lint && npm run build`
Expected: 0 failures，生产资源生成到 `web/dist`。

- [ ] **Step 2: 后端全量验证**

Run: `go test ./... && go test -race ./... && go vet ./...`
Expected: 0 failures。

- [ ] **Step 3: Reflection 审查**

检查所有设置页按钮均有事件，Toast 不泄漏计时器，目录删除保持规则有效，cursor 不可伪造为 SQL，分页无重复且搜索变化不混入旧页，类目代码没有改动。

- [ ] **Step 4: 检查并提交**

运行 `git diff --check`、`git status --short`，只暂存本计划涉及文件，提交：

```bash
git commit -m "fix(ui): clarify settings actions and paginate library"
```
