# InkHub MVP Release 1 实施计划

> **面向执行代理：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，逐任务执行本计划。所有步骤使用复选框（`- [ ]`）跟踪状态。

**目标：** 构建 InkHub MVP Release 1：一个本地优先的 Go + React 内容工作台，可以扫描 Obsidian Vault，完成编辑审核、AI 建议和 taxonomy 治理，并为 Hugo 与微信公众号准备内容。

**架构：** 采用模块化 Go 单体，明确划分 Domain、Application、Provider、Infrastructure 和 Transport 边界。SQLite 保存索引和工作流历史，Vault 中的 Markdown 始终是正文权威来源；React 通过本机 HTTP API 工作，并嵌入最终二进制。

**技术栈：** Go 1.24、Gin 1.10、modernc SQLite 1.46.1、Goldmark 1.7、YAML v3、React 19.2.0、TypeScript 5.7.3、Vite 5.4.21、Vitest 2.1.9、Testing Library React 16.3.2、Playwright 1.61.1。

## 全局约束

- 产品名称使用 `InkHub`；CLI、对外命令、配置键和目录使用 `inkhub`。
- 新代码放在 `cmd/inkhub`、`internal` 和 `web/app` 下；旧应用统一位于 `old/`，禁止向其中增加 InkHub 行为。
- Markdown 正文不得进入 SQLite；Hugo `data/taxonomy.yaml` 是 MVP 唯一 taxonomy 权威来源。
- 稳定文章 ID 和包括 `keywords` 在内的标准元数据，只能在用户确认后写入 Obsidian frontmatter。
- 图片和附件不参与 MVP 内容 hash。
- Secret 不得进入 SQLite、API 响应、普通日志或诊断包。
- 每个公开 Go 方法都有中文文档注释；关键逻辑和补偿路径有简短中文注释。
- migration 中每张表和每个字段都必须在 `schema_comments` 中登记。
- HTTP 默认监听 `127.0.0.1`；路径必须规范化、解析符号链接并检查授权根目录。
- 每个任务采用 TDD，结束时执行聚焦验证；未启用连续开发模式时使用 Conventional Commit。
- 每个任务进入下一项前必须审查实现问题、中文注释覆盖、事务/补偿/安全边界，并运行全量 race、vet、构建和 diff 检查；存在未修复问题时不得标记完成。
- 只有设计文档审核并提交到 `codex/` 分支或隔离 worktree 后才开始执行；不得将用户已有改动混入任务提交。

---

## 计划文件结构

```text
cmd/inkhub/main.go                         # 新 CLI 入口
internal/domain/{article,editorial,...}/   # 纯领域模型与规则
internal/app/{workspace,editorial,...}/    # 用例、事务和任务编排
internal/provider/{registry,source,ai,publish}/
internal/content/{markdown,assets,mermaid}/
internal/storage/sqlite/{migrations,repository}/
internal/platform/{filesystem,process,secrets,dialog,...}/
internal/transport/{http,cli}/
web/app/                                   # React + TypeScript 源码
web/dist/                                  # 跟踪的 Vite 构建产物，供 Go embed
templates/wechat/{inkhub-default,inkhub-minimal}/
testdata/{obsidian,hugo,templates}/
```

## 阶段 A：可用基础

### 任务 1：新入口与构建骨架

**文件：**
- 新建: `cmd/inkhub/main.go`
- 新建: `internal/app/bootstrap/bootstrap.go`
- 新建: `internal/app/bootstrap/bootstrap_test.go`
- 新建: `internal/buildinfo/buildinfo.go`
- 修改: `go.mod`
- 修改: `.github/workflows/ci.yml`

**接口：**
- 产出：`bootstrap.Run(ctx context.Context, args []string) error`
- 保留：旧 `old/main.go` 在任务 14 前持续可构建。

- [ ] **步骤 1：编写失败的 bootstrap 测试**

```go
func TestRunVersionDoesNotOpenWorkspace(t *testing.T) {
    err := Run(context.Background(), []string{"inkhub", "--version"})
    if err != nil {
        t.Fatalf("Run() error = %v", err)
    }
}
```

- [ ] **步骤 2：确认新包尚不存在**

运行: `go test ./internal/app/bootstrap`
预期：失败，因为包或 `Run` 尚不存在。

- [ ] **步骤 3：增加最小命令骨架**

使用 `flag.FlagSet` 实现 `Run`，支持 `--version`、`--data-dir`、`--workspace`、`--host` 和 `--port`；`cmd/inkhub/main.go` 只负责安装信号取消并调用 `Run`，不包含业务逻辑。

- [ ] **步骤 4：修改 module 并扩展 CI**

将 module path 修改为与 Git 远端一致的 `github.com/gkmz/InkHub`，机械更新全部 import，并在 CI 中增加 `go vet ./...` 和 `go test -race ./...`。CI 固定安装 Hugo Extended 0.163.3，用于 Provider 集成测试。不要删除旧包。

- [ ] **步骤 5：验证新旧两个入口**

运行: `go test ./... && go build ./cmd/inkhub && go build -o /tmp/inkhub-old ./old`
预期：通过；新旧二进制均可构建。

- [ ] **步骤 6：提交**

运行: `git commit -m "chore: establish inkhub application skeleton"`

### 任务 2：领域模型与状态规则

**文件：**
- 新建: `internal/domain/article/{article.go,hash.go,article_test.go,hash_test.go}`
- 新建: `internal/domain/editorial/{state.go,state_test.go}`
- 新建: `internal/domain/publication/{publication.go,publication_test.go}`
- 新建: `internal/domain/taxonomy/{taxonomy.go,taxonomy_test.go}`
- 新建: `internal/domain/job/{job.go,job_test.go}`

**接口：**
- 产出：`article.Article`、`article.NormalizeAndHash`、`editorial.Transition`、`publication.DisplayState`、`taxonomy.ValidateTags`、`job.Job`。
- 只能导入 Go 标准库和同级 Domain 包。

- [ ] **步骤 1：编写表驱动失败测试**

覆盖确定性 LF/BOM 规范化、`keywords` 改变内容 hash、图片字节不改变 hash、稳定 ID 校验、全部合法审核转换、发布过期状态派生、Tag alias/数量规则和任务转换。

- [ ] **步骤 2：确认 Domain 测试失败**

运行: `go test ./internal/domain/...`
预期：失败，提示领域类型和函数未定义。

- [ ] **步骤 3：实现不可变值对象和规则**

使用强类型 ID 和枚举；返回校验错误而不是布尔值。hash 输入使用带版本的规范 JSON 包装，包含正文、标题、描述、Tags、Keywords、Category、Series、Slug 和 Cover。

- [ ] **步骤 4：验证依赖纯净度和行为**

运行: `go test -race ./internal/domain/... && go list -deps ./internal/domain/... | rg 'gin|sqlite|goldmark'`
预期：测试通过；依赖搜索无输出。

- [ ] **步骤 5：提交**

运行: `git commit -m "feat(domain): define article and workflow rules"`

### 任务 3：SQLite Migration、Repository 与备份

**文件：**
- 新建: `internal/storage/sqlite/db.go`
- 新建: `internal/storage/sqlite/migrate.go`
- 新建: `internal/storage/sqlite/backup.go`
- 新建: `internal/storage/sqlite/migrations/0001_init.sql`
- 新建: `internal/storage/sqlite/migrations/comments_test.go`
- 新建: `internal/storage/sqlite/repository/*.go`
- 新建: `internal/storage/sqlite/sqlite_test.go`

**接口：**
- 产出：`sqlite.Open`、`sqlite.Migrate`、`sqlite.Backup`，以及与架构设计一致的 Repository 构造函数。
- 使用：任务 2 的领域对象。

- [ ] **步骤 1：增加 SQLite 驱动和失败的 migration 测试**

运行: `go get modernc.org/sqlite@v1.46.1`

测试空库迁移、重复迁移、checksum 不匹配、外键、跨工作区拒绝、稳定 ID 不可复用、活动任务去重，以及每张表/字段均有 comment。

- [ ] **步骤 2：确认 migration 测试失败**

运行: `go test ./internal/storage/sqlite/...`
预期：失败，因为 schema 和 migration runner 尚不存在。

- [ ] **步骤 3：实现嵌入式 migration 和连接策略**

打开 SQLite 时启用外键、busy timeout、WAL、有限连接数、UTC 时间编码和事务 migration。使用 `//go:embed migrations/*.sql` 嵌入脚本。

- [ ] **步骤 4：实现职责聚焦的 Repository**

每个聚合使用一个 Repository 文件。Publication 投影和事件必须在同一事务提交；Repository 方法返回领域类型，禁止返回 SQL row 或 `map[string]any`。

- [ ] **步骤 5：实现备份和恢复校验**

在 migration 和每日首次重要写入前备份；恢复时先对临时数据库执行 `PRAGMA integrity_check`，通过后再原子替换。

- [ ] **步骤 6：验证存储行为**

运行: `go test -race ./internal/storage/sqlite/...`
预期：通过，包括空库、已有 schema 和回滚测试。

- [ ] **步骤 7：提交**

运行: `git commit -m "feat(storage): add sqlite persistence and backups"`

### 任务 4：平台、配置与安全边界

**文件：**
- 新建: `internal/platform/filesystem/{paths.go,atomic.go,atomic_test.go}`
- 新建: `internal/platform/process/{runner.go,runner_test.go}`
- 新建: `internal/platform/secrets/{store.go,env.go,keyring.go,keyring_test.go}`
- 新建: `internal/platform/dialog/{picker.go,picker_darwin.go,picker_linux.go,picker_windows.go}`
- 新建: `internal/platform/{clipboard,browser}/`
- 新建: `internal/app/bootstrap/config.go`
- 新建: `internal/app/bootstrap/config_test.go`

**接口：**
- 产出：`filesystem.AuthorizedFS`、`process.Runner`、`secrets.Store`、`dialog.DirectoryPicker` 和合并后的 `bootstrap.Config`。

- [ ] **步骤 1：编写安全优先的失败测试**

覆盖符号链接逃逸、同前缀兄弟路径、原子替换保留权限、命令参数隔离、配置优先级、Keychain/环境变量 Secret 降级、Secret 脱敏，以及目录选择器不可用时的降级行为。

- [ ] **步骤 2：确认测试失败**

运行: `go test ./internal/platform/... ./internal/app/bootstrap`
预期：失败，因为平台契约尚不存在。

- [ ] **步骤 3：实现平台接口和安全默认值**

使用参数数组而不是 shell 字符串。授权前解析真实路径。加入 `github.com/zalando/go-keyring@v0.2.8`；优先使用操作系统 Keychain，不可用时读取环境变量，禁止使用明文 SQLite。

- [ ] **步骤 4：实现配置优先级**

按 CLI > 环境变量 > `.inkhub/config.yaml` > SQLite > 默认值合并配置，同时保留来源元数据供 Doctor 输出。

- [ ] **步骤 5：验证跨平台编译**

运行: `go test ./internal/platform/... ./internal/app/bootstrap && GOOS=linux go build ./internal/platform/... && GOOS=windows go build ./internal/platform/...`
预期：当前平台测试通过，Linux/Windows 包可交叉编译，且不执行原生目录选择器。

- [ ] **步骤 6：提交**

运行: `git commit -m "feat(platform): add secure local system boundaries"`

### 任务 5：Obsidian Source Provider 与增量扫描

**文件：**
- 新建: `internal/provider/source/obsidian/{provider.go,scan.go,frontmatter.go,write.go,watch.go}`
- 新建: `internal/provider/source/obsidian/*_test.go`
- 新建: `testdata/obsidian/{valid,invalid,duplicate-id,moved}/`
- 新建: `internal/app/workspace/{initialize.go,scan.go,scan_test.go}`

**接口：**
- 实现：`provider-contracts.md` 中的 `SourceProvider`。
- 产出：`workspace.InitializeWorkspace`、`workspace.ScanWorkspace`。

- [ ] **步骤 1：创建黄金样例和失败的契约测试**

包含带 `keywords` 的固定 frontmatter、未知字段、WikiLink、callout、中文路径、无效 YAML、重复 ID、移动/删除，以及写回元数据后正文和无关字段逐字节不变的样例。

- [ ] **步骤 2：确认测试失败**

运行: `go test ./internal/provider/source/obsidian ./internal/app/workspace`
预期：失败，因为 Provider 和用例尚不存在。

- [ ] **步骤 3：实现解析、扫描和乐观并发元数据写回**

使用 YAML Node API 保留字段顺序。扫描输出 fingerprint，且不得覆盖重复稳定 ID。写回时校验预期 fingerprint，并通过 `AuthorizedFS` 原子替换。

- [ ] **步骤 4：实现监听事件合并与降级**

合并重复文件事件，忽略 InkHub 临时文件并排队增量扫描；监听失败时切换到定时扫描，不能丢失已索引内容。

- [ ] **步骤 5：验证完整 Source 契约**

运行: `go test -race ./internal/provider/source/obsidian ./internal/app/workspace`
预期：黄金样例、移动身份保持、重复 ID 阻断和写冲突测试全部通过。

- [ ] **步骤 6：提交**

运行: `git commit -m "feat(obsidian): scan and update vault articles"`

### 任务 6：Taxonomy、编辑检查与 SEO

**文件：**
- 新建: `internal/app/taxonomy/{load.go,approve.go,govern.go}`
- 新建: `internal/app/editorial/{check.go,review.go}`
- 新建: `internal/content/markdown/{parse.go,checks.go}`
- 新建: matching `*_test.go`
- 新建: `testdata/hugo/taxonomy/data/taxonomy.yaml`

**接口：**
- 产出：`taxonomy.LoadAuthoritative`、`taxonomy.ApproveTerm`、`editorial.CheckArticle`、`editorial.ReviewArticle`。

- [ ] **步骤 1：编写失败的 taxonomy 与检查器测试**

覆盖 alias 规范化、未知/低频 Tag、3-6 个推荐规则、YAML diff、taxonomy 优先写入补偿、标题层级、图片缺失、无效 Slug、Description、Keywords 和渠道特有检查。

- [ ] **步骤 2：确认测试失败**

运行: `go test ./internal/app/taxonomy ./internal/app/editorial ./internal/content/markdown`
预期：失败，因为用例尚不存在。

- [ ] **步骤 3：在 AI 前实现确定性规则**

返回强类型 `blocking/recommended/optional/passed` 发现项。批准操作先原子写 taxonomy，重新加载为权威事实，再写文章元数据；文章部分失败必须可以重试。

- [ ] **步骤 4：验证检查与补偿**

运行: `go test -race ./internal/app/taxonomy ./internal/app/editorial ./internal/content/markdown`
预期：通过，包括 taxonomy 写入失败和文章部分写入失败。

- [ ] **步骤 5：提交**

运行: `git commit -m "feat(editorial): add taxonomy and content review"`

### 任务 7：Provider Registry 与 OpenAI-Compatible AI

**文件：**
- 新建: `internal/provider/contracts/*.go`
- 新建: `internal/provider/registry/{registry.go,registry_test.go}`
- 新建: `internal/provider/ai/openai/{provider.go,client.go,provider_test.go}`
- 新建: `internal/app/editorial/{suggest.go,accept.go,suggest_test.go}`

**接口：**
- 严格实现 `provider-contracts.md` 中的类型化工厂和 AI 契约。
- 产出：`editorial.GenerateSuggestions`、`editorial.AcceptSuggestion`。

- [x] **步骤 1：编写共享契约和 HTTP Fake 测试**

测试稳定 Descriptor、重复注册、配置 schema 拒绝、超时、429 可重试性、响应大小限制、无效 JSON、隐私模式、过期内容 hash 和未知 Tag 候选。

- [x] **步骤 2：确认测试失败**

运行: `go test ./internal/provider/... ./internal/app/editorial`
预期：失败，因为 Registry 和 AI 实现缺失。

- [x] **步骤 3：实现类型化 Registry 和 AI Client**

使用有限制的 HTTP Client、明确的重定向策略、结构化 JSON Schema、脱敏错误和 `SecretResolver`。校验完成后丢弃原始响应。

- [x] **步骤 4：实现字段级采用**

重新校验文章 content hash，应用单个类型化元数据 patch，新 Tag 必须经过 taxonomy 准入，并持久化建议处理结果。

- [x] **步骤 5：验证 Provider 契约**

运行: `go test -race ./internal/provider/... ./internal/app/editorial`
预期：无需网络即可通过。

- [ ] **步骤 6：提交**

运行: `git commit -m "feat(ai): add structured metadata suggestions"`

### 任务 8：持久化 Job Runner

**文件：**
- 新建: `internal/app/job/{runner.go,locks.go,recovery.go}`
- 新建: `internal/app/job/*_test.go`

**接口：**
- 产出：`job.Runner.Enqueue`、`job.Runner.Cancel`、`job.Runner.Recover` 及文章/Provider 锁获取能力。

- [x] **步骤 1：编写失败的生命周期测试**

覆盖去重、进度、重试退避、取消、文章/Provider 串行化、重启恢复、原子替换不可盲目重试和优雅关闭。

- [x] **步骤 2：确认测试失败**

运行: `go test ./internal/app/job`
预期：失败，因为 Runner 尚不存在。

- [x] **步骤 3：实现有限 Worker Runner**

使用 SQLite 领取/更新事务、context 取消、类型化 Handler 和确定性去重键。外部工作期间不得持有数据库事务。

- [x] **步骤 4：验证竞态和重启行为**

运行: `go test -race -count=20 ./internal/app/job`
预期：通过，且 Handler 不会重复执行。

- [ ] **步骤 5：提交**

运行: `git commit -m "feat(jobs): add recoverable background execution"`

## 阶段 B：核心产品价值

### 任务 9：Hugo Publish Provider

**文件：**
- 新建: `internal/provider/publish/hugo/{provider.go,convert.go,bundle.go,staging.go,build.go,recover.go}`
- 新建: matching `*_test.go`
- 新建: `testdata/hugo/site/`

**接口：**
- 实现：带 OperationID 幂等性的发布 `Preflight`、`Prepare` 和 `Deliver`。

- [ ] **步骤 1：构建最小 Hugo Fixture 和失败集成测试**

测试按 source ID 创建/更新同一 bundle、元数据/Keywords 映射、WikiLink/callout 转换、图片冲突、taxonomy 失败、构建失败、原子替换失败、恢复和预览 URL。

- [ ] **步骤 2：确认测试失败；仅 Hugo 不可用时允许跳过集成测试**

运行: `go test ./internal/provider/publish/hugo`
预期：单元测试初始失败；CI 中集成测试要求 Hugo Extended 0.163.3，本机仅在缺少 Hugo 时明确跳过。

- [ ] **步骤 3：在 staging 中实现 Prepare**

在目标文件系统创建 Job 级临时站点，转换内容/资源、校验 taxonomy、使用参数数组运行 Hugo，并在不修改真实 bundle 的前提下返回有期限的 Artifact。

- [ ] **步骤 4：实现 Deliver 与补偿**

备份目标 bundle、原子替换、构建真实站点，失败时恢复，并返回实际源 content hash/revision。重复 OperationID 返回已记录结果。

- [ ] **步骤 5：验证 Hugo 行为**

运行: `go test -race ./internal/provider/publish/hugo`
预期：通过；存在 Hugo 时运行全部集成测试。

- [ ] **步骤 6：提交**

运行: `git commit -m "feat(hugo): add idempotent blog synchronization"`

### 任务 10：微信模板引擎与 Publish Provider

**文件：**
- 新建: `internal/domain/template/*.go`
- 新建: `internal/provider/publish/wechat/{provider.go,render.go,inline.go,sanitize.go,assets.go}`
- 新建: `internal/app/template/{install.go,update.go,repository.go}`
- 新建: `templates/wechat/{inkhub-default,inkhub-minimal}/`
- 新建: `testdata/templates/{valid,invalid}/`
- 迁移行为来源：`old/markdown/processor.go`、`old/services/mermaid.go`、`old/services/uploader.go`、`old/web/static/css/wechat.css`

**接口：**
- 产出：规范 1.0 校验器/安装器、Default/Minimal 模板、WeChat Publish Provider。

- [ ] **步骤 1：编写失败的 Manifest 与安全测试**

覆盖 YAML 重复键/alias、Zip Slip、文件限制、摘要不匹配、主动内容格式、Selector/属性/值允许列表、Boolean 映射、不安全 HTML、脚本 URL、版本不可变、更新回滚和索引降级。

- [ ] **步骤 2：编写渲染黄金测试**

Default、Minimal 和第三方 Fixture 使用同一标准预览。断言最终 HTML 不包含 style/script、本地路径或未解析 Token，并且各模板输出不同且确定。

- [ ] **步骤 3：确认测试失败**

运行: `go test ./internal/domain/template ./internal/app/template ./internal/provider/publish/wechat`
预期：失败，因为校验器和 Provider 尚不存在。

- [ ] **步骤 4：实现校验器和原子安装器**

结构化解析 YAML/CSS，流式限量解压 zip，校验全部摘要、许可证和媒体签名，写入不可变版本目录，并通过 SQLite 事务激活。内置索引使用 `https://raw.githubusercontent.com/gkmz/InkHub/main/templates/index.json`，同时保留高级自定义索引设置。

- [ ] **步骤 5：实现微信 Prepare 与 Deliver**

渲染 Markdown/Mermaid，列出图片供确认，按内容寻址键上传，内联允许的 CSS，清理 HTML，创建预览 Artifact；只有用户直接触发 Deliver 时才能复制。

- [ ] **步骤 6：验证全部模板与微信测试**

运行: `go test -race ./internal/domain/template ./internal/app/template ./internal/provider/publish/wechat`
预期：两个内置模板通过同一管线测试。

- [ ] **步骤 7：提交**

运行: `git commit -m "feat(wechat): add secure template publishing"`

### 任务 11：Application 用例、HTTP API 与 CLI

**文件：**
- 新建: `internal/app/publication/{hugo.go,wechat.go,confirm.go}`
- 新建: `internal/transport/http/{router.go,middleware.go,workspace.go,articles.go,editorial.go,publication.go,templates.go,jobs.go}`
- 新建: `internal/transport/http/*_test.go`
- 新建: `internal/transport/cli/{root.go,doctor.go,scan.go,backup.go,template.go}`
- 修改: `internal/app/bootstrap/bootstrap.go`

**接口：**
- 产出：带版本的 `/api/v1` JSON API，以及 `init`、`doctor`、`scan`、`db backup`、`template init`、`template validate` CLI 命令。

- [ ] **步骤 1：编写失败的 HTTP 边界测试**

测试同源写请求、本机默认监听、输入校验、设置/诊断之外不返回绝对路径、稳定错误码、Cursor 分页、长任务 Job ID、过期内容冲突和脱敏。

- [ ] **步骤 2：确认测试失败**

运行: `go test ./internal/transport/... ./internal/app/publication`
预期：失败，因为路由和发布编排尚不存在。

- [ ] **步骤 3：实现用例编排**

只允许审核当前 content hash；长任务进入队列；持有文章/Provider 锁；Provider 返回后提交 Publication/Event；微信复制和确认保持为独立命令。

- [ ] **步骤 4：实现薄 HTTP 与 CLI Adapter**

Handler 只负责解码、校验、调用一个用例和映射类型化错误。CLI 调用相同用例，并提供面向用户的非 JSON 输出。

- [ ] **步骤 5：验证 API 与 CLI**

运行: `go test -race ./internal/transport/... ./internal/app/publication && go run ./cmd/inkhub --help`
预期：测试通过，帮助信息列出全部 MVP 命令。

- [ ] **步骤 6：提交**

运行: `git commit -m "feat(api): expose inkhub workflows"`

### 任务 12：React 基础、初始化与内容库

**文件：**
- 新建: `web/app/{package.json,tsconfig.json,vite.config.ts,index.html}`
- 新建: `web/app/src/{main.tsx,app.tsx,api/,styles/,components/,pages/}`
- 新建: `web/app/src/pages/{setup,workspace,library}/`
- 修改: `.github/workflows/ci.yml`

**接口：**
- 使用：任务 11 的 `/api/v1` Endpoint。
- 产出：响应式 Shell、四项导航、初始化向导、工作台、支持搜索/筛选的内容库。

- [ ] **步骤 1：搭建固定版本的 React 工具链**

创建 `dev`、`build`、`test`、`typecheck`、`lint` 脚本；固定 React 19.2.0、TypeScript 5.7.3、Vite 5.4.21、`@vitejs/plugin-react` 4.3.4、Vitest 2.1.9、jsdom 25.0.1、Testing Library React 16.3.2、DOM 10.4.1、user-event 14.6.1、ESLint 9.39.1、typescript-eslint 8.48.0、Lucide React 0.468.0、Playwright 1.61.1 和 `@axe-core/playwright` 4.11.0。提交 `package-lock.json`。Vite 输出配置为跟踪的 `web/dist`，使干净检出的 Go 项目无需 Node 运行时即可嵌入并构建。

- [ ] **步骤 2：编写失败的 UI 测试**

测试四步初始化、可选渠道跳过、空工作台、优先级排序、IME 安全搜索、筛选、脏表单导航保护、键盘焦点和移动端导航。

- [ ] **步骤 3：实现 Token 和应用 Shell**

实现 `interactions.md` 中精确的 `paper/surface/ink/muted/action/warning/danger/line` Token、稳定的桌面/平板/移动 Grid、可见焦点、减少动态效果支持，并禁止嵌套卡片。

- [ ] **步骤 4：实现初始化和列表流程**

使用类型化 API Client 和查询状态。目录按钮调用本机 DirectoryPicker Endpoint，同时保留手工路径输入。长扫描按 Job ID 恢复。

- [ ] **步骤 5：验证前端基础**

运行: `cd web/app && npm ci && npm run typecheck && npm test -- --run && npm run build`
预期：全部命令通过并生成 `web/dist`。

- [ ] **步骤 6：提交**

运行: `git commit -m "feat(ui): add workspace and content library"`

### 任务 13：React 审核、治理、模板与发布

**文件：**
- 新建: `web/app/src/pages/article/`
- 新建: `web/app/src/pages/taxonomy/`
- 新建: `web/app/src/pages/settings/`
- 新建: `web/app/src/pages/wechat-preview/`
- 新建: `web/app/src/components/{publication-track,metadata-form,checks,ai-suggestions,job-status,template-picker}/`
- 新建: `web/app/e2e/*.spec.ts`
- 新建: `web/app/e2e/screenshots.spec.ts`

**接口：**
- 产出：`interactions.md` 定义的全部交互流程和验收状态。

- [ ] **步骤 1：编写失败的组件测试**

覆盖元数据 diff/保存冲突、逐字段采用 AI 建议、过期建议、Tag 准入/YAML diff、唯一主要操作、发布轨道、Hugo 失败、微信准备/复制/确认、模板降级和任务恢复。

- [ ] **步骤 2：实现文章审核和发布轨道**

构建内容优先的桌面布局，使用 336px 工具栏；移动端使用内容/审核/发布标签。诊断之外禁止显示内部 hash 或 Job ID。

- [ ] **步骤 3：实现治理、模板和设置**

使用全宽区段、真实预览图、分组保存操作、仅 Secret 状态、Doctor 状态、受影响文章预览和明确的破坏性文案。

- [ ] **步骤 4：增加 Playwright 关键旅程**

自动化初始化、无 AI 手工审核、采用 AI 字段、Hugo 同步、模板切换、微信复制/确认、文章变化导致过期状态、移动端导航和 axe 可访问性检查。

- [ ] **步骤 5：在规定视口验证 UI**

运行: `cd web/app && npm run typecheck && npm test -- --run && npx playwright test`
预期：在 1440×900、1024×768、390×844 和 320×568 下通过，不存在重叠、页面横向滚动、严重 axe 违规或焦点指示缺失。截取初始化、工作台、文章审核、Taxonomy、模板预览和微信预览截图，与 `interactions.md` 手工对比。

- [ ] **步骤 6：提交**

运行: `git commit -m "feat(ui): complete review and publishing workflows"`

### 任务 14：嵌入分发、旧代码清理与发布回归

**文件：**
- 新建: `internal/transport/http/static.go`
- 新建: `web/embed.go`
- 修改: `internal/app/bootstrap/bootstrap.go`
- 修改: `README.md`
- 修改: `.github/workflows/ci.yml`
- 新入口验收后删除整个 `old/` 目录；删除前确认所有保留行为均已迁移并通过黄金测试。

**接口：**
- 产出：一个嵌入 migration、React 资源和内置模板的 `inkhub` 二进制。

- [ ] **步骤 1：编写失败的嵌入和启动测试**

断言 migration/模板/UI 已嵌入，未知 UI 路由返回 React 入口，API 路由绝不落入静态文件，默认绑定本机地址，数据库 migration 缺失时进入只读恢复。

- [ ] **步骤 2：嵌入生产资源并完成生命周期**

在 `web/embed.go` 中使用 `//go:embed dist/*` 嵌入已跟踪的 React 资源；migration 和 Default/Minimal 模板使用包内嵌入。Embed pattern 禁止使用 `..`。装配单实例锁、优雅关闭、浏览器打开、任务恢复和文件监听生命周期。

- [ ] **步骤 3：删除旧代码前运行新入口验收**

运行: `go test -race ./... && go build -o ./bin/inkhub ./cmd/inkhub && ./bin/inkhub doctor`
预期：通过；Doctor 将已配置能力显示为正常/需要处理/未启用，且不泄露 Secret。

- [ ] **步骤 4：删除已替代的旧代码并更新文档**

只删除行为已经迁移并通过黄金测试的模块。重写 README，涵盖 InkHub 初始化、CLI、数据目录、备份、隐私、Hugo、微信模板和开发命令。

- [ ] **步骤 5：运行完整后端、前端和 E2E 回归**

运行: `cd web/app && npm ci && npm run typecheck && npm test -- --run && npm run build && npx playwright test && cd ../.. && test -z "$(find cmd internal web -name '*.go' -print0 | xargs -0 gofmt -l)" && go vet ./... && go test -race ./... && go build ./cmd/inkhub && git diff --exit-code -- web/dist`
预期：全部命令通过。

- [ ] **步骤 6：检查最终改动范围**

运行: `git diff --check && git status --short && git diff --stat`
预期：没有空白错误、Secret 或计划外生成文件；已跟踪的 `package-lock.json` 和 `web/dist` 与固定版本前端构建一致。

- [ ] **步骤 7：提交可发布 MVP**

运行: `git commit -m "feat: deliver inkhub mvp release 1"`

## 最终验收

- [ ] PRD 第 32 章每项标准都有通过的自动测试或有记录的手工测试。
- [ ] 空库和已有 SQLite schema 均能在自动备份后迁移，且 comment 完整。
- [ ] 从 Vault 重建索引不会丢失审核和发布历史。
- [ ] Hugo 与微信失败相互独立且可以恢复。
- [ ] Default 和 Minimal 使用同一模板校验/渲染路径。
- [ ] SQLite、诊断和普通日志中没有 Secret 或文章正文。
- [ ] Default 和 Minimal 均记录真实微信公众号粘贴验证结果。
- [ ] macOS 完整验证；Linux 和 Windows 可编译并通过平台契约测试。
