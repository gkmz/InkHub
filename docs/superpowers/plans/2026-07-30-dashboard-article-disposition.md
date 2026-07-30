# 工作台与文章批量处置实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复工作台硬编码空响应，并让用户在内容库批量标记外部已发表、长期忽略和恢复文章。

**Architecture:** 新增独立 `article_dispositions` 投影，由 `internal/app/disposition` 校验批量命令，Bootstrap SQLite Adapter 在单一事务内写入处置、渠道投影和事件。文章列表与工作台使用两个独立读模型，统一限定最近工作区；React 内容库负责选择和批量处置，工作台只负责分组待办。

**Tech Stack:** Go 1.24、SQLite/modernc.org/sqlite、net/http、React 19.2、TypeScript 5.7、Vite 5.4、Vitest 2.1、Testing Library、Playwright 1.61。

## Global Constraints

- 关键 Go、TypeScript 和 SQL 逻辑使用简短明确的中文注释；所有公开 Go 方法和导出类型都有文档注释。
- 新数据库表与每个字段必须通过 `schema_comments` 提供数据库层面可见的中文说明。
- 处置状态不得写回 Markdown，不得修改 `editorial_reviews` 来伪造审核通过。
- 所有读写严格限定最近工作区；客户端不能提交 Workspace ID 或 Provider ID。
- 批量命令每次 1 至 100 篇，全有或全无，版本冲突返回 409，重复执行保持幂等。
- `published` 必须多选至少一个已启用渠道；`ignored` 跨内容变化持续生效；`restore` 只恢复忽略。
- 内容库只全选当前已加载结果，不提供跨分页全选；默认不显示忽略文章。
- 不新增前端依赖，使用现有 Lucide 图标、Toast、颜色 token 和对话框样式。
- 每个实现任务严格执行测试先行：先运行并确认预期失败，再写生产代码。
- 每个提交遵循 Conventional Commits，且不得包含用户已有无关改动。

---

## 文件结构

### 新建

- `internal/storage/sqlite/migrations/0006_article_dispositions.sql`：文章处置投影和索引。
- `internal/domain/disposition/disposition.go`：处置 Kind、Record 和有效性判断。
- `internal/domain/disposition/disposition_test.go`：内容变化与忽略有效性测试。
- `internal/app/disposition/service.go`：批量命令规范化、校验和 Store 契约。
- `internal/app/disposition/service_test.go`：输入边界、去重和错误透传测试。
- `internal/app/bootstrap/disposition_store.go`：SQLite 批量事务适配器。
- `internal/app/bootstrap/disposition_store_test.go`：真实数据库事务、渠道和幂等测试。
- `internal/app/bootstrap/article_list_api.go`：当前工作区内容库查询、筛选与 cursor。
- `internal/app/bootstrap/article_list_api_test.go`：搜索、筛选、分页和工作区隔离测试。
- `internal/app/bootstrap/dashboard_api.go`：四组工作台读模型。
- `internal/app/bootstrap/dashboard_api_test.go`：分组优先级、排序和数量测试。
- `web/app/src/components/BatchDispositionDialog.tsx`：已发表渠道多选、忽略和恢复确认。
- `web/app/src/components/BatchDispositionDialog.test.tsx`：对话框行为和可访问性测试。
- `web/app/src/pages/library/library-page.test.tsx`：内容库选择、筛选、成功和失败流程。

### 修改

- `internal/storage/sqlite/migrate.go`、`internal/storage/sqlite/sqlite_test.go`：migration 版本和新字段 comment。
- `internal/app/bootstrap/runtime_api.go`：装配 Disposition Service，保留发布命令和公共标签映射。
- `internal/transport/http/router.go`、`internal/transport/http/router_test.go`：结构化列表、Dashboard 和批量处置端点。
- `internal/transport/http/runtime.go`：移除 Dashboard 硬编码拦截，让请求进入核心 Router；文章详情返回处置视图。
- `internal/transport/http/runtime_test.go`：文章详情处置 DTO 测试。
- `internal/app/publication/history.go`、`internal/app/publication/history_test.go`：外部标记发表事件中文映射。
- `web/app/src/api/types.ts`、`web/app/src/api/client.ts`：新 DTO 和请求函数。
- `web/app/src/pages/workspace/DashboardPage.tsx`、`web/app/src/pages/library/LibraryPage.tsx`、`web/app/src/pages/article/ArticlePage.tsx`：真实工作台、批量处置和详情提示。
- `web/app/src/components/ArticleRow.tsx`：可选 checkbox 与处置状态显示。
- `web/app/src/app.test.tsx`、`web/app/src/pages/article/article-workflow.test.tsx`：页面回归测试。
- `web/app/src/styles/app.css`：稳定批量栏、checkbox、状态提示和响应式对话框。
- `web/app/vite.config.ts`：开发验收 API 支持新 DTO 和批量状态变化。
- `web/app/e2e/workflows.spec.ts`、`web/app/e2e/screenshots.spec.ts`：批量流程、四视口溢出和截图。
- `web/dist/*`：重新生成嵌入式生产资源。

---

### Task 1: 处置领域类型与数据库迁移

**Files:**
- Create: `internal/domain/disposition/disposition.go`
- Create: `internal/domain/disposition/disposition_test.go`
- Create: `internal/storage/sqlite/migrations/0006_article_dispositions.sql`
- Modify: `internal/storage/sqlite/migrate.go`
- Modify: `internal/storage/sqlite/sqlite_test.go`

**Interfaces:**
- Produces: `disposition.Kind`, `disposition.Record`, `Record.Effective(currentHash string) bool`。
- Produces: SQLite `article_dispositions(article_id,workspace_id,kind,content_hash,cleared_at,created_at,updated_at)`。

- [ ] **Step 1: 写处置有效性和 migration 失败测试**

```go
func TestRecordEffectiveUsesVersionOnlyForPublished(t *testing.T) {
    tests := []struct {
        record disposition.Record
        current string
        want bool
    }{
        {disposition.Record{Kind: disposition.KindPublished, ContentHash: "v1"}, "v1", true},
        {disposition.Record{Kind: disposition.KindPublished, ContentHash: "v1"}, "v2", false},
        {disposition.Record{Kind: disposition.KindIgnored, ContentHash: "v1"}, "v2", true},
        {disposition.Record{Kind: disposition.KindIgnored, ClearedAt: ptrTime(time.Now())}, "v1", false},
    }
    for _, test := range tests {
        if got := test.record.Effective(test.current); got != test.want {
            t.Fatalf("Effective() = %v, want %v", got, test.want)
        }
    }
}

func ptrTime(value time.Time) *time.Time { return &value }
```

在 `TestOpenMigratesEmptyDatabaseAndIsRepeatable` 中把期望版本改为 6，并新增插入 `article_dispositions` 后验证复合外键、CHECK 约束和 `schema_comments` 的断言。

- [ ] **Step 2: 运行测试确认红灯**

Run: `go test ./internal/domain/disposition ./internal/storage/sqlite`

Expected: FAIL，原因是 `internal/domain/disposition` 不存在且 migration 版本仍为 5。

- [ ] **Step 3: 实现最小领域类型和 migration**

```go
// Package disposition 定义文章管理处置状态。
package disposition

import "time"

// Kind 是文章当前管理处置类型。
type Kind string

const (
    KindPublished Kind = "published"
    KindIgnored   Kind = "ignored"
)

// Record 是文章当前处置投影。
type Record struct {
    ArticleID   string
    WorkspaceID string
    Kind        Kind
    ContentHash string
    ClearedAt   *time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// Effective 判断处置对当前内容版本是否有效。
func (r Record) Effective(currentHash string) bool {
    if r.ClearedAt != nil {
        return false
    }
    return r.Kind == KindIgnored || r.Kind == KindPublished && r.ContentHash == currentHash
}
```

`0006_article_dispositions.sql` 使用设计文档中的完整 DDL。在 `tableComments` 增加 `article_dispositions`；在 `columnComments` 增加 `kind` 和 `cleared_at` 的通用说明，确保 comment 全覆盖测试通过。

- [ ] **Step 4: 运行测试确认绿灯**

Run: `go test ./internal/domain/disposition ./internal/storage/sqlite`

Expected: PASS，migration version 为 6，表/字段 comment 全覆盖。

- [ ] **Step 5: 提交**

```bash
git add internal/domain/disposition internal/storage/sqlite/migrations/0006_article_dispositions.sql internal/storage/sqlite/migrate.go internal/storage/sqlite/sqlite_test.go
git commit -m "feat(disposition): add article disposition schema"
```

---

### Task 2: 批量处置 Application 与 SQLite 原子事务

**Files:**
- Create: `internal/app/disposition/service.go`
- Create: `internal/app/disposition/service_test.go`
- Create: `internal/app/bootstrap/disposition_store.go`
- Create: `internal/app/bootstrap/disposition_store_test.go`

**Interfaces:**
- Consumes: `disposition.KindPublished`、`disposition.KindIgnored` 和 migration 6。
- Produces: `disposition.Service.Apply(ctx context.Context, command Command) (Result, error)`。
- Produces: `newDispositionService(db *sql.DB) *disposition.Service`，供 Task 3 的 Bootstrap API 装配。

- [ ] **Step 1: 写 Service 输入边界失败测试**

```go
func TestServiceApplyNormalizesAndValidatesBatch(t *testing.T) {
    store := &capturingStore{result: Result{Processed: 2, Changed: 2}}
    service := NewService(store)
    got, err := service.Apply(context.Background(), Command{
        Operation: OperationPublished,
        Articles: []ArticleVersion{{ID: "a1", ContentVersion: "v1"}, {ID: "a1", ContentVersion: "v1"}, {ID: "a2", ContentVersion: "v2"}},
        Channels: []string{"wechat", "hugo", "wechat"},
    })
    if err != nil || got.Processed != 2 || len(store.command.Articles) != 2 || !reflect.DeepEqual(store.command.Channels, []string{"hugo", "wechat"}) {
        t.Fatalf("Apply() = %+v, %v command=%+v", got, err, store.command)
    }
}
```

分别断言空批次、超过 100 篇、空版本、未知操作、`published` 无渠道、`ignored/restore` 带渠道返回 `ErrInvalidCommand`。

- [ ] **Step 2: 运行 Service 测试确认红灯**

Run: `go test ./internal/app/disposition`

Expected: FAIL，原因是 Service 和错误类型尚不存在。

- [ ] **Step 3: 实现 Application 校验与 Store 契约**

```go
type Operation string

const (
    OperationPublished Operation = "published"
    OperationIgnored   Operation = "ignored"
    OperationRestore   Operation = "restore"
)

var (
    ErrInvalidCommand     = errors.New("文章处置请求无效")
    ErrContentChanged     = errors.New("文章内容已变化")
    ErrArticleNotFound    = errors.New("文章不存在")
    ErrChannelUnavailable = errors.New("发布渠道未配置")
)

type ArticleVersion struct {
    ID             string
    ContentVersion string
}

type Command struct {
    Operation Operation
    Articles  []ArticleVersion
    Channels  []string
}

type Result struct {
    Processed int
    Changed   int
    Unchanged int
}

type Store interface {
    Apply(ctx context.Context, command Command) (Result, error)
}

// Apply 校验并规范化批量命令后交给持久化事务执行。
func (s *Service) Apply(ctx context.Context, command Command) (Result, error) {
    normalized, err := normalize(command)
    if err != nil {
        return Result{}, err
    }
    return s.store.Apply(ctx, normalized)
}
```

`normalize` 使用 map 去重后稳定排序文章 ID 和渠道，只允许 `hugo`、`wechat`。

- [ ] **Step 4: 写真实 SQLite 事务失败测试**

在 `disposition_store_test.go` 建立两个工作区、两篇文章和两个 Provider，覆盖：

```go
func TestDispositionStorePublishesMultipleChannelsAtomically(t *testing.T) {
    service, db := seedDispositionService(t)
    result, err := service.Apply(context.Background(), disposition.Command{
        Operation: disposition.OperationPublished,
        Articles: []disposition.ArticleVersion{{ID: "a1", ContentVersion: "hash-1"}, {ID: "a2", ContentVersion: "hash-2"}},
        Channels: []string{"hugo", "wechat"},
    })
    if err != nil || result.Changed != 2 {
        t.Fatalf("Apply() = %+v, %v", result, err)
    }
    assertRows(t, db, "article_dispositions", 2)
    assertRows(t, db, "publications", 4)
    assertRows(t, db, "publication_events", 4)
}

func seedDispositionService(t *testing.T) (*disposition.Service, *sql.DB) {
    t.Helper()
    db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { db.Close() })
    _, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','当前','/tmp','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-07-30','2026-07-30');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,content_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','one','one.md','hash-1','2026-07-30','2026-07-30','2026-07-30'),('a2','w1','s1','two','two.md','hash-2','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('h1','w1','hugo','Hugo','2026-07-30','2026-07-30'),('m1','w1','wechat','微信','2026-07-30','2026-07-30')`)
    if err != nil { t.Fatal(err) }
    return newDispositionService(db), db
}

func assertRows(t *testing.T, db *sql.DB, table string, want int) {
    t.Helper()
    var got int
    if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil || got != want {
        t.Fatalf("%s rows=%d want=%d err=%v", table, got, want, err)
    }
}
```

再覆盖任一版本冲突整批 0 写入、跨工作区拒绝、渠道缺失、重复 published 不追加 Event、ignore 内容变化后仍有效、restore 仅清除 ignored。

- [ ] **Step 5: 运行 Adapter 测试确认红灯**

Run: `go test ./internal/app/bootstrap -run 'Disposition'`

Expected: FAIL，原因是 SQLite Store 尚不存在。

- [ ] **Step 6: 实现单事务 Store**

`disposition_store.Apply` 必须按以下顺序执行：

```go
tx, err := s.db.BeginTx(ctx, nil)
// 1. 查询最近工作区。
// 2. 一次查询并锁定批次文章的当前 content_hash，验证数量和版本。
// 3. published 时解析每个所选渠道唯一启用的 Provider。
// 4. 对文章 upsert article_dispositions。
// 5. 对新渠道事实 upsert publications 并 INSERT publication_events。
// 6. ignored 只更新 disposition；restore 只设置 ignored.cleared_at。
// 7. 计算 changed/unchanged 后提交。
```

Event ID 使用现有 `stableAPIID("event", publicationID, "marked_published", contentHash)`；插入前以 Publication 当前状态和版本判断是否已有相同事实。任一步失败必须 `Rollback`。

- [ ] **Step 7: 运行 Service 与 Adapter 测试确认绿灯**

Run: `go test ./internal/app/disposition`

Run: `go test ./internal/app/bootstrap -run 'Disposition'`

Expected: PASS，批量事务、幂等和错误类型均符合断言。

- [ ] **Step 8: 提交**

```bash
git add internal/app/disposition internal/app/bootstrap/disposition_store.go internal/app/bootstrap/disposition_store_test.go
git commit -m "feat(disposition): apply article batches atomically"
```

---

### Task 3: 批量处置 HTTP API

**Files:**
- Modify: `internal/transport/http/router.go`
- Modify: `internal/transport/http/router_test.go`
- Modify: `internal/app/bootstrap/disposition_store.go`
- Modify: `internal/app/bootstrap/runtime_api.go`

**Interfaces:**
- Consumes: `disposition.Service.Apply`。
- Produces: `POST /api/v1/articles/batch-disposition`。
- Produces: `BatchDispositionCommand`、`BatchDispositionResult` 和 API `BatchDisposition` 方法。
- Produces: Bootstrap 将 Transport DTO 映射为 `disposition.Command`，不让 Application 依赖 HTTP。

- [ ] **Step 1: 写路由失败测试**

```go
func TestBatchDispositionValidatesAndReturnsCounts(t *testing.T) {
    api := &fakeAPI{dispositionResult: BatchDispositionResult{Processed: 2, Changed: 1, Unchanged: 1}}
    request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/batch-disposition",
        strings.NewReader(`{"operation":"published","articles":[{"id":"a1","content_version":"v1"},{"id":"a2","content_version":"v2"}],"channels":["hugo","wechat"]}`))
    request.Header.Set("Content-Type", "application/json")
    request.Header.Set("Origin", "http://localhost")
    response := httptest.NewRecorder()
    NewRouter(api).ServeHTTP(response, request)
    if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"processed":2`) {
        t.Fatalf("code=%d body=%s", response.Code, response.Body.String())
    }
}
```

新增表驱动测试验证未知字段/操作、空文章、超过 100 篇、错误 Content-Type、错误 Origin、409/404/422 稳定错误码。

- [ ] **Step 2: 运行路由测试确认红灯**

Run: `go test ./internal/transport/http -run 'BatchDisposition'`

Expected: FAIL，端点返回 404 或 API 接口缺少方法。

- [ ] **Step 3: 实现 DTO、Handler 和错误映射**

```go
type ArticleVersion struct {
    ID             string `json:"id"`
    ContentVersion string `json:"content_version"`
}

type BatchDispositionCommand struct {
    Operation string           `json:"operation"`
    Articles  []ArticleVersion `json:"articles"`
    Channels  []string         `json:"channels,omitempty"`
}

type BatchDispositionResult struct {
    Processed int `json:"processed"`
    Changed   int `json:"changed"`
    Unchanged int `json:"unchanged"`
}
```

Router 在精确路径上执行现有 `validateWriteRequest`，使用 `decodeJSON` 禁止未知字段，然后调用 `API.BatchDisposition`。`mapError` 映射：内容变化 409 `disposition.content_changed`、文章不存在 404、渠道不可用 422 `disposition.channel_unavailable`、命令无效 400。

Transport 定义 `ErrDispositionContentChanged`、`ErrDispositionInvalid` 和 `ErrDispositionChannelUnavailable`，不直接导入 Application package。在 `databaseAPI` 增加 `dispositions *disposition.Service`，`newDatabaseAPI` 调用 `newDispositionService(db)`；`BatchDisposition` 逐字段映射 Transport/Application 类型与错误并返回 counts。

- [ ] **Step 4: 运行 Transport 与 Bootstrap 测试确认绿灯**

Run: `go test ./internal/transport/http ./internal/app/bootstrap -run 'BatchDisposition|Disposition'`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/transport/http/router.go internal/transport/http/router_test.go internal/app/bootstrap/disposition_store.go internal/app/bootstrap/runtime_api.go
git commit -m "feat(api): expose batch article dispositions"
```

---

### Task 4: 当前工作区内容库查询与处置筛选

**Files:**
- Create: `internal/app/bootstrap/article_list_api.go`
- Create: `internal/app/bootstrap/article_list_api_test.go`
- Modify: `internal/app/bootstrap/runtime_api.go`
- Modify: `internal/app/bootstrap/article_cursor_test.go`
- Modify: `internal/transport/http/router.go`
- Modify: `internal/transport/http/router_test.go`

**Interfaces:**
- Produces: `ArticleListQuery{Cursor,Limit,Search,State,Disposition}`。
- Produces: `ArticleSummary.ContentVersion` 和 `ArticleSummary.Disposition`。
- Produces: `ArticlePage.AvailableChannels []string`，来自当前工作区启用的 Hugo/微信 Provider。
- Preserves: 原有稳定 keyset cursor。

- [ ] **Step 1: 写列表查询失败测试**

`article_list_api_test.go` 建立最近工作区 `w2` 和旧工作区 `w1`，并断言：

```go
page, err := api.ListArticles(ctx, httptransport.ArticleListQuery{
    Search: "SQLite", State: "pending_review", Disposition: "unresolved", Limit: 50,
})
if err != nil || len(page.Items) != 1 || page.Items[0].ID != "w2-match" || page.Items[0].ContentVersion == "" {
    t.Fatalf("ListArticles() = %+v, %v", page, err)
}
```

分别覆盖默认排除 ignored、`published` 仅匹配当前 hash、`ignored` 跨 hash 匹配、审核和处置 AND 组合、分页不跨工作区，以及 `available_channels` 只包含当前工作区启用渠道。

- [ ] **Step 2: 运行列表测试确认红灯**

Run: `go test ./internal/app/bootstrap -run 'ListArticles|ArticleCursor'`

Expected: FAIL，现有查询返回全部工作区且忽略搜索/筛选。

- [ ] **Step 3: 实现结构化查询和安全动态条件**

将 `ListArticles(ctx,cursor,limit)` 移到 `article_list_api.go` 并改为：

```go
func (api databaseAPI) ListArticles(ctx context.Context, input httptransport.ArticleListQuery) (httptransport.ArticlePage, error)
```

基础 SQL 必须包含：

```sql
WHERE articles.deleted_at IS NULL
  AND articles.workspace_id=(SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1)
  AND NOT (article_dispositions.kind='ignored' AND article_dispositions.cleared_at IS NULL)
```

搜索使用参数化 `LOWER(articles.title) LIKE ? ESCAPE '\'`，并转义 `%`、`_` 和 `\`。审核状态与处置状态只允许固定枚举后拼接固定 SQL 片段，值继续通过参数传入。统一 scanner 填充 `content_version`、有效 `disposition`、目录和渠道自然语言状态；第一页和后续页都返回相同的当前工作区 `available_channels`。

- [ ] **Step 4: 更新 Router 查询解析失败测试**

在 `router_test.go` 断言 `/articles?q=SQLite&state=pending_review&disposition=published&limit=25` 被完整传入 fake API；未知 `state`/`disposition` 返回 400，不调用 API。

- [ ] **Step 5: 实现 Router 查询解析并运行测试**

```go
query := ArticleListQuery{
    Cursor: request.URL.Query().Get("cursor"),
    Search: strings.TrimSpace(request.URL.Query().Get("q")),
    State: request.URL.Query().Get("state"),
    Disposition: request.URL.Query().Get("disposition"),
    Limit: limit,
}
```

Run: `go test ./internal/app/bootstrap ./internal/transport/http -run 'ListArticles|ArticleCursor|ListQuery'`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/app/bootstrap/article_list_api.go internal/app/bootstrap/article_list_api_test.go internal/app/bootstrap/runtime_api.go internal/app/bootstrap/article_cursor_test.go internal/transport/http/router.go internal/transport/http/router_test.go
git commit -m "fix(library): scope and filter article queries"
```

---

### Task 5: 真实工作台读模型与端点

**Files:**
- Create: `internal/app/bootstrap/dashboard_api.go`
- Create: `internal/app/bootstrap/dashboard_api_test.go`
- Modify: `internal/transport/http/router.go`
- Modify: `internal/transport/http/router_test.go`
- Modify: `internal/transport/http/runtime.go`

**Interfaces:**
- Consumes: Task 4 的 `ArticleSummary` scanner 和渠道标签映射。
- Produces: `databaseAPI.Dashboard(ctx) (httptransport.DashboardView, error)`。
- Produces: `GET /api/v1/dashboard` 四组响应。

- [ ] **Step 1: 写四组分类失败测试**

```go
func TestDashboardGroupsCurrentWorkspaceWithoutDuplicates(t *testing.T) {
    api := seedDashboardAPI(t)
    view, err := api.Dashboard(context.Background())
    if err != nil {
        t.Fatal(err)
    }
    assertIDs(t, view.Failed, "failed")
    assertIDs(t, view.Changed, "changed")
    assertIDs(t, view.NeedsReview, "pending")
    assertIDs(t, view.RecentlyHandled, "published", "approved")
}

// seedDashboardAPI 在两个工作区插入 ignored、failed、changed、pending、published-current 和 approved 夹具。
func seedDashboardAPI(t *testing.T) databaseAPI {
    t.Helper()
    db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { db.Close() })
    _, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES
('w1','旧工作区','/tmp','2026-07-01','2026-07-01','2026-07-01'),
('w2','当前工作区','/tmp','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES
('s1','w1','obsidian','/tmp/old','2026-07-01','2026-07-01'),
('s2','w2','obsidian','/tmp/current','2026-07-30','2026-07-30');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,indexed_at,created_at,updated_at) VALUES
('old','w1','s1','old','old.md','旧文章','old','2026-07-01','2026-07-01','2026-07-01'),
('ignored','w2','s2','ignored','ignored.md','忽略','i1','2026-07-30','2026-07-30','2026-07-30'),
('failed','w2','s2','failed','failed.md','失败','f1','2026-07-30','2026-07-30','2026-07-30'),
('changed','w2','s2','changed','changed.md','更新','c2','2026-07-30','2026-07-30','2026-07-30'),
('pending','w2','s2','pending','pending.md','待审核','p1','2026-07-30','2026-07-30','2026-07-30'),
('published','w2','s2','published','published.md','已发表','x1','2026-07-30','2026-07-30','2026-07-30'),
('approved','w2','s2','approved','approved.md','已审核','a1','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO editorial_reviews(article_id,state,approved_content_hash,updated_at) VALUES
('failed','blocked',NULL,'2026-07-30'),('changed','changed','c1','2026-07-30'),
('pending','pending_review',NULL,'2026-07-30'),('approved','approved','a1','2026-07-30');
INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,created_at,updated_at) VALUES
('ignored','w2','ignored','i1','2026-07-30','2026-07-30'),
('published','w2','published','x1','2026-07-30','2026-07-30')`)
    if err != nil { t.Fatal(err) }
    return newDatabaseAPI(db)
}

func assertIDs(t *testing.T, items []httptransport.ArticleSummary, want ...string) {
    t.Helper()
    got := make([]string, 0, len(items))
    for _, item := range items { got = append(got, item.ID) }
    if !reflect.DeepEqual(got, want) { t.Fatalf("IDs=%v want=%v", got, want) }
}
```

额外覆盖：未选择渠道的当前失败优先于有效 published disposition、stale published 进入 changed、ignored 永不出现、最近处理按处理时间且最多 10 篇。

- [ ] **Step 2: 运行 Bootstrap 测试确认红灯**

Run: `go test ./internal/app/bootstrap -run 'Dashboard'`

Expected: FAIL，`Dashboard` 尚不存在。

- [ ] **Step 3: 实现读模型分类**

使用单次参数化查询读取当前工作区文章、Review、Disposition 和两个渠道当前投影，再在 Go 中按固定优先级分类：

```go
switch {
case row.Ignored:
    continue
case row.CurrentFailure:
    view.Failed = append(view.Failed, row.Summary)
case row.StalePublished || row.Changed || row.Outdated:
    view.Changed = append(view.Changed, row.Summary)
case row.CurrentPublished:
    recent = append(recent, handled{row.Summary, row.DispositionUpdatedAt})
case row.NeedsReview:
    view.NeedsReview = append(view.NeedsReview, row.Summary)
case row.CurrentApproved:
    recent = append(recent, handled{row.Summary, row.ReviewUpdatedAt})
}
```

三个待办组按 `modified_at DESC,id DESC`；recent 按处理时间降序并截取 10。

- [ ] **Step 4: 写端点和 Runtime 穿透失败测试**

Router fake API 返回四组各一项，断言 JSON 字段完整。Runtime 测试使用 core handler 返回标记响应，断言 `/api/v1/dashboard` 不再命中空数组分支。

- [ ] **Step 5: 实现端点并删除硬编码**

在核心 Router 增加 `GET /api/v1/dashboard`，调用 `API.Dashboard`。删除 `runtime.go` 中：

```go
case request.Method == http.MethodGet && request.URL.Path == "/api/v1/dashboard":
    writeJSON(response, http.StatusOK, map[string]any{"items": []any{}})
```

Run: `go test ./internal/app/bootstrap ./internal/transport/http -run 'Dashboard'`

Expected: PASS，Runtime 请求进入核心 Router。

- [ ] **Step 6: 提交**

```bash
git add internal/app/bootstrap/dashboard_api.go internal/app/bootstrap/dashboard_api_test.go internal/transport/http/router.go internal/transport/http/router_test.go internal/transport/http/runtime.go
git commit -m "fix(dashboard): return current workspace tasks"
```

---

### Task 6: 前端 DTO、真实工作台与发布历史文案

**Files:**
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/pages/workspace/DashboardPage.tsx`
- Modify: `web/app/src/components/ArticleRow.tsx`
- Modify: `web/app/src/app.test.tsx`
- Modify: `internal/app/publication/history.go`
- Modify: `internal/app/publication/history_test.go`

**Interfaces:**
- Consumes: `DashboardView`、`ArticleSummary.content_version`、`ArticleSummary.disposition`。
- Produces: `getDashboard(): Promise<DashboardView>`。
- Preserves: ArticleRow 在内容库中的现有操作。

- [ ] **Step 1: 写工作台四区段失败测试**

将 `app.test.tsx` Dashboard mock 改为四组响应并断言：

```tsx
expect(await screen.findByRole("heading", { name: "处理失败" })).toBeInTheDocument();
expect(screen.getByRole("heading", { name: "内容已更新" })).toBeInTheDocument();
expect(screen.getByRole("heading", { name: "需要审核" })).toBeInTheDocument();
expect(screen.getByRole("heading", { name: "最近处理" })).toBeInTheDocument();
expect(screen.getAllByTestId("dashboard-row")).toHaveLength(4);
```

另测四组全空时显示空状态；只有 recent 时不显示空状态。

- [ ] **Step 2: 运行前端测试确认红灯**

Workdir: `web/app`

Run: `npm test -- --run src/app.test.tsx`

Expected: FAIL，现有页面读取 `items` 且只渲染一个区段。

- [ ] **Step 3: 实现 DTO 和分组组件**

```ts
export interface DashboardView {
  failed: ArticleSummary[];
  changed: ArticleSummary[];
  needs_review: ArticleSummary[];
  recently_handled: ArticleSummary[];
}
```

`DashboardPage` 用一个 `DashboardSection` 模块级组件渲染非空组；总数为 0 才显示空状态。不要在 render 中重复 filter/sort，后端顺序为权威。

`ArticleRow` 状态文案优先使用 `article.disposition === "published" ? "已发表" : article.disposition === "ignored" ? "已忽略" : stateText[article.state]`。

- [ ] **Step 4: 写并实现外部发表历史映射**

后端测试输入 `hugo/marked_published` 与 `wechat/marked_published`，期望“已标记为 Hugo 已发表”和“已标记为微信已发表”，State 对浏览器归一化为 `published`。实现 `historyItem` 对应 case，避免页面显示通用“发布状态已更新”。

- [ ] **Step 5: 运行后端映射与工作台测试确认绿灯**

Run: `go test ./internal/app/publication`

Workdir: `web/app`

Run: `npm test -- --run src/app.test.tsx`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/app/publication/history.go internal/app/publication/history_test.go web/app/src/api/types.ts web/app/src/api/client.ts web/app/src/pages/workspace/DashboardPage.tsx web/app/src/components/ArticleRow.tsx web/app/src/app.test.tsx
git commit -m "feat(ui): render actionable dashboard groups"
```

---

### Task 7: 内容库选择与处置筛选

**Files:**
- Create: `web/app/src/pages/library/library-page.test.tsx`
- Modify: `web/app/src/pages/library/LibraryPage.tsx`
- Modify: `web/app/src/components/ArticleRow.tsx`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: Task 4 的 `disposition` 查询参数。
- Produces: 当前可见文章选择集合、表头全选和处置状态筛选。
- Produces: `ArticleRow` 的 `selected`、`onSelectedChange` 可选 props。

- [ ] **Step 1: 写选择与筛选失败测试**

```tsx
const articleA = { id: "a1", title: "第一篇", directory: "notes", category: "", modified_at: "2026-07-30T10:00:00Z", state: "pending_review", hugo_state: "尚未同步", wechat_state: "尚未准备" };
const articleB = { ...articleA, id: "a2", title: "第二篇", modified_at: "2026-07-30T09:00:00Z" };

test("内容库只选择当前已加载文章并在筛选变化后清理不可见选择", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).includes("disposition=ignored")
    ? Response.json({ items: [], available_channels: ["hugo", "wechat"] })
    : Response.json({ items: [{ ...articleA, content_version: "v1" }, { ...articleB, content_version: "v2" }], available_channels: ["hugo", "wechat"] }));
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);
  await userEvent.click(await screen.findByRole("checkbox", { name: "选择当前已加载文章" }));
  expect(screen.getByText("已选择 2 篇")).toBeInTheDocument();
  await userEvent.selectOptions(screen.getByLabelText("处置状态"), "ignored");
  await waitFor(() => expect(screen.queryByText("已选择 2 篇")).not.toBeInTheDocument());
});
```

另测单行 checkbox 不触发打开文章、加载更多保留旧选择且不选择新项、默认请求不带 ignored、筛选请求带 `disposition=published/ignored/unresolved`。

- [ ] **Step 2: 运行测试确认红灯**

Workdir: `web/app`

Run: `npm test -- --run src/pages/library/library-page.test.tsx`

Expected: FAIL，checkbox、处置筛选和批量栏不存在。

- [ ] **Step 3: 实现选择状态和查询参数**

```tsx
const [disposition, setDisposition] = useState("");
const [selected, setSelected] = useState<Set<string>>(() => new Set());
const visibleIDs = new Set((items ?? []).map((item) => item.id));

useEffect(() => {
  setSelected((current) => new Set([...current].filter((id) => visibleIDs.has(id))));
}, [items]);
```

实际实现避免把临时 `Set` 放入 effect dependency；在列表请求成功时用 `page.items` 与合并后的 next items 直接收敛选择。`articleQuery` 增加 disposition。表头 checkbox 使用 `aria-checked="mixed"` 表达部分选中。

- [ ] **Step 4: 实现 ArticleRow checkbox 和稳定样式**

```tsx
{onSelectedChange && <input
  type="checkbox"
  aria-label={`选择文章 ${article.title || "未命名文章"}`}
  checked={selected}
  onChange={(event) => onSelectedChange(article.id, event.currentTarget.checked)}
/>}
```

CSS 为列表 checkbox 列设置固定 32px 轨道；批量栏使用稳定最小高度、非浮动全宽 band。移动端把 checkbox 保留在标题左侧，批量按钮允许换行但不重排列表宽度。

- [ ] **Step 5: 运行测试、类型检查确认绿灯**

Workdir: `web/app`

Run: `npm test -- --run src/pages/library/library-page.test.tsx`

Run: `npm run typecheck`

Expected: PASS，无 TypeScript 错误。

- [ ] **Step 6: 提交**

```bash
git add web/app/src/pages/library/LibraryPage.tsx web/app/src/pages/library/library-page.test.tsx web/app/src/components/ArticleRow.tsx web/app/src/api/types.ts web/app/src/styles/app.css
git commit -m "feat(library): select and filter article dispositions"
```

---

### Task 8: 批量确认、提交、恢复与文章详情提示

**Files:**
- Create: `web/app/src/components/BatchDispositionDialog.tsx`
- Create: `web/app/src/components/BatchDispositionDialog.test.tsx`
- Modify: `web/app/src/pages/library/LibraryPage.tsx`
- Modify: `web/app/src/pages/library/library-page.test.tsx`
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/api/types.ts`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/runtime_test.go`
- Modify: `web/app/src/pages/article/ArticlePage.tsx`
- Modify: `web/app/src/pages/article/article-workflow.test.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: `POST /articles/batch-disposition` 和 `ArticlePage.available_channels`。
- Produces: `batchDisposition(command): Promise<BatchDispositionResult>`。
- Produces: `ArticleDetail.disposition?: {kind,channels}`。

- [ ] **Step 1: 写对话框失败测试**

```tsx
test("标记已发表允许多选已启用渠道并阻止空提交", async () => {
  const confirm = vi.fn();
  render(<BatchDispositionDialog mode="published" count={3} channels={{ hugo: true, wechat: true }} onClose={vi.fn()} onConfirm={confirm} />);
  expect(screen.getByRole("button", { name: "确认标记" })).toBeDisabled();
  await userEvent.click(screen.getByRole("checkbox", { name: "Hugo" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "微信" }));
  await userEvent.click(screen.getByRole("button", { name: "确认标记" }));
  expect(confirm).toHaveBeenCalledWith(["hugo", "wechat"]);
});
```

另测未配置渠道禁用并出现“前往设置”、ignored 文案说明跨内容更新、busy 时 Escape/背景点击不关闭、关闭后由父组件恢复焦点。

- [ ] **Step 2: 运行对话框测试确认红灯**

Workdir: `web/app`

Run: `npm test -- --run src/components/BatchDispositionDialog.test.tsx`

Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现对话框和 API Client**

```ts
export function batchDisposition(command: BatchDispositionCommand) {
  return request<BatchDispositionResult>("/articles/batch-disposition", {
    method: "POST",
    body: JSON.stringify(command),
  });
}
```

对话框复用 `.dialog-backdrop`，独立 `.disposition-dialog` 最大宽度 440px；用 Lucide `X`、`Check`、`EyeOff`，所有图标按钮有 tooltip 或明确 aria-label。

- [ ] **Step 4: 写内容库成功与失败流程测试**

成功用例断言请求体包含选中文章 ID/版本和两个渠道，成功后 checkbox 清空、列表重新 GET、Toast 显示数量。失败用例让 POST 返回 409，断言选择保留且显示“部分文章已更新，请刷新后重新选择”。ignored 筛选下只显示 `恢复管理` 并提交 `operation: restore`。

- [ ] **Step 5: 实现内容库批量命令**

```tsx
const selectedArticles = (items ?? []).filter((item) => selected.has(item.id));
const articles = selectedArticles.map((item) => ({ id: item.id, content_version: item.content_version }));

async function applyDisposition(operation: "published" | "ignored" | "restore", channels: Array<"hugo" | "wechat"> = []) {
  const result = await batchDisposition({ operation, articles, ...(channels.length ? { channels } : {}) });
  setSelected(new Set());
  setDialog(null);
  setReloadKey((value) => value + 1);
  toast.show({ kind: "success", message: `已处理 ${result.processed} 篇文章` });
}
```

`LibraryPage` 增加 `const [reloadKey, setReloadKey] = useState(0)`，并把 `reloadKey` 放入首屏列表请求 effect dependency；失败分支不更新 reloadKey、items 或 selected。

对话框直接使用 Task 4 的 `ArticlePage.available_channels`，列表首屏保存该数组，加载更多不覆盖能力；这样不额外请求包含路径和诊断信息的 Settings API。

- [ ] **Step 6: 写文章详情处置 DTO 后端失败测试**

`runtime_test.go` 为当前版本 published 和 ignored 各建一条处置记录，断言：

```json
"disposition":{"kind":"published","channels":["hugo","wechat"]}
```

stale published 不返回 disposition；ignored 返回空 channels。

- [ ] **Step 7: 实现文章详情查询和提示**

`articleDetail` 通过参数化查询读取有效 Disposition；published 渠道从当前版本、state=`published` 的 Provider 投影派生，不返回 Provider ID。React 在 PublicationTrack 下方渲染非卡片提示 band：

```tsx
const channelText = { hugo: "Hugo", wechat: "微信" } as const;

{article.disposition && <p className={`article-disposition state-${article.disposition.kind}`}>
  {article.disposition.kind === "ignored"
    ? "此文章已忽略，可在内容库恢复"
    : `当前版本已标记为外部发表：${article.disposition.channels.map((channel) => channelText[channel]).join("、")}`}
</p>}
```

- [ ] **Step 8: 运行后端详情与前端批量流程测试确认绿灯**

Run: `go test ./internal/transport/http -run 'ArticleDetail.*Disposition'`

Workdir: `web/app`

Run: `npm test -- --run src/components/BatchDispositionDialog.test.tsx src/pages/library/library-page.test.tsx src/pages/article/article-workflow.test.tsx`

Run: `npm run typecheck`

Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add internal/transport/http/runtime.go internal/transport/http/runtime_test.go web/app/src/components/BatchDispositionDialog.tsx web/app/src/components/BatchDispositionDialog.test.tsx web/app/src/pages/library/LibraryPage.tsx web/app/src/pages/library/library-page.test.tsx web/app/src/api/client.ts web/app/src/api/types.ts web/app/src/pages/article/ArticlePage.tsx web/app/src/pages/article/article-workflow.test.tsx web/app/src/styles/app.css
git commit -m "feat(library): batch publish and ignore articles"
```

---

### Task 9: 开发验收数据、浏览器回归与生产资源

**Files:**
- Modify: `web/app/vite.config.ts`
- Modify: `web/app/e2e/workflows.spec.ts`
- Modify: `web/app/e2e/screenshots.spec.ts`
- Modify: `web/dist/*`

**Interfaces:**
- Consumes: 完整 Dashboard、ArticlePage 和批量处置 HTTP 契约。
- Produces: 四视口可重复 E2E 和最新嵌入式生产资源。

- [ ] **Step 1: 写 E2E 失败流程**

在 Demo API 为每篇文章增加 `content_version`，Dashboard 返回四组；用模块级 Set 保存 ignored 和每渠道 published。新增 E2E：

```ts
test("内容库批量标记渠道已发表并从工作台最近处理查看", async ({ page }) => {
  await page.goto("/library");
  await page.getByRole("checkbox", { name: /选择文章 构建可靠/ }).click();
  await page.getByRole("checkbox", { name: /选择文章 从笔记到发布/ }).click();
  await page.getByRole("button", { name: "标记已发表" }).click();
  await page.getByRole("checkbox", { name: "Hugo" }).click();
  await page.getByRole("checkbox", { name: "微信" }).click();
  await page.getByRole("button", { name: "确认标记" }).click();
  await expect(page.getByText("已处理 2 篇文章")).toBeVisible();
  await page.getByRole("link", { name: "工作台" }).first().click();
  await expect(page.getByRole("heading", { name: "最近处理" })).toBeVisible();
});
```

再增加 ignore 后默认列表消失、筛选“已忽略”并批量恢复的 E2E。

- [ ] **Step 2: 运行 E2E 确认红灯**

Workdir: `web/app`

Run: `npx playwright test e2e/workflows.spec.ts --project=desktop`

Expected: FAIL，Demo API 与 UI 尚未完整联动或缺少新测试数据。

- [ ] **Step 3: 完成 Demo API 和截图页面**

`vite.config.ts` 对 POST `/articles/batch-disposition` 读取 JSON body，更新内存处置集合并返回 counts；后续 `/articles` 与 `/dashboard` 根据集合返回一致状态。`screenshots.spec.ts` 增加 `dashboard`、`library-selection` 和 `publish-dialog` 截图场景。

- [ ] **Step 4: 运行完整前端验证**

Workdir: `web/app`

Run: `npm run typecheck`

Run: `npm run lint`

Run: `npm test -- --run`

Run: `npm run build`

Run: `npx playwright test`

Expected: typecheck/lint/build exit 0，Vitest 全部 PASS，Playwright 在 desktop/tablet/mobile/small-mobile 全部 PASS，无严重 axe 违规和横向溢出。

- [ ] **Step 5: 检查截图和响应式布局**

查看 Playwright 输出中的 Dashboard、内容库选择态和确认对话框截图，确认 1440×900、1024×768、390×844、320×568 下：文本不遮挡、checkbox 不挤压标题、批量栏不覆盖移动底栏、对话框可滚动且按钮可见。

- [ ] **Step 6: 运行完整 Go 回归**

Run: `go test ./...`

Expected: 所有 Go package PASS。

Run: `go test -race ./internal/app/... ./internal/storage/sqlite/... ./internal/transport/http/...`

Expected: 所有相关 package PASS，无 data race。

- [ ] **Step 7: 复查改动范围**

Run: `git diff --check`

Run: `git status --short`

Run: `git diff --stat`

Expected: 无空白错误；只包含本计划文件、Go/React 源码、测试和 `web/dist` 构建产物，不包含数据库、日志、Playwright 临时文件或用户无关改动。

- [ ] **Step 8: 提交最终验收资源**

```bash
git add web/app/vite.config.ts web/app/e2e web/dist
git commit -m "test(ui): cover article disposition workflows"
```

---

## 最终验收清单

- [ ] `/api/v1/dashboard` 不再硬编码，四组来自最近工作区真实数据。
- [ ] 默认内容库排除 ignored，搜索、审核、处置和 cursor 可以组合。
- [ ] published 多渠道事务写入处置、Publication 和 Event，重复命令无重复 Event。
- [ ] ignored 跨内容变化持续隐藏，restore 后重新参与工作台分类。
- [ ] 任一版本冲突或无效文章导致整批回滚并返回稳定错误。
- [ ] 内容库批量选择不跨未加载分页，失败保留选择，成功刷新并清空。
- [ ] 文章详情提示有效处置，不泄露 hash、Provider ID 或路径。
- [ ] 数据库新表与每个字段都有可见中文 comment。
- [ ] Go 全量与 race、React 测试/typecheck/lint/build、四视口 Playwright 全部通过。
- [ ] `git diff` 和 `git status` 只包含预期文件。
