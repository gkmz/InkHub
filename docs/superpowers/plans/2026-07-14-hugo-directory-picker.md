# Hugo 目录选择实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在首次引导中通过原生目录选择器填写 Hugo 项目根目录。

**Architecture:** 扩展现有目录选择 API，以受限用途决定原生对话框标题；SetupPage 复用相同路径控件完成 Vault 与 Hugo 选择。后端保持同源边界并验证用途白名单。

**Tech Stack:** Go、React、TypeScript、Vitest、Testing Library

## Global Constraints

- 关键代码和公开方法使用中文注释。
- 不引入新依赖。
- 使用 TDD，先验证失败，再实现并执行完整回归。

---

### Task 1: 目录选择用途契约与 Hugo 交互

**Files:**
- Modify: `internal/transport/http/runtime.go`
- Test: `internal/transport/http/runtime_test.go`
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/pages/setup/SetupPage.tsx`
- Test: `web/app/src/app.test.tsx`

**Interfaces:**
- Consumes: `dialog.DirectoryPicker.Pick(context.Context, string) (string, error)`
- Produces: `pickDirectory(purpose: "vault" | "hugo"): Promise<{ path: string }>`

- [x] **Step 1: Write failing tests**

验证 Hugo 文件夹按钮发送 `{ "purpose": "hugo" }` 并填写返回路径；验证后端 `hugo` 用途使用“选择 Hugo 项目根目录”，未知用途返回 `request.invalid`。

- [x] **Step 2: Verify tests fail**

Run: `go test ./internal/transport/http -run TestRuntimeHandlerPicksDirectory -count=1 && npm --prefix web/app test -- --run src/app.test.tsx`

Expected: FAIL，因为 API 尚无用途参数且 Hugo 没有选择按钮。

- [x] **Step 3: Implement minimal behavior**

将前端 API 改为发送用途 JSON；为 Hugo 输入框添加文件夹按钮和独立错误状态；后端解析严格 JSON 白名单并映射固定标题。

- [x] **Step 4: Verify focused and full regression**

Run: `go test ./... && go test -race ./internal/transport/http && go vet ./... && npm --prefix web/app test -- --run && npm --prefix web/app run typecheck && npm --prefix web/app run build`

Expected: 所有命令退出码为 0。

- [x] **Step 5: Commit**

```bash
git add internal/transport/http/runtime.go internal/transport/http/runtime_test.go web/app/src/api/client.ts web/app/src/pages/setup/SetupPage.tsx web/app/src/app.test.tsx docs/superpowers
git commit -m "feat(setup): add Hugo directory picker"
```
