# InkHub

InkHub 是面向 Markdown 内容创作者的本地优先内容工作台：从 Obsidian Vault 建立索引，在本机完成元数据审核、AI 建议、SEO 检查和标签治理，并将确认后的内容交付到 Hugo 与微信公众号。

```text
Obsidian Vault → InkHub 审核与治理 → Hugo / 微信公众号草稿
```

正文始终保存在 Vault。SQLite 只保存可重建索引、配置、审核历史、发布状态和后台任务；AI 建议不会自动覆盖文章，微信公众号也不会被误报为自动发布。

## 快速开始

要求：Go 1.24。只有修改前端时才需要 Node.js 22；运行发布二进制不依赖 Node.js。

```bash
go build -o ./bin/inkhub ./cmd/inkhub
./bin/inkhub
```

默认访问地址为 `http://127.0.0.1:8080`。首次打开后依次选择 Obsidian Vault、可选 Hugo、微信模板和 AI 配置。服务默认只监听本机回环地址。

常用启动参数：

```bash
./bin/inkhub --host 127.0.0.1 --port 8080
./bin/inkhub --data-dir /path/to/inkhub-data
./bin/inkhub --version
./bin/inkhub doctor
```

## 启动与日志配置

InkHub 启动时读取当前目录的 `.env`，进程环境变量优先于文件值。可以从 `.env.example` 开始配置：

```dotenv
INKHUB_DATA_DIR="/Users/your-name/Library/Application Support/InkHub"
INKHUB_LOG_LEVEL=info
INKHUB_LOG_FILE=
INKHUB_LOG_MAX_SIZE=100
INKHUB_LOG_CONSOLE=true
```

`INKHUB_DATA_DIR` 必须使用绝对路径，不支持 `~`。数据目录优先级为显式 `--data-dir`、`INKHUB_DATA_DIR`、操作系统默认目录；修改目录不会自动迁移旧数据库。启动日志会记录最终数据目录及其来源，便于确认当前实例使用的数据位置。

日志文件为空时默认写入 `<data-dir>/logs/inkhub.log`。单文件默认最大 100 MiB，保留 5 个备份和 30 天并压缩；文件内容为 JSON，控制台为易读格式。日志记录请求、扫描、数据库和后台任务的稳定 ID、错误码、Provider 错误分类、重试属性与耗时；HTTP 错误还会关联 `request_id`、路由和响应状态。不记录文章正文、Secret、完整 AI 请求、完整上游响应或微信 HTML。

## 数据与隐私

- 默认数据目录：macOS 为 `~/Library/Application Support/InkHub`，其他系统使用各自用户配置目录。
- 主数据库：`inkhub.db`；migration 前会在 `backups/` 创建一致性备份。
- Secret 由平台 Secret Store 管理，不进入 SQLite、普通日志、API 响应或诊断包。
- 前端、SQLite migration 和内置微信模板均嵌入最终二进制。
- 写请求要求同源 JSON，HTTP 服务拒绝非本机 Host。

## 内容与渠道

标准 frontmatter 字段：`id`、`title`、`description`、`tags`、`keywords`、`publish.category`、`publish.series`、`publish.slug` 和 `publish.cover`。文章默认是草稿；只有明确写入以下字段才进入审核与发布工作流：

```yaml
publish:
  status: ready
```

内容库始终保留全部文章，并可按“已就绪/草稿”筛选。工作台只显示已就绪文章的下一步行动；内容版本由内容哈希判断，文件修改时间只用于排序。微信公众号已经人工确认的草稿是终态，后续正文变化不会自动重复发布。

Hugo 使用 staging、真实构建和原子替换，同一文章会稳定更新同一 page bundle。Taxonomy 以 Hugo 配置、文章 frontmatter 和 term 页面为权威来源，InkHub 在 SQLite 中保存最近成功快照。

微信公众号提供 `InkHub Default` 和 `InkHub Minimal` 两个同规格模板。流程严格区分准备内容、复制格式化 HTML 和人工确认草稿；含本地图片的文章需要先配置图片托管。

## 开发与验证

后端：

```bash
go test -race ./...
go vet ./...
go build ./cmd/inkhub
```

前端：

```bash
cd web/app
npm ci
npm run dev
npm run typecheck
npm run lint
npm test -- --run
npx playwright test
npm run build
```

Vite 构建输出到已跟踪的 `web/dist`。修改前端后必须重新构建并提交产物，确保干净检出的 Go 项目可以直接嵌入 UI。

## 项目结构

```text
cmd/inkhub/       应用入口
internal/app/     用例与任务编排
internal/domain/  领域模型和规则
internal/provider Provider 实现
internal/storage/ SQLite 与 Repository
internal/transport HTTP 和 CLI Adapter
web/app/          React + TypeScript 源码
web/dist/         已跟踪的生产资源
docs/             PRD、架构、交互和实施计划
```

更完整的产品范围与设计约束见 `docs/PRD.md`、`docs/design/architecture.md` 和 `docs/design/interactions.md`。
