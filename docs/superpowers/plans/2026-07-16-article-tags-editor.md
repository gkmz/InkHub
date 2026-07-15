# 文章 Tags 多选与 AI 建议实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将文章 Tags 改为基于 Hugo taxonomy 持久化快照的可创建多选编辑器，并打通可逐项采用的 AI Tag 建议运行链路。

**Architecture:** `TagMultiSelect` 只处理无 API 依赖的多选交互，`MetadataForm` 持有草稿，`ArticlePage` 负责 taxonomy 与 AI 请求。后端复用现有 Taxonomy Service、AI Provider、Editorial Suggestion 和 SQLite Repository；HTTP 只做运行时依赖装配和安全视图转换。

**Tech Stack:** Go、SQLite、React、TypeScript、Vitest、Testing Library、Vite。

## Global Constraints

- 所有新增行为必须先写失败测试并确认 RED，再写最小实现。
- 关键代码和公开方法使用中文注释。
- 新 Tag 不做准入审批，不创建 Hugo term page。
- Tag 数量 3 至 6 只提示，不阻止保存。
- AI 不得提供或覆盖 Tag 使用次数；使用次数只来自 SQLite taxonomy 快照。
- 连续开发模式下功能点之间不提交；全部回归通过后使用一个 Conventional Commit 聚合提交。

---

### Task 1: Tag 规范化规则

**Files:**
- Modify: `internal/domain/taxonomy/taxonomy.go`
- Test: `internal/domain/taxonomy/taxonomy_test.go`
- Modify: `docs/PRD.md`
- Modify: `docs/design/interactions.md`

**Interfaces:**
- Produces: `NormalizeTags(tags []string, canonical map[string]string) []string`
- Preserves: `ValidateTags(tags []string, rules Rules) ([]string, error)` 作为发布检查入口，但不再通过 `Allowed` 拒绝新 Tag。

- [ ] **Step 1: 写失败测试**

覆盖空值删除、大小写不敏感去重、命中标准显示名称，以及未知 Tag 可以通过校验。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/domain/taxonomy -run 'TestNormalizeTags|TestValidateTagsAllowsNewTerms'`

Expected: FAIL，原因是 `NormalizeTags` 不存在且旧 `Allowed` 仍拒绝新 Tag。

- [ ] **Step 3: 实现最小规则**

`NormalizeTags` 使用 `strings.ToLower(strings.TrimSpace(value))` 作为比较 key；空 key 跳过，命中 `canonical[key]` 时写入标准名称，否则保留用户修剪后的名称。`ValidateTags` 复用规范化与数量检查，不再读取准入白名单。

- [ ] **Step 4: 更新产品文档**

删除新 Tag 审批、低频豁免和白名单阻断描述，改为已有 Tag 优先、用户可创建、AI 逐项建议和数量软提示。

- [ ] **Step 5: 验证 GREEN**

Run: `go test ./internal/domain/taxonomy`

Expected: PASS。

---

### Task 2: 无 API 依赖的 TagMultiSelect

**Files:**
- Create: `web/app/src/components/TagMultiSelect.tsx`
- Create: `web/app/src/components/TagMultiSelect.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: `TaxonomyFieldState`
- Produces: `TagOption`、`TagMultiSelectProps`、`TagMultiSelect`

```ts
export interface TagOption {
  key: string;
  name: string;
  usageCount: number;
}

export interface TagMultiSelectProps {
  value: string[];
  options: TagOption[];
  state: TaxonomyFieldState;
  onChange: (value: string[]) => void;
}
```

- [ ] **Step 1: 写基础渲染失败测试**

断言已选项、快照外标记、候选文章数量、已选候选隐藏和 3 至 6 个软提示。

- [ ] **Step 2: 确认 RED**

Run: `npm test -- --run src/components/TagMultiSelect.test.tsx`

Expected: FAIL，原因是组件不存在。

- [ ] **Step 3: 实现基础渲染与鼠标交互**

按 name/key 大小写不敏感匹配，候选使用快照标准名称；删除、选择和创建均通过 `onChange` 返回新数组，不直接修改输入值。

- [ ] **Step 4: 写键盘行为失败测试**

覆盖上下键高亮、回车选择/创建、Esc 关闭和空输入退格删除最后一项。

- [ ] **Step 5: 确认 RED 并实现键盘行为**

Run: `npm test -- --run src/components/TagMultiSelect.test.tsx`

Expected: 先因键盘行为缺失 FAIL；实现后 PASS。

- [ ] **Step 6: 完成状态反馈与样式**

`loading`、`not_enabled`、`unavailable`、`ready` 使用标签语义中文文案。控件具有稳定尺寸、可见焦点、列表滚动边界和移动端单列布局。

- [ ] **Step 7: 验证组件**

Run: `npm test -- --run src/components/TagMultiSelect.test.tsx && npm run typecheck && npm run lint`

Expected: PASS。

---

### Task 3: 文章草稿集成

**Files:**
- Modify: `web/app/src/components/MetadataForm.tsx`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`

**Interfaces:**
- `MetadataForm` 新增 `tagOptions?: TagOption[]`
- `ArticlePage` 将 `TaxonomyTerm(kind="tag")` 映射为 `{ key, name, usageCount }`

- [ ] **Step 1: 写页面失败测试**

断言文章页显示 Tag 使用次数、可选择已有 Tag、可创建新 Tag、变更只进入草稿，并且不会请求 taxonomy term preview/apply。

- [ ] **Step 2: 确认 RED**

Run: `npm test -- --run src/pages/article/article-workflow.test.tsx`

Expected: FAIL，原因是 Tags 仍为自由文本输入。

- [ ] **Step 3: 集成组件**

移除 `MetadataForm` 中 Tags 的逗号解析输入，仅 Keywords 保留字符串数组输入；taxonomy 请求失败时仍传入当前草稿和状态。

- [ ] **Step 4: 验证页面行为**

Run: `npm test -- --run src/pages/article/article-workflow.test.tsx src/components/TagMultiSelect.test.tsx`

Expected: PASS，Category、Series 和 Keywords 断言无回归。

---

### Task 4: AI Tag 建议的可信数据模型

**Files:**
- Modify: `internal/domain/editorial/suggestion.go`
- Modify: `internal/app/editorial/suggest.go`
- Modify: `internal/app/editorial/suggest_test.go`
- Modify: `internal/storage/sqlite/repository/suggestion.go`
- Modify: `internal/storage/sqlite/repository/suggestion_test.go`

**Interfaces:**
- `SuggestionItem` 增加只由 Application 填充的 `UsageCount int`
- `GenerateSuggestionOptions` 使用现有 taxonomy 上下文，并接收标准 Tag 使用次数映射
- Provider 的 `Suggestion` 仍只提供 value、reason、confidence；`NewTerm` 和 `UsageCount` 由 Application 重算

- [ ] **Step 1: 写 Application 失败测试**

AI 返回 `Go`、重复 `go`、空值和 `NewTerm` 伪造值；断言 Application 去重、使用快照标准名称、填充真实文章数量，并把未知项标记为新 Tag。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/app/editorial -run TestGenerateSuggestions`

Expected: FAIL，原因是建议项没有可信使用次数且 tags 尚未按快照标准名归一。

- [ ] **Step 3: 实现校验与富化**

Application 建立大小写不敏感的标准 Tag 索引；忽略 Provider 的 `NewTerm` 判断，生成持久化前的最终 `NewTerm` 与 `UsageCount`。

- [ ] **Step 4: 验证 Repository 向后兼容**

旧 suggestion JSON 缺少 `usage_count` 时读取为 0；新增字段可往返保存。

- [ ] **Step 5: 验证 GREEN**

Run: `go test ./internal/app/editorial ./internal/storage/sqlite/repository`

Expected: PASS。

---

### Task 5: AI Provider 配置闭环

**Files:**
- Modify: `internal/transport/http/runtime.go`
- Create: `internal/transport/http/ai_settings.go`
- Create: `internal/transport/http/ai_settings_test.go`
- Modify: `internal/app/bootstrap/bootstrap.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/pages/settings/SettingsPage.tsx`
- Modify: `web/app/src/pages/settings/settings.test.tsx`

**Interfaces:**
- Adds: `PUT /api/v1/settings/ai`
- `RuntimeOptions` 注入最小 `AISecretStore`，只暴露 `Set/Delete`，读取仍由 Provider Registry 的 `SecretResolver` 完成。
- 配置保存 `enabled`、`base_url`、`model` 和可选新 API Key；GET 设置只返回 `ai_secret_saved`，绝不回传 Secret。

- [ ] **Step 1: 写后端失败测试**

覆盖启用时必填 URL、model 和已有/新 Secret；配置写入当前 workspace 的 `openai-compatible` Provider instance；禁用保留非敏感配置；跨 workspace 不可覆盖；响应和日志不含 API Key。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/transport/http -run TestAISettings`

Expected: FAIL，原因是设置路由和 Secret 写入依赖不存在。

- [ ] **Step 3: 实现后端配置**

使用事务更新 `provider_instances`，Secret key 使用稳定 Provider instance ID；先完成输入校验，再写 Keychain，数据库失败时返回可恢复错误。公开处理入口和依赖接口添加中文文档注释，API Key 不进入 SQLite 和日志。

- [ ] **Step 4: 写前端失败测试并实现设置表单**

设置页提供启用开关、Base URL、模型和 API Key；已有 Secret 时 API Key 可留空。保存期间禁用重复提交，成功/失败均通过 Toast 反馈。

Run: `npm test -- --run src/pages/settings/settings.test.tsx`

Expected: 先因请求未发送 FAIL；实现后 PASS。

- [ ] **Step 5: 验证配置闭环**

Run: `go test ./internal/transport/http -run TestAISettings && cd web/app && npm test -- --run src/pages/settings/settings.test.tsx`

Expected: PASS。

---

### Task 6: AI 生成 HTTP 运行链路

**Files:**
- Create: `internal/transport/http/article_suggestions.go`
- Create: `internal/transport/http/article_suggestions_test.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/runtime_test.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`

**Interfaces:**
- Adds: `POST /api/v1/articles/{id}/suggestions`
- Article detail returns persisted pending suggestions and derives `ai_configured` from an enabled AI Provider instance.
- Runtime builds AI through `ProviderRuntime.BuildAI`, never reads Secret directly and never logs prompt/body/result.

- [ ] **Step 1: 写 HTTP 失败测试**

覆盖未配置返回明确 409、已配置时只读取当前工作区文章、传入 taxonomy 候选、持久化建议，以及文章详情返回建议和 stale 状态。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/transport/http -run 'TestArticleSuggestions|TestArticleDetailSuggestions'`

Expected: FAIL，原因是路由和处理器不存在。

- [ ] **Step 3: 实现运行时处理器**

处理器查询当前 session workspace、文章、Source 文档、AI Provider 配置和 taxonomy 快照；构造 `GenerateSuggestionOptions` 后调用现有 Editorial Application。所有数据库查询按 workspace 约束，错误通过现有 `mapError` 输出。

- [ ] **Step 4: 扩展文章详情视图**

只输出前端需要的字段：建议 ID、字段、字符串值、理由、是否新 Tag、使用次数；内容 hash 不匹配时 `suggestions_stale=true`。不返回 Provider 原始响应或 Secret。

- [ ] **Step 5: 验证后端链路**

Run: `go test ./internal/transport/http ./internal/app/editorial ./internal/storage/sqlite/repository && go vet ./...`

Expected: PASS。

---

### Task 7: AI Tag 建议前端闭环

**Files:**
- Modify: `web/app/src/components/AISuggestions.tsx`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- `generateArticleSuggestions(articleID)` 调用 Task 6 API
- Tag 建议逐项调用 Tags 草稿追加回调，不复用覆盖整个字段的字符串接口

- [ ] **Step 1: 写失败测试**

覆盖生成按钮 loading/重复点击保护、已有 Tag 数量、新 Tag 标记、逐项采用/忽略、采用不重复、建议失效禁用和请求失败 Toast。

- [ ] **Step 2: 确认 RED**

Run: `npm test -- --run src/pages/article/article-workflow.test.tsx`

Expected: FAIL，原因是页面没有生成入口且 tags 建议仍按字段字符串覆盖。

- [ ] **Step 3: 实现生成与逐项处理**

生成成功刷新文章建议；采用 Tag 时通过 `MetadataForm` 的受控草稿入口追加标准名称，忽略只更新当前页面建议显示状态，不自动保存文章。

- [ ] **Step 4: 验证前端闭环**

Run: `npm test -- --run && npm run typecheck && npm run lint`

Expected: 全部 PASS。

---

### Task 8: Reflection、文档与整体回归

**Files:**
- Modify: `.codex/HANDOFF.md`
- Modify: `docs/design/interactions.md`
- Modify: `docs/design/provider-contracts.md`（仅当实际契约与现文档不同）
- Generated: `web/dist/*`

- [ ] **Step 1: 功能点 reflection**

逐项检查产品交互、代码职责、中文注释、空状态、重复点击、请求取消、跨工作区访问、Secret 和正文日志泄露、旧建议 JSON 兼容，以及 Category/Series 回归。

- [ ] **Step 2: 修复 reflection 问题**

每个问题先补失败测试，再修复；重复运行最小测试直到通过。

- [ ] **Step 3: 生产构建**

Run: `cd web/app && npm run build`

Expected: Vite 成功生成 `web/dist`。

- [ ] **Step 4: 整体回归**

Run:

```bash
cd web/app
npm test -- --run
npm run typecheck
npm run lint
npm run build
cd ../../..
go test ./...
go vet ./...
git diff --check
```

Expected: 全部 PASS。

- [ ] **Step 5: 检查并提交**

复查 `git diff`、`git status --short` 和生成物，只暂存本计划范围内文件，然后提交：

```bash
git commit -m "feat(editorial): edit and suggest article tags"
```
