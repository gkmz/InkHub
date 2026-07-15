# 文章 Series 选择器实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让文章审核页从 Hugo taxonomy 快照选择和创建 Series，并用单值 taxonomy 字段统一 Category 与 Series 行为。

**Architecture:** 新建无 API 依赖的 `SingleTaxonomyField` 负责单值 taxonomy 的候选、旧值和状态展示；`MetadataForm` 只维护文章草稿。`ArticlePage` 继续复用一个 taxonomy 快照，通用创建对话框按 kind 规划 Category 或 Series 文件变更。

**Tech Stack:** React 19、TypeScript、Vitest、Testing Library、现有 Taxonomy HTTP API

## Global Constraints

- 本阶段只处理 Category 和 Series，Tags 不进入单值字段抽象。
- Series 创建后只回填草稿，必须点击“保存到文章”才写回 Obsidian。
- taxonomy 失败不得阻止文章打开或清空旧值。
- 创建文件路径和内容只能由服务端 Taxonomy Provider 生成。
- 每个新增行为先确认失败测试，再写最小实现。
- 关键逻辑使用中文注释，公开组件和类型具有中文文档注释。
- 全部功能、reflection 和整体回归通过后只做一个聚合实现提交。

---

### Task 1: 单值 Taxonomy 字段抽取

**Files:**
- Create: `web/app/src/components/SingleTaxonomyField.tsx`
- Create: `web/app/src/components/SingleTaxonomyField.test.tsx`
- Modify: `web/app/src/components/MetadataForm.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: 字符串 value、`TaxonomyFieldOption[]`、`TaxonomyFieldState`
- Produces: `SingleTaxonomyField`、`TaxonomyFieldOption`、`TaxonomyFieldState`

- [ ] **Step 1: 写通用字段失败测试**

渲染：

```tsx
<SingleTaxonomyField
  label="Series"
  noun="系列"
  value="旧系列"
  options={[{ key: "course", name: "课程" }, { key: "course-copy", name: "课程" }]}
  state="ready"
  emptyLabel="无系列"
  canCreate
  onChange={change}
  onCreate={create}
/>
```

断言 combobox 保留“旧系列（博客中未发现）”，重复“课程”只出现一次，选择“课程”调用 `onChange("课程")`，点击“新建系列”调用 `onCreate`。

- [ ] **Step 2: 运行测试确认 RED**

Run: `npm test -- --run src/components/SingleTaxonomyField.test.tsx`

Expected: FAIL，组件文件尚不存在。

- [ ] **Step 3: 实现最小通用组件**

定义设计文档中的 `TaxonomyFieldState`、`TaxonomyFieldOption` 和 props。原生 select 依次展示空值、快照外当前值和按 name 去重候选；状态文案保持 Category 现有四种反馈；创建按钮使用 `Plus` 图标和 `新建${noun}` accessible name。

- [ ] **Step 4: 迁移 MetadataForm**

删除 `CategoryOption`、`CategoryState` 和 `uniqueCategories`。props 改为：

```ts
taxonomyState?: TaxonomyFieldState;
categoryOptions?: TaxonomyFieldOption[];
seriesOptions?: TaxonomyFieldOption[];
canCreateTaxonomy?: boolean;
onCreateTaxonomy?: (kind: "category" | "series", select: (name: string) => void) => void;
```

Category 使用 `noun="类目"`、`emptyLabel="未分类"`，Series 使用 `noun="系列"`、`emptyLabel="无系列"`。两个字段分别更新 draft，不改变 Tags。

- [ ] **Step 5: Category 回归和 GREEN 验证**

更新文章测试调用新 props，运行：

```bash
npm test -- --run src/components/SingleTaxonomyField.test.tsx src/pages/article/article-workflow.test.tsx
```

Expected: 全部 PASS，Category 现有选择、旧值和回填断言不变。

### Task 2: Series 创建与文章页闭环

**Files:**
- Modify: `web/app/src/pages/taxonomy/CreateTaxonomyTermDialog.tsx`
- Modify: `web/app/src/pages/taxonomy/taxonomy-page.test.tsx`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`

**Interfaces:**
- Consumes: `TaxonomyOverview`、`CreateTaxonomyTermDialog`、Task 1 通用字段 props
- Produces: `kind="series"` 创建命令、Series 快照候选和创建后 draft 回填

- [ ] **Step 1: 写对话框参数化失败测试**

新增对话框级或文章集成测试：用 `kind="series"`、`noun="系列"` 打开后出现“系列名称”“系列说明”“确认创建系列”；preview 请求 JSON 的 `kind` 必须为 `series`，成功 Toast 为“系列已创建”。保留 taxonomy 页面默认 Category 测试。

- [ ] **Step 2: 写文章 Series 失败测试**

taxonomy fixture 增加 `{ kind: "series", key: "go-course", name: "Go 课程" }`。断言 Series 是 combobox 且可选择；点击“新建系列”创建“AI 系列”，Provider 返回 `content/series/ai/_index.md` 后确认，断言只更新 Series draft、Category 保持原值，且没有 metadata PUT。

- [ ] **Step 3: 运行相关测试确认 RED**

Run: `npm test -- --run src/pages/article/article-workflow.test.tsx src/pages/taxonomy/taxonomy-page.test.tsx`

Expected: FAIL，Series 仍是 input，对话框仍写死 Category。

- [ ] **Step 4: 参数化创建对话框**

新增可选 `kind`、`noun`，默认 `category`、`类目`。所有标签、按钮、进度、成功 Toast 和 command.kind 使用配置；默认调用方行为不变。

- [ ] **Step 5: 接通 ArticlePage**

从同一快照筛选 series options；把创建状态改为 `{ kind, select } | null`；向 `MetadataForm` 传 category/series options 与统一创建回调。对话框按 kind 设置 noun，成功后只调用对应 select。

- [ ] **Step 6: 运行相关测试确认 GREEN**

重复 Step 3 命令，并运行 `npm run typecheck`。Expected: 全部 PASS。

### Task 3: Reflection、文档与整体回归

**Files:**
- Modify: `docs/design/interactions.md`
- Modify: `.codex/HANDOFF.md`
- Replace: `web/dist/assets/index-*.js`
- Replace if changed: `web/dist/assets/index-*.css`
- Modify: `web/dist/index.html`

**Interfaces:**
- Consumes: Task 1-2 完整行为
- Produces: 嵌入式生产前端和 Tags 阶段 handoff

- [ ] **Step 1: Reflection 审查**

检查通用组件没有 API/草稿职责，Category 行为无回归，Series 旧值不丢失，重复 term 去重，两个创建回调不串字段，创建失败保留对话框，source changed 仍禁用保存，Tags 代码未被抽象污染，移动端两个 select 和按钮不溢出。

- [ ] **Step 2: 更新中文文档**

交互文档明确 Category/Series 共用单值 taxonomy 行为和创建后保存语义；handoff 下一阶段指向 Tags 多选与准入治理专项设计。

- [ ] **Step 3: 前端全量验证与构建**

Run: `npm test -- --run && npm run typecheck && npm run lint && npm run build`

Expected: 0 failures，生产资源写入 `web/dist`。

- [ ] **Step 4: 后端回归**

Run: `go test ./... && go vet ./...`

Expected: 0 failures。

- [ ] **Step 5: 提交前审查和提交**

运行 `git diff --check`、`git status --short`、`git diff --stat`，只暂存本阶段文件并提交：

```bash
git commit -m "feat(editorial): select Hugo series"
```
