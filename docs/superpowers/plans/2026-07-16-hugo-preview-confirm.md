# Hugo 发布预览与确认实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Hugo 发布增加 Section 扫描、真实 Artifact 预览、用户确认和同 Artifact 原子交付闭环。

**Architecture:** 通用 Publish 契约增加受控目标 Section 和 Artifact 文件 manifest；Hugo Provider 负责发现 Section、定位已有 bundle、Prepare 与 Deliver。Application `HugoPreviewService` 编排确定性任务、版本校验和安全视图，HTTP 不直接读取 staging；前端只使用相对路径和脱敏摘要。

**Tech Stack:** Go、SQLite、React、TypeScript、Vitest、Testing Library、Vite、Hugo CLI。

## Global Constraints

- 每个新增行为先写失败测试并确认 RED，再写最小实现。
- 关键代码和公开方法使用中文注释。
- 未确认前不得修改 Hugo 正式 `content/`。
- 浏览器响应不得包含 Vault、Hugo root、staging、Artifact Location 或 Secret 的绝对路径。
- 新文章只能选择扫描到的一级 Section；已有文章继续更新原 Section，本阶段禁止移动。
- 通用 Job API 不返回原始 `result_json`。
- 微信流程和现有兼容 publication 任务保持不变。
- 连续开发模式下功能点之间不提交；整体回归通过后使用一个 Conventional Commit 聚合提交。

---

### Task 1: Publish 契约与 Hugo Section 发现

**Files:**
- Modify: `internal/provider/contracts/publish.go`
- Create: `internal/provider/publish/hugo/sections.go`
- Create: `internal/provider/publish/hugo/sections_test.go`
- Modify: `internal/provider/publish/hugo/bundle.go`
- Modify: `internal/provider/publish/hugo/convert_test.go`

**Interfaces:**

```go
type PublishSection struct {
    Name         string
    ArticleCount int
}

type SectionDiscovery struct {
    Sections        []PublishSection
    ExistingSection string
    ExistingTarget  string
    SelectionLocked bool
}

type SectionAwarePublishProvider interface {
    PublishProvider
    DiscoverSections(ctx context.Context, sourceID string) (SectionDiscovery, error)
}
```

`PublishInput` 增加 `TargetSection string`。`PreparedArtifact` 增加 `TargetRelativePath string`、`Change string` 和 `Files []ArtifactFile`；`ArtifactFile` 包含 `RelativePath`、`MediaType`、`SHA256`、`Size`。

- [ ] **Step 1: 写 Section 安全扫描失败测试**

创建 `posts`、`notes`、`.hidden`、普通文件、符号链接和非法目录名，断言只返回合法真实一级目录，并统计递归 Markdown 文件数。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/provider/publish/hugo -run 'TestDiscoverSections|TestFindBundleAcrossSections'`

Expected: FAIL，原因是 `DiscoverSections` 和全 content bundle 查找不存在。

- [ ] **Step 3: 实现 Section 发现**

`DiscoverSections` 使用 `os.ReadDir(contentRoot)`，对每个候选执行 `entry.Type()&os.ModeSymlink == 0`、`safeSegment(name)`、非隐藏和 `withinOrEqual` 校验；计数只接受 `.md` 扩展名。

- [ ] **Step 4: 扩展 bundle 查找**

将 `findBundle(root, section, sourceID)` 重构为扫描整个 `content/` 的 `findBundleBySourceID(root, sourceID)`，返回绝对 bundle、一级 Section 和 found。重复 `source_id` 返回稳定错误；不在合法一级 Section 下的 bundle 返回错误。

- [ ] **Step 5: 验证 GREEN**

Run: `go test ./internal/provider/publish/hugo`

Expected: PASS。

---

### Task 2: Hugo Prepare 目标选择与文件 Manifest

**Files:**
- Modify: `internal/provider/publish/hugo/staging.go`
- Modify: `internal/provider/publish/hugo/deliver.go`
- Modify: `internal/provider/publish/hugo/provider_test.go`
- Modify: `internal/provider/publish/hugo/deliver_test.go`

**Interfaces:**
- Consumes: `PublishInput.TargetSection` 和 Task 1 的 `SectionAwarePublishProvider`。
- Produces: 包含安全相对目标和完整文件 manifest 的 `PreparedArtifact`。

- [ ] **Step 1: 写新文章目标失败测试**

断言新文章选择 `notes` 后生成 `content/notes/<slug>/index.md`，未提供 Section、Section 不在扫描结果或 Section 被删除时 Prepare 失败，正式 Hugo 目录不产生 bundle。

- [ ] **Step 2: 写已有文章锁定失败测试**

已有 `source_id` 位于 `content/posts/existing` 时，即使输入 `notes` 也返回冲突而不移动；输入或推导 `posts` 时更新原目标。

- [ ] **Step 3: 确认 RED**

Run: `go test ./internal/provider/publish/hugo -run 'TestPrepareUsesSelectedSection|TestPrepareLocksExistingSection|TestPreparedArtifactFiles'`

Expected: FAIL，原因是 Prepare 仍使用 Provider 固定 Section，Artifact 没有 manifest。

- [ ] **Step 4: 实现目标选择和 manifest**

Prepare 在复制站点前重新调用 Section 发现。新文章只接受发现列表中的 `TargetSection`；已有文章要求输入为空或与原 Section 一致。生成 bundle 后递归枚举 staged bundle，计算相对路径、MIME、大小和 SHA-256，并按路径排序。

- [ ] **Step 5: 收紧 Deliver 校验**

Artifact 的 `TargetPath` 只需位于 Hugo `content/`，但 `TargetRelativePath` 必须解析回同一目标，manifest 中每个文件必须位于 `Location`。继续使用现有备份、原子替换、真实站点 build 和回滚。

- [ ] **Step 6: 验证 Provider**

Run: `go test ./internal/provider/publish/hugo`

Expected: PASS。

---

### Task 3: HugoPreview Application 服务

**Files:**
- Create: `internal/app/publication/hugo_preview.go`
- Create: `internal/app/publication/hugo_preview_test.go`
- Modify: `internal/app/job/runner.go`（仅在 dedupe key 需要 Section 扩展时）

**Interfaces:**

```go
type HugoPreviewService struct {
    jobs JobStore
    previews PreviewDependencies
    now func() time.Time
}

type PreviewRequest struct {
    ArticleID string
    ProviderInstanceID string
    ContentHash string
    Section string
}

type ConfirmPreviewRequest struct {
    PreviewID string
}

type PreviewView struct {
    ID string
    ArticleID string
    ContentHash string
    Section string
    TargetPath string
    Change string
    Files []PreviewFile
    Diagnostics []PreviewDiagnostic
    PreviewURL string
    ExpiresAt *time.Time
    State string
    JobID string
}
```

- [ ] **Step 1: 写入队与幂等失败测试**

相同文章、Provider、content hash、Section 返回同一 `hugo_preview` Job；不同 Section 使用不同 ID。确认同一 preview 两次返回同一 `hugo_deliver` Job。

- [ ] **Step 2: 写安全视图失败测试**

从成功 Job `result_json` 读取完整 Artifact 后，只返回相对 `TargetPath`、文件摘要和诊断；断言绝对 `Location`、`TargetPath` 和 Provider 配置不出现在编码后的视图。

- [ ] **Step 3: 写确认校验失败测试**

文章 hash 变化、Artifact 过期、准备任务未成功、manifest 文件缺失、其他工作区 preview 均拒绝确认。

- [ ] **Step 4: 确认 RED**

Run: `go test ./internal/app/publication -run TestHugoPreview`

Expected: FAIL，原因是服务与模型不存在。

- [ ] **Step 5: 实现服务**

Preview ID 使用文章、Provider、hash、Section 的确定性摘要；payload 只包含这些字段。读取结果时反序列化服务端 Artifact，逐字段构造安全视图。Confirm 在入队前重新调用依赖验证当前文章和 Artifact staging 状态。

- [ ] **Step 6: 验证 GREEN**

Run: `go test ./internal/app/publication`

Expected: PASS。

---

### Task 4: Preview 与 Deliver Job Handler

**Files:**
- Create: `internal/app/bootstrap/hugo_preview_runner.go`
- Create: `internal/app/bootstrap/hugo_preview_runner_test.go`
- Modify: `internal/app/bootstrap/publication_runner.go`
- Modify: `internal/app/bootstrap/bootstrap.go`

**Interfaces:**
- 注册 `hugo_preview`：加载文章、构建 Hugo Provider、Preflight、Prepare、保存完整 Artifact result。
- 注册 `hugo_deliver`：读取对应 preview Job 的 Artifact、重新验证、Deliver、保存 Publication/Event。
- 旧 `publication`、`hugo_sync` 和 `wechat_prepare` Handler 保持兼容。

- [ ] **Step 1: 写 preview Handler 失败测试**

断言 Preflight blocking 时不调用 Prepare；成功时进度经过加载、检查、转换、构建阶段并持久化 Artifact；Prepare 不修改真实 bundle。

- [ ] **Step 2: 写 deliver Handler 失败测试**

断言只使用 preview result 中 Artifact，文章 hash 变化时拒绝，成功后 Publication 为 `published`；Provider Deliver 失败时不写成功状态。

- [ ] **Step 3: 确认 RED**

Run: `go test ./internal/app/bootstrap -run 'TestHugoPreviewJob|TestHugoDeliverJob'`

Expected: FAIL，原因是 Handler 未注册。

- [ ] **Step 4: 实现 Handler 与装配**

复用 `publicationJobHandler.loadInput`，但 preview OperationID 固定为 preview ID，输入 `TargetSection`。result JSON 包含 Artifact、Preflight diagnostics、article/provider/section 身份。Deliver 重新构建同一 Provider instance并调用 `Deliver`，不调用 `Prepare`。

- [ ] **Step 5: 验证任务恢复**

`hugo_preview` 和 `hugo_deliver` 均为幂等可重试任务；运行中断后 Runner 重排，Provider manifest 和 delivered manifest 保证重复执行安全。

- [ ] **Step 6: 验证 GREEN**

Run: `go test ./internal/app/bootstrap ./internal/app/job`

Expected: PASS。

---

### Task 5: HTTP API 与安全状态恢复

**Files:**
- Create: `internal/transport/http/hugo_preview.go`
- Create: `internal/transport/http/hugo_preview_test.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/runtime_test.go`
- Modify: `internal/app/bootstrap/bootstrap.go`

**Interfaces:**
- `GET /api/v1/articles/{id}/hugo-sections`
- `POST /api/v1/articles/{id}/hugo-previews`
- `GET /api/v1/hugo-previews/{id}`
- `POST /api/v1/hugo-previews/{id}/confirm`
- RuntimeOptions 注入 Task 3 的最小 `HugoPreviewAPI` interface。

- [ ] **Step 1: 写 Section 与 preview API 失败测试**

覆盖当前工作区隔离、扫描列表、已有 Section 锁定、非法 Section、stale hash、创建任务和查询安全视图。

- [ ] **Step 2: 写确认 API 失败测试**

覆盖 Origin 校验、重复确认、过期、stale 和其他工作区 preview。响应与日志不包含 fixture 的绝对 Hugo/staging 路径。

- [ ] **Step 3: 确认 RED**

Run: `go test ./internal/transport/http -run TestHugoPreview`

Expected: FAIL，原因是路由和 API 依赖不存在。

- [ ] **Step 4: 实现 HTTP 适配**

Handler 只解析 article/preview ID、Section 和 content hash，调用 Application API；所有错误映射为稳定中文 code/message。通用 Job endpoint继续只返回 state/progress/stage/error，不返回 `result_json`。

- [ ] **Step 5: 验证 GREEN**

Run: `go test ./internal/transport/http ./internal/app/bootstrap`

Expected: PASS。

---

### Task 6: 前端 Hugo 发布内联流程

**Files:**
- Create: `web/app/src/components/HugoPublishFlow.tsx`
- Create: `web/app/src/components/HugoPublishFlow.test.tsx`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**

```ts
interface HugoSectionView {
  sections: Array<{ name: string; article_count: number }>;
  existing_section?: string;
  selection_locked: boolean;
}

interface HugoPreviewView {
  id: string;
  content_hash: string;
  section: string;
  target_path: string;
  change: "added" | "updated";
  files: Array<{ relative_path: string; media_type: string; size: number }>;
  diagnostics: Array<{ code: string; level: string; message: string }>;
  preview_url?: string;
  expires_at?: string;
  state: string;
  job_id?: string;
}
```

- [ ] **Step 1: 写 Section 交互失败测试**

点击 `同步到 Hugo` 展开流程；多候选必须选择，单候选自动选择，已有 Section 只读；没有 Section 显示 Hugo 侧创建目录引导。

- [ ] **Step 2: 写预览与确认失败测试**

生成预览后轮询 Job/preview，展示目标相对路径、增加/更新、文件大小和诊断；未 ready 前没有确认按钮；确认只调用 preview confirm API，不调用旧 `/publications`。

- [ ] **Step 3: 写恢复与错误失败测试**

刷新后恢复当前 content hash preview；stale/expired 禁用确认并提供重新生成；请求失败和重复点击有 Toast/disabled 反馈。

- [ ] **Step 4: 确认 RED**

Run: `npm test -- --run src/components/HugoPublishFlow.test.tsx src/pages/article/article-workflow.test.tsx`

Expected: FAIL，原因是组件和 API 不存在，文章页仍直接创建 publication。

- [ ] **Step 5: 实现组件与页面集成**

组件内部管理 Section、preview 和 polling；ArticlePage 只控制流程展开与成功后刷新文章。桌面右栏和移动“发布”标签复用同一组件，不创建嵌套卡片。

- [ ] **Step 6: 验证前端**

Run: `npm test -- --run && npm run typecheck && npm run lint`

Expected: PASS。

---

### Task 7: Reflection、文档、真实页面与整体回归

**Files:**
- Modify: `docs/design/interactions.md`
- Modify: `docs/design/provider-contracts.md`
- Modify: `.codex/HANDOFF.md`
- Generated: `web/dist/*`

- [ ] **Step 1: 功能点 reflection**

检查 Section 安全、重复 `source_id`、绝对路径泄露、Artifact 过期、文章变化、重复点击、任务恢复、Deliver 回滚、微信回归、移动端遮挡和日志可诊断性。

- [ ] **Step 2: 修复 reflection 问题**

每个行为问题先补失败测试并确认 RED，再实现修复并运行最小 GREEN 测试。

- [ ] **Step 3: 真实页面验证**

使用隔离 Vault/Hugo fixture 启动本地服务，用 Playwright 在 1440×1000 和 390×844 验证 Section 选择、预览文件清单、确认按钮、无水平溢出和无 console error；临时 fixture 不进入 Git。

- [ ] **Step 4: 完整验证**

Run:

```bash
cd web/app
npm test -- --run
npm run typecheck
npm run lint
npm run build
cd ../..
go test ./...
go vet ./...
git diff --check
```

Expected: 全部 PASS。

- [ ] **Step 5: 聚合提交**

复查 `git diff` 和 `git status --short`，只暂存本计划文件，提交：

```bash
git commit -m "feat(publication): preview Hugo delivery"
```
