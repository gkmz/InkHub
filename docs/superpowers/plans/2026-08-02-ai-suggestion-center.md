# AI 建议中心 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将文章页的 AI Tag 建议升级为覆盖全部元数据字段、可保留历史版本、只修改页面草稿且可查询历史的 AI 建议中心。

**Architecture:** 每次生成使用唯一建议 ID 插入 `ai_suggestions`，Repository 提供按文章查询历史和按 ID 查询详情；HTTP 只返回脱敏的类型化建议值。React 端按字段分组展示最新版本，采用/忽略只通过受控回调更新 `MetadataForm` 草稿，历史版本只读，最终仍复用现有 `saveMetadata` 写回流程。

**Tech Stack:** Go 1.24+、SQLite、现有 HTTP Runtime、React 18、TypeScript、Vitest、Playwright。

## Global Constraints

- AI 建议永远只是草稿输入，只有用户点击“保存到文章”才写回 Markdown。
- 不新增向量数据库、embedding、自动保存或自动修改 taxonomy。
- 所有数据库查询必须带 workspace/article 归属约束；HTTP 不返回 Secret、原始 Provider 响应或完整提示词。
- 关键代码和公开方法必须有明确中文注释/文档注释；提交使用 Conventional Commits。
- 保留用户现有未提交改动，不把无关文件加入提交。

---

### Task 1: 持久化多版本建议并补齐 Repository 查询

**Files:**
- Modify: `internal/transport/http/article_suggestions.go`
- Modify: `internal/storage/sqlite/repository/suggestion.go`
- Modify: `internal/storage/sqlite/repository/suggestion_test.go`
- Modify: `internal/transport/http/article_suggestions_test.go`

**Interfaces:**
- Produces `SuggestionRepository.ListByArticle(ctx, workspaceID, articleID, limit) ([]SuggestionSet, error)`，按 `created_at DESC, id DESC` 返回完整集合。
- Produces `SuggestionRepository.FindByArticleID(ctx, workspaceID, articleID, suggestionID) (SuggestionSet, error)`，找不到或归属不匹配返回现有 not-found 语义。
- `generateArticleSuggestions` 改用每次请求唯一的 suggestion ID，现有 `SuggestionRepository.Save` 的 upsert 仅用于同一版本状态更新。

- [ ] **Step 1: Write the failing test**

在 Repository 测试中保存 `suggestion_1`、`suggestion_2` 两个同文章同内容哈希的集合，调用 `ListByArticle`，断言返回两个集合且顺序为 `suggestion_2,suggestion_1`。在 HTTP 测试中连续 POST 两次，断言数据库记录数为 2，第二次响应包含第二个 Tag。

- [ ] **Step 2: Run RED**

```bash
go test ./internal/storage/sqlite/repository ./internal/transport/http -run 'TestSuggestionRepositoryHistory|TestArticleSuggestionsKeepHistory' -count=1
```

Expected: FAIL，因为 Repository 没有历史查询方法，且生成 ID 仍由文章 ID 与内容哈希稳定计算。

- [ ] **Step 3: Implement**

在 `suggestion.go` 增加带中文文档注释的历史/详情查询方法。历史 SQL 必须约束 `workspace_id=? AND article_id=?`，按 `created_at DESC,id DESC` 排序并使用受限 limit；详情 SQL 同时约束 workspace、article 和 suggestion ID。沿用 `scanSuggestion` 兼容旧 JSON。

在 `article_suggestions.go` 使用 `crypto/rand` 生成 16 字节随机值并编码为 `suggestion_<hex>`；随机失败返回错误，不回退稳定 ID。

- [ ] **Step 4: Run GREEN and commit**

```bash
gofmt -w internal/transport/http/article_suggestions.go internal/storage/sqlite/repository/suggestion.go internal/storage/sqlite/repository/suggestion_test.go internal/transport/http/article_suggestions_test.go
go test ./internal/storage/sqlite/repository ./internal/transport/http -run 'TestSuggestionRepositoryHistory|TestArticleSuggestionsKeepHistory' -count=1
git add internal/transport/http/article_suggestions.go internal/storage/sqlite/repository/suggestion.go internal/storage/sqlite/repository/suggestion_test.go internal/transport/http/article_suggestions_test.go
git commit -m "feat(ai): preserve suggestion generations"
```

### Task 2: 增加历史 API 和类型化建议 DTO

**Files:**
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/article_suggestions.go`
- Modify: `internal/transport/http/router.go`
- Modify: `internal/transport/http/runtime_test.go`
- Modify: `internal/transport/http/article_suggestions_test.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`

**Interfaces:**
- `GET /api/v1/articles/{id}/suggestions?limit=20` 返回 `{ items: SuggestionHistoryItem[], latest_id?: string }`。
- `GET /api/v1/articles/{id}/suggestions/{suggestionID}` 返回指定版本的完整建议。
- `AISuggestion.value` 类型为 `string | string[]`；Tags 拆成逐项字符串，Keywords 保留数组。

- [ ] **Step 1: Write failing tests**

覆盖历史列表的 workspace/article 约束、时间倒序和 limit；指定错误 suggestion ID 返回 404；Keywords 返回数组，Tags 返回逐项字符串。

- [ ] **Step 2: Run RED**

```bash
go test ./internal/transport/http -run 'TestSuggestionHistory|TestSuggestionDetailTypedValues' -count=1
```

Expected: FAIL，因为 runtime 尚未分发 GET suggestions 路由，且 DTO 当前只支持字符串。

- [ ] **Step 3: Implement DTO and routes**

新增历史摘要 DTO，至少包含 `id`、`generated_at`、`model`、`input_content_hash`、`state`、`suggestion_count`、`current`。详情 DTO 返回同一版本的建议项、生成时间和 `suggestions_stale`。

字符串值填充现有 `name`，数组值填充 `value`；保留 `reason/new_term/usage_count/accepted`，无法解析的单项跳过。把 `/suggestions/{suggestionID}` GET 路由放在通用 article detail 路由之前，limit 只接受 1 到 100。

- [ ] **Step 4: Add client methods and verify**

在 `web/app/src/api/client.ts` 添加 `getSuggestionHistory(articleID, signal?)`、`getSuggestionVersion(articleID, suggestionID, signal?)`，两者只读 GET；文章详情增加最新建议版本 ID/生成时间字段。

```bash
gofmt -w internal/transport/http/runtime.go internal/transport/http/article_suggestions.go internal/transport/http/router.go internal/transport/http/runtime_test.go internal/transport/http/article_suggestions_test.go
go test ./internal/transport/http -run 'TestSuggestionHistory|TestSuggestionDetailTypedValues' -count=1
cd web/app && npm run typecheck
git add internal/transport/http web/app/src/api/types.ts web/app/src/api/client.ts
git commit -m "feat(ai): add suggestion history api"
```

### Task 3: 扩展元数据草稿桥接到所有字段

**Files:**
- Modify: `web/app/src/components/MetadataForm.tsx`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/api/types.ts`

**Interfaces:**
- `MetadataForm.externalSuggestion.value` 为 `string | string[]`。
- `tags` 建议追加去重，`keywords` 建议替换数组，其他字段替换字符串。
- 采用建议不调用 `saveMetadata`。

- [ ] **Step 1: Write failing tests**

增加测试：采用 Description 后输入框变为 AI 描述且 save mock 未调用；Keywords 数组替换当前数组；Tags 大小写不敏感去重追加。

- [ ] **Step 2: Run RED**

```bash
cd web/app && npm test -- --run src/pages/article/article-workflow.test.tsx
```

Expected: FAIL，因为 `externalSuggestion.value` 当前只接受字符串且 Keywords 不按数组处理。

- [ ] **Step 3: Implement draft adapter**

在 MetadataForm effect 中使用以下字段规则，保留现有差异摘要和保存按钮：

```ts
if (field === "tags" && typeof value === "string") {
  const exists = current.tags.some((tag) => normalize(tag) === normalize(value));
  return exists ? current : { ...current, tags: [...current.tags, value] };
}
if (field === "keywords" && Array.isArray(value)) {
  return { ...current, keywords: value };
}
return typeof value === "string" ? { ...current, [field]: value } as ArticleMetadata : current;
```

- [ ] **Step 4: Run GREEN and commit**

```bash
cd web/app && npm test -- --run src/pages/article/article-workflow.test.tsx && npm run typecheck
git add web/app/src/components/MetadataForm.tsx web/app/src/pages/article/ArticlePage.tsx web/app/src/pages/article/article-workflow.test.tsx web/app/src/api/types.ts
git commit -m "feat(ai): apply suggestions to metadata draft"
```

### Task 4: 实现 AI 建议中心字段分组和历史查看

**Files:**
- Modify: `web/app/src/components/AISuggestions.tsx`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/styles/app.css`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Create: `web/app/src/components/SuggestionHistory.tsx`
- Create: `web/app/src/components/SuggestionHistory.test.tsx`

**Interfaces:**
- `AISuggestions` 接收所有字段建议、stale、生成状态、`onAccept(suggestion)`、`onGenerate()` 和 `onOpenHistory()`。
- `SuggestionHistory` 接收历史摘要、加载状态和 `onSelect(id)`；详情只读。

- [ ] **Step 1: Write failing component tests**

覆盖标题“AI 建议中心”、六个字段分组、单项采用/忽略、组级采用全部/忽略全部、采用后“已加入草稿”、stale 禁用采用、历史项调用 onSelect 且不渲染采用按钮。

- [ ] **Step 2: Run RED**

```bash
cd web/app && npm test -- --run src/pages/article/article-workflow.test.tsx src/components/SuggestionHistory.test.tsx
```

Expected: FAIL，因为当前组件只渲染 Tag。

- [ ] **Step 3: Implement grouped center**

按固定字段顺序分组；数组值使用 `value` 展示；本地 `acceptedIDs`/ `ignoredIDs` 只影响当前渲染，建议版本 ID 变化时清空本地状态。组级按钮复用 `onAccept`，Tags 仍逐项追加。忽略列表提供“显示已忽略”切换，所有按钮带字段和展示值的 aria-label。

- [ ] **Step 4: Implement history panel**

ArticlePage 按需加载历史，避免打开文章就额外请求；选择历史版本后加载详情并显示只读内容。当前版本标注“当前”，内容哈希不匹配标注“内容已变化”，历史版本不显示采用按钮。

- [ ] **Step 5: Style and verify**

沿用现有 `.tool-section`、`.suggestion` token，新增字段分组、数组预览和历史列表样式，窄屏不能横向滚动。

```bash
cd web/app
npm test -- --run src/pages/article/article-workflow.test.tsx src/components/SuggestionHistory.test.tsx
npm run typecheck
npm run lint
git add web/app/src/components/AISuggestions.tsx web/app/src/components/SuggestionHistory.tsx web/app/src/components/SuggestionHistory.test.tsx web/app/src/pages/article/ArticlePage.tsx web/app/src/pages/article/article-workflow.test.tsx web/app/src/styles/app.css
git commit -m "feat(ai): build suggestion center"
```

### Task 5: 文档、端到端边界和整体回归

**Files:**
- Modify: `internal/transport/http/runtime_test.go`
- Modify: `internal/transport/http/article_suggestions_test.go`
- Modify: `web/app/src/app.test.tsx`
- Modify: `web/app/e2e/workflows.spec.ts`
- Modify: `docs/PRD.md`
- Modify: `docs/design/interactions.md`
- Generated: `web/dist/*`

**Interfaces:**
- 文章详情返回最新建议版本 ID/生成时间；历史接口与详情接口在同源工作区内可互相校验。
- 生成失败、历史读取失败和建议过期时仍可编辑文章元数据。

- [ ] **Step 1: Add boundary tests**

后端覆盖旧内容哈希详情的 `suggestions_stale=true\)、生成失败保留旧版本、跨 workspace suggestion ID 404。前端覆盖生成失败旧建议仍在、重复点击只发一个请求、采用后点击“保存到文章”才发 PUT metadata、历史详情只读。

- [ ] **Step 2: Update product copy**

将 `docs/PRD.md`、`docs/design/interactions.md` 中“AI Tag 建议”改为“AI 建议中心”，写明支持字段、历史查询、草稿采用和手动保存边界；保留 AI 不自动覆盖文章原则。

- [ ] **Step 3: Run complete verification**

```bash
go test ./...
go vet ./...
cd web/app
npm test -- --run
npm run typecheck
npm run lint
npm run build
npx playwright test
```

Expected: 全部通过；Playwright 覆盖桌面和移动视口，文章页无横向溢出，建议中心和历史入口可访问。

- [ ] **Step 4: Review and commit production assets**

检查 `git diff --check`、`git status --short`，只提交本功能与用户明确修改；确认 `web/dist` 仅包含本次构建产物。

```bash
git add internal/transport/http web/app/src docs/PRD.md docs/design/interactions.md web/dist
git commit -m "feat(ai): complete suggestion center workflow"
```


