# 内容目录范围与 Obsidian 附件实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让用户只索引 Vault 中明确选择的目录，同时兼容普通 Obsidian Markdown，并安全显示文章引用的 Vault 内图片。

**Architecture:** `folder.Scope` 负责纯路径规则，Obsidian Provider 组合范围过滤和附件解析；Application 扫描用例负责索引生命周期；HTTP Runtime 只编排配置与资源响应。数据库通过新迁移修复空稳定 ID 唯一约束，React 引导页使用目录候选和简化的纳入/忽略选择器。

**Tech Stack:** Go 1.24、SQLite、React 19、TypeScript、Vitest、Testing Library、Goldmark

## Global Constraints

- 关键代码使用中文注释，Go 公开方法和类型使用中文文档注释。
- 数据库新增表和字段必须写入 `schema_comments` 可见注释。
- 不引入新依赖，不使用 glob、正则或文件级范围规则。
- 目录配置只保存 Vault 相对路径，拒绝越界和符号链接逃逸。
- 每个任务执行 RED、GREEN、reflection 和相关回归后单独提交。

---

### Task 1: 修复文章身份与真实 Obsidian Markdown 兼容

**Files:**
- Create: `internal/storage/sqlite/migrations/0002_optional_stable_id.sql`
- Modify: `internal/storage/sqlite/sqlite_test.go`
- Modify: `internal/provider/source/obsidian/frontmatter.go`
- Modify: `internal/provider/source/obsidian/provider.go`
- Test: `internal/provider/source/obsidian/provider_test.go`
- Test: `internal/provider/source/obsidian/scan_test.go`

**Interfaces:**
- Produces: `parseDocument(content []byte) (contracts.SourceDocument, error)` 接受无 frontmatter 文档。
- Produces: `Provider.Read` 在标题为空时使用文件名回退，但不修改原始 frontmatter。

- [x] **Step 1: 写数据库与解析失败测试**

新增测试断言同工作区可插入两篇 `stable_id=''` 的不同路径文章，重复 `article_ONE` 仍失败；无 frontmatter 文档可读取，空标题回退为文件名，单字符串 tags 变为单元素数组，损坏 YAML 仍失败。

- [x] **Step 2: 运行测试并确认 RED**

Run: `go test ./internal/storage/sqlite ./internal/provider/source/obsidian -count=1`

Expected: 空稳定 ID触发唯一键冲突，无 frontmatter 返回“缺少 frontmatter”。

- [x] **Step 3: 实现迁移和最小兼容**

迁移重建 `articles` 表以移除表级稳定 ID 唯一约束，并创建：

```sql
CREATE UNIQUE INDEX idx_articles_workspace_stable_id
ON articles(workspace_id, stable_id) WHERE stable_id <> '';
```

`parseDocument` 将无 frontmatter 解释为空 mapping；`Provider.Read` 在标准标题为空时使用 `strings.TrimSuffix(filepath.Base(relativePath), filepath.Ext(relativePath))`；tags 标量仅接受字符串并规范成数组。

- [x] **Step 4: reflection 与验证**

Run: `go test ./internal/storage/sqlite ./internal/provider/source/obsidian ./internal/app/workspace -count=1 && go test -race ./internal/storage/sqlite ./internal/provider/source/obsidian`

检查旧数据库迁移保留外键、索引、数据和 schema comments；检查回退标题不被写回文件。

- [x] **Step 5: 提交**

```bash
git commit -m "fix(index): support notes without stable IDs"
```

### Task 2: 实现可解释的内容目录规则

**Files:**
- Create: `internal/provider/source/folder/scope.go`
- Create: `internal/provider/source/folder/scope_test.go`
- Modify: `internal/provider/source/folder/source.go`
- Modify: `internal/provider/source/obsidian/provider.go`
- Modify: `internal/provider/source/obsidian/scan_test.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/runtime_test.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Create: `web/app/src/components/ContentScopePicker.tsx`
- Create: `web/app/src/components/ContentScopePicker.test.tsx`
- Modify: `web/app/src/pages/setup/SetupPage.tsx`
- Modify: `web/app/src/pages/settings/SettingsPage.tsx`
- Test: `web/app/src/pages/settings/settings.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Produces: `folder.NewScope(contentRoots, ignoredFolders []string) (Scope, error)`。
- Produces: `Scope.Includes(relativePath string) bool`。
- Produces: `POST /api/v1/directories/inspect` 返回一级目录和 Markdown 数量。
- Produces: `PUT /api/v1/settings/content-scope` 保存当前 Source 规则并触发重扫；精确新增/移出预览由 Task 3 补齐。
- Extends: `WorkspaceDraft` 增加 `content_roots: string[]`、`ignored_folders: string[]`。

- [x] **Step 1: 写范围规则和 UI 失败测试**

覆盖多根目录、递归、忽略优先、父目录去重、系统目录、绝对路径、`.`、`..`、忽略目录不属于内容根；初始化 UI 覆盖默认不勾选、至少选择一个内容目录、添加忽略目录和摘要；设置页覆盖读取已有规则、保存并显示重扫结果。

- [x] **Step 2: 运行测试并确认 RED**

Run: `go test ./internal/provider/source/folder ./internal/transport/http -count=1 && npm --prefix web/app test -- --run src/components/ContentScopePicker.test.tsx`

Expected: `Scope`、inspect API 和组件尚不存在。

- [x] **Step 3: 实现纯规则、检查接口和组件**

`Scope` 构造时规范化、排序并去重路径；`MarkdownPaths` 只返回 `Scope.Includes` 的文档。inspect API 采用受同源保护的 POST，只接受已校验 Vault，返回相对目录与数量，不返回文章名或正文。创建工作区和设置页保存时严格校验并保存：

```json
{"content_roots":["Areas"],"ignored_folders":["Areas/私人记录"]}
```

前端只展示“管理这些目录”和“其中不管理这些目录”，附件目录不作为必选项。旧工作区的空配置明确显示“尚未选择内容目录”，不得隐式扫描整个 Vault；用户保存范围后立即执行 Task 3 定义的重扫。

- [x] **Step 4: reflection 与验证**

Run: `go test ./internal/provider/source/folder ./internal/provider/source/obsidian ./internal/transport/http -count=1 && npm --prefix web/app test -- --run && npm --prefix web/app run typecheck`

检查默认隐私、路径越界、嵌套规则和移动 Vault 后配置可用性。

- [x] **Step 5: 提交**

```bash
git commit -m "feat(setup): configure managed content folders"
```

### Task 3: 重扫已有工作区并维护索引生命周期

**Files:**
- Modify: `internal/app/workspace/scan.go`
- Modify: `internal/app/workspace/scan_test.go`
- Modify: `internal/storage/sqlite/repository/article.go`
- Modify: `internal/storage/sqlite/repository/article_test.go`
- Create: `internal/app/bootstrap/workspace_scan.go`
- Create: `internal/app/bootstrap/workspace_scan_test.go`
- Modify: `internal/app/bootstrap/bootstrap.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/runtime_test.go`

**Interfaces:**
- Extends: `ArticleStore` 增加 `MarkMissing(ctx, workspaceID, sourceID string, seenIDs []string) error`。
- Produces: `RescanRecentWorkspace(ctx context.Context, db *sql.DB) (workspace.ScanReport, error)`。
- Produces: 范围保存前返回精确的新增和移出数量，并要求用户确认后执行。

- [x] **Step 1: 写软删除、恢复和幂等失败测试**

第一次扫描写入范围内文章；第二次规则缩小后将未见文章设置 `deleted_at`；重新纳入清除 `deleted_at`；应用启动重扫现有 Source 并补齐空稳定 ID文章；重复启动不增加记录。

- [x] **Step 2: 运行测试并确认 RED**

Run: `go test ./internal/app/workspace ./internal/storage/sqlite/repository ./internal/app/bootstrap -count=1`

Expected: 缺少 `MarkMissing` 和启动重扫编排。

- [x] **Step 3: 实现事务化索引生命周期与启动重扫**

扫描收集成功索引的内部 ID，仅在完整扫描结束后软删除未见记录；单篇解析失败的既有文章保留并标记失败，不误删。启动装配在 HTTP 服务监听前重扫最近工作区，错误记录为扫描任务失败但不阻止 UI 启动。

- [x] **Step 4: reflection 与验证**

Run: `go test ./internal/app/workspace ./internal/storage/sqlite/repository ./internal/app/bootstrap ./internal/transport/http -count=1 && go test -race ./internal/app/workspace ./internal/app/bootstrap`

使用临时 Vault 验证缩小、扩大、解析失败和重复启动；不得在测试中读取用户真实 Vault。

- [x] **Step 5: 提交**

```bash
git commit -m "feat(index): rescan configured workspace safely"
```

### Task 4: 安全解析并预览 Obsidian 图片

**Files:**
- Create: `internal/provider/source/obsidian/assets.go`
- Create: `internal/provider/source/obsidian/assets_test.go`
- Create: `internal/transport/http/assets.go`
- Create: `internal/transport/http/assets_test.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/runtime_test.go`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Produces: `Provider.ResolveAssets(ctx context.Context, ref contracts.SourceRef, body string) ([]ResolvedAsset, error)`。
- Produces: `GET /api/v1/articles/{articleID}/assets/{assetToken}`。

- [ ] **Step 1: 写附件解析和 HTTP 安全失败测试**

覆盖 Markdown 相对图、Wiki 嵌入、尺寸、`attachmentFolderPath`、唯一文件名、同名歧义、远程 URL、绝对路径、`..` 越界、符号链接逃逸、未引用资源、错误文章 token、非图片 MIME 和 `nosniff`。

- [ ] **Step 2: 运行测试并确认 RED**

Run: `go test ./internal/provider/source/obsidian ./internal/transport/http -run 'Asset|Image' -count=1`

Expected: 附件解析器和资源路由尚不存在。

- [ ] **Step 3: 实现解析、token 和受控响应**

解析器只输出 Vault 内规范路径或已校验 `http/https` URL；本地图片在 Goldmark 输出后重写为文章级资源 URL。token 使用服务进程密钥进行 HMAC，绑定文章 ID、引用路径和源指纹；响应只允许 PNG、JPEG、GIF、WebP 和 AVIF，并设置 `X-Content-Type-Options: nosniff`。

- [ ] **Step 4: reflection、浏览器验证和全量回归**

Run: `go test ./... && go test -race ./... && go vet ./... && npm --prefix web/app test -- --run && npm --prefix web/app run lint && npm --prefix web/app run typecheck && npm --prefix web/app run build`

启动临时 Vault 和本地服务，用浏览器验证桌面与手机宽度下的目录配置、普通笔记列表、Markdown 图片、Wiki 图片、远程图片、缺失图片状态；复查 `git diff` 和 `git status`。

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(preview): render Obsidian vault images safely"
```
