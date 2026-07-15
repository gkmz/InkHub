# 文章 Category 选择器实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让文章审核页从 Hugo taxonomy 快照选择 Category，并在同页安全创建新类目后回填文章草稿。

**Architecture:** `ArticlePage` 统一并行读取文章与 taxonomy，向纯表单组件注入候选和创建能力。`MetadataForm` 只维护草稿，现有 `CreateTaxonomyTermDialog` 继续负责 Provider 预览和 revision 约束写入。

**Tech Stack:** React 19、TypeScript、Vitest、Testing Library、现有 Go HTTP API

## Global Constraints

- 本阶段只改造 Category，不实现 Series 和 Tags 治理。
- 新类目创建后只回填草稿，必须点击“保存到文章”才写回 Obsidian。
- taxonomy 失败不得阻止文章打开或清空旧 Category。
- 每个行为先确认失败测试，再实现最小代码。
- 关键逻辑使用中文注释，公开组件和方法使用中文文档注释。
- 全部功能完成并 reflection、整体回归通过后只做一个聚合提交。

---

### Task 1: Category 草稿选择控件

**Files:**
- Modify: `web/app/src/components/MetadataForm.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: `ArticleMetadata.category: string`
- Produces: `CategoryOption`、`CategoryState` 和 `MetadataFormProps.onCreateCategory(select)`

- [ ] **Step 1: 写选择与旧值兼容失败测试**

在 `article-workflow.test.tsx` 传入：

```tsx
<MetadataForm
  value={metadata}
  sourceChanged={false}
  categoryState="ready"
  categoryOptions={[{ key: "engineering", name: "工程实践" }, { key: "product", name: "产品" }]}
  onSave={save}
/>
```

断言 Category 是 combobox，选择“产品”后变更摘要出现 `Category：工程实践 → 产品`；当 `value.category="旧分类"` 时选项显示“旧分类（博客中未发现）”且值仍为“旧分类”。

- [ ] **Step 2: 写新建回填失败测试**

传入 `onCreateCategory={(select) => select("AI")}`，点击“新建类目”后断言变更摘要为 `Category：工程实践 → AI`，且 `onSave` 尚未调用。

- [ ] **Step 3: 运行测试确认 RED**

Run: `npm test -- --run src/pages/article/article-workflow.test.tsx`

Expected: FAIL，Category 仍为自由输入且不存在新建回调。

- [ ] **Step 4: 实现最小控件**

新增并导出：

```ts
export interface CategoryOption { key: string; name: string }
export type CategoryState = "loading" | "ready" | "unavailable";
```

扩展 `MetadataFormProps`，按名称去重 category，保留不在快照中的当前值。用原生 `select` 替换 Category input，并在可创建时显示带 `Plus` 图标的按钮；按钮调用 `onCreateCategory((name) => update("category", name))`。

- [ ] **Step 5: 样式与 GREEN 验证**

为 select、状态文案和同一行图标按钮增加紧凑响应式样式。重复 Step 3 命令，Expected: PASS。

### Task 2: 文章页 taxonomy 数据流与创建闭环

**Files:**
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/pages/taxonomy/CreateTaxonomyTermDialog.tsx`

**Interfaces:**
- Consumes: `getTaxonomyOverview()`、`TaxonomyOverview`、`CreateTaxonomyTermDialog`
- Produces: 文章页 category options、显式加载状态、创建后 snapshot 更新与 draft 回填

- [ ] **Step 1: 写页面级加载失败测试**

mock `/articles/:id` 成功、`/taxonomy` 返回 category 快照，渲染 `ArticlePage` 后断言 Category combobox 包含快照类目。另测 `/taxonomy` 失败时文章标题和当前 Category 仍出现，并显示“博客类目暂不可用”。

- [ ] **Step 2: 写创建闭环失败测试**

依次 mock taxonomy preview 和 apply。点击元数据区“新建类目”，填写“AI”，预览并确认后断言 Category 值为 `AI`，且尚未出现 `PUT /articles/:id/metadata`；点击“保存到文章”后才出现 PUT。

- [ ] **Step 3: 运行测试确认 RED**

Run: `npm test -- --run src/pages/article/article-workflow.test.tsx`

Expected: FAIL，ArticlePage 尚未读取 taxonomy，也未挂载创建对话框。

- [ ] **Step 4: 实现并行加载和显式降级**

`ArticlePage` 增加 `taxonomy`、`taxonomyState`、`categorySelection` 状态。文章与 taxonomy 请求互不阻塞；请求失败设置 `unavailable`。筛选 category term 后传给 `MetadataForm`，创建能力只依赖 `readonly/provider_id/revision`。

- [ ] **Step 5: 扩展对话框回调并闭环**

`CreateTaxonomyTermDialog` 增加 `onCreated?: (name: string) => void`。apply 成功后依次调用 `onApplied(next)` 和 `onCreated(name.trim())`。文章页保存一次性选择回调，创建成功后用它更新表单草稿并清空回调。

- [ ] **Step 6: 运行相关测试确认 GREEN**

重复 Step 3 命令，并运行 `npm test -- --run src/pages/taxonomy/taxonomy-page.test.tsx`，Expected: 全部 PASS。

### Task 3: Reflection、文档与整体回归

**Files:**
- Modify: `docs/design/interactions.md`
- Modify: `.codex/HANDOFF.md`
- Replace: `web/dist/assets/index-*.js`
- Replace if changed: `web/dist/assets/index-*.css`
- Modify: `web/dist/index.html`

**Interfaces:**
- Consumes: Task 1-2 完整行为
- Produces: 可嵌入 Go 二进制的生产前端和下一阶段 handoff

- [ ] **Step 1: 功能点 reflection**

检查旧值不会丢失、taxonomy 失败不会阻塞文章、新建不会提前保存文章、source changed 仍阻止写回、重复 term 不产生重复 option、创建失败保留草稿和对话框、移动端按钮和 select 不溢出。

- [ ] **Step 2: 更新中文文档**

在交互文档明确 Category 选择器的旧值兼容和同页创建语义；更新 handoff，将下一阶段指向 Series 或 Tags 设计，不提前承诺实现顺序。

- [ ] **Step 3: 前端全量验证与构建**

Run: `npm test -- --run && npm run typecheck && npm run lint && npm run build`

Expected: 0 failures，生产资源写入 `web/dist`。

- [ ] **Step 4: 后端回归**

Run: `go test ./... && go vet ./...`

Expected: 0 failures。

- [ ] **Step 5: 提交前审查和提交**

运行 `git diff --check`、`git status --short`、`git diff --stat`，确认仅包含本计划文件。提交：

```bash
git commit -m "feat(editorial): select Hugo categories"
```
