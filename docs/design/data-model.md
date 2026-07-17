# InkHub 数据模型设计

> 对应 PRD 1.4、架构设计 1.1，面向 MVP Release 1。

## 1. 目标与边界

本设计定义 InkHub 的标准文章模型、SQLite 持久化模型、状态机、事件、内容版本和任务模型。它只保存索引、配置、状态、历史和可重建缓存，不保存 Markdown 正文；正文始终由 Obsidian Vault 提供。

数据模型必须满足：

- 文章移动、改名和正文更新后仍能通过稳定 ID 追踪。
- 审核和各发布渠道分别判断是否过期。
- 外部文件写入、AI 请求和 Hugo 构建不占用数据库写事务。
- 发布记录和事件可审计，任务可恢复和重试。
- 可重建数据与不可完全重建数据明确区分。
- 空库和已有 schema 均可通过有序 migration 创建或升级。

## 2. 标准文章模型

### 2.1 Core Article

Article 是跨 Source Provider 和 Publish Provider 的内部标准模型。正文通过 `SourceLocator` 按需读取，不序列化进 SQLite。

```go
// Article 表示一篇可被 InkHub 管理的 Markdown 文章。
type Article struct {
    ID              ArticleID
    WorkspaceID     WorkspaceID
    SourceID        SourceID
    StableID        string
    RelativePath    string
    Title           string
    Description     string
    Category        string
    Series          string
    Tags            []string
    Keywords        []string
    Slug            string
    Cover           string
    SourceMTime     *time.Time
    SourceSize      int64
    ContentHash     ContentHash
    FrontmatterHash ContentHash
    IndexedAt       time.Time
    DeletedAt       *time.Time
}
```

`StableID` 是写入 Obsidian frontmatter 的 `id`，是跨路径变化的业务身份；`ArticleID` 是 SQLite 内部主键，不能暴露为内容身份。`ContentHash` 只对规范化正文和影响发布的标准元数据计算，图片、附件和纯索引字段变化暂不改变它。

### 2.2 Obsidian frontmatter

MVP 只读取和写回以下字段：

```yaml
---
id: article_01J2ABCDEF
title: 示例标题
description: 文章概要
tags:
  - go
  - 流式输出
keywords:
  - Go 流式输出
  - Markdown 发布
publish:
  category: AI应用开发
  series: 大模型应用开发入门
  slug: llm-streaming-guide
  cover: assets/cover.png
---
```

字段约束：`id`、`title`、`description`、`publish.category`、`publish.series`、`publish.slug`、`publish.cover` 为字符串；`tags` 和 `keywords` 为字符串数组；`series` 和 `cover` 可为空或省略；category 只能一个，series 最多一个。审核状态、渠道状态、content hash 和时间戳不得写回文章。

### 2.3 规范化与 hash

Source Provider 读取文章后执行确定性规范化：统一换行符为 LF、去除 UTF-8 BOM、保留正文语义空白、规范 frontmatter 字段顺序和值类型，并将 tags、keywords、category、series、slug、cover 纳入发布 hash。图片和附件不纳入 MVP 内容版本。规范化结果只在内存或任务临时目录中使用。

- `content_hash`：正文 + 影响所有渠道输出的标准元数据。
- `frontmatter_hash`：标准 frontmatter 的规范化表示，用于检测元数据变化。
- `source_fingerprint`：文件大小、修改时间和必要时的快速摘要，用于增量扫描优化。

hash 使用 SHA-256，小写十六进制保存。相同规范化输入必须产生相同 hash；hash 算法或规范化规则变更时，migration 不直接重写旧 hash，而由重新扫描任务更新。

## 3. SQLite 总体模型

### 3.1 表清单

| 表 | 用途 | 可重建性 |
| --- | --- | --- |
| `schema_migrations` | migration 版本 | 必须保留 |
| `schema_comments` | 表和字段的数据库可见说明 | 可由 migration 重建 |
| `workspaces` | 工作区和默认配置 | 不可完全重建 |
| `sources` | Obsidian 内容源 | 可由配置重建，配置本身需保护 |
| `articles` | 文章索引和当前快照 | 可重建 |
| `editorial_reviews` | 审核投影 | 不可完全重建 |
| `taxonomy_terms` | taxonomy 缓存和统计 | 可重建 |
| `article_taxonomies` | 文章与 taxonomy 关系 | 可重建 |
| `provider_instances` | Provider 配置和能力快照 | 不可完全重建，Secret 除外 |
| `publications` | 文章到渠道的当前处理投影 | 不可完全重建 |
| `publication_events` | 渠道处理事件 | 不可完全重建 |
| `ai_suggestions` | AI 建议及采纳结果 | 不可完全重建 |
| `taxonomy_snapshots` | Provider taxonomy 同步状态和最近成功 revision | 可从 Provider 重建 |
| `templates` | 目标明确的模板安装和版本 | 可从模板包重建部分信息 |
| `jobs` | 持久化后台任务 | 运行中任务需恢复 |
| `settings` | 非敏感本机设置 | 可重建 |

SQLite 使用不依赖 CGO 的驱动。所有时间保存为 UTC RFC3339 字符串，布尔值保存为 `0/1`，JSON 字段只用于 Provider 配置、能力快照和结构化诊断，不向 Repository 暴露 `map[string]any`。

### 3.2 主键与通用列

业务表使用 SQLite `TEXT` 主键，值为 UUID/ULID 字符串；这样日志、事件和跨设备导出不会依赖自增序号。所有需要审计的表统一包含：`created_at TEXT NOT NULL`、`updated_at TEXT NOT NULL`。删除优先使用 `deleted_at TEXT NULL` 软删除，历史表不删除；stable ID 永不复用，删除后重新发现原文件必须恢复原文章记录。

## 4. Schema 与字段目录

以下 SQL 是逻辑 schema 的规范。实际 migration 可拆分为多个文件，但不得改变字段语义。

```sql
CREATE TABLE schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  checksum TEXT NOT NULL,
  applied_at TEXT NOT NULL
);

CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  data_dir TEXT NOT NULL,
  last_used_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE sources (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  provider_type TEXT NOT NULL,
  root_path TEXT NOT NULL,
  config_json TEXT NOT NULL DEFAULT '{}',
  last_scan_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, provider_type),
  UNIQUE (id, workspace_id)
);

CREATE TABLE articles (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  source_id TEXT NOT NULL REFERENCES sources(id),
  stable_id TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  series TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  slug TEXT NOT NULL DEFAULT '',
  cover TEXT NOT NULL DEFAULT '',
  keywords_json TEXT NOT NULL DEFAULT '[]',
  source_mtime TEXT,
  source_size INTEGER NOT NULL DEFAULT 0,
  source_fingerprint TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  frontmatter_hash TEXT NOT NULL DEFAULT '',
  indexed_at TEXT NOT NULL,
  deleted_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (source_id, relative_path),
  UNIQUE (id, workspace_id),
  UNIQUE (stable_id, workspace_id),
  FOREIGN KEY (source_id, workspace_id) REFERENCES sources(id, workspace_id)
);

CREATE TABLE editorial_reviews (
  article_id TEXT PRIMARY KEY REFERENCES articles(id),
  state TEXT NOT NULL CHECK (state IN ('draft','incomplete','pending_review','approved','changed','blocked')),
  approved_content_hash TEXT,
  approved_frontmatter_hash TEXT,
  approved_at TEXT,
  approved_by TEXT,
  blocking_count INTEGER NOT NULL DEFAULT 0,
  recommended_count INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);

CREATE TABLE taxonomy_terms (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id),
  kind TEXT NOT NULL,
  external_key TEXT NOT NULL,
  name TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  usage_count INTEGER NOT NULL DEFAULT 0,
  source_revision TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  UNIQUE (provider_instance_id, kind, external_key),
  UNIQUE (id, workspace_id),
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id)
);

CREATE TABLE taxonomy_snapshots (
  provider_instance_id TEXT PRIMARY KEY REFERENCES provider_instances(id),
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  revision TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('ready','refreshing','failed')),
  complete INTEGER NOT NULL DEFAULT 0,
  last_error_code TEXT,
  last_error_message TEXT,
  last_attempt_at TEXT NOT NULL,
  last_success_at TEXT,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id)
);

CREATE TABLE article_taxonomies (
  article_id TEXT NOT NULL REFERENCES articles(id),
  taxonomy_term_id TEXT NOT NULL REFERENCES taxonomy_terms(id),
  workspace_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (article_id, taxonomy_term_id),
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  FOREIGN KEY (taxonomy_term_id, workspace_id) REFERENCES taxonomy_terms(id, workspace_id)
);

CREATE TABLE provider_instances (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  provider_type TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  config_json TEXT NOT NULL DEFAULT '{}',
  capabilities_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, provider_type),
  UNIQUE (id, workspace_id)
);

CREATE TABLE publications (
  id TEXT PRIMARY KEY,
  article_id TEXT NOT NULL REFERENCES articles(id),
  provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id),
  workspace_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('never','prepared','copied','confirmed','published','failed')),
  content_hash TEXT NOT NULL DEFAULT '',
  provider_revision TEXT NOT NULL DEFAULT '',
  last_error_code TEXT,
  last_error_message TEXT,
  last_processed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (article_id, provider_instance_id),
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id)
);

CREATE TABLE publication_events (
  id TEXT PRIMARY KEY,
  publication_id TEXT NOT NULL REFERENCES publications(id),
  event_type TEXT NOT NULL,
  content_hash TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE ai_suggestions (
  id TEXT PRIMARY KEY,
  article_id TEXT NOT NULL REFERENCES articles(id),
  input_content_hash TEXT NOT NULL,
  provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id),
  workspace_id TEXT NOT NULL,
  suggestion_json TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('pending','partially_accepted','accepted','rejected','expired','invalid')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id)
);

CREATE TABLE templates (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  template_id TEXT NOT NULL,
  version TEXT NOT NULL,
  source TEXT NOT NULL,
  manifest_json TEXT NOT NULL,
  install_path TEXT NOT NULL,
  target TEXT NOT NULL,
  format TEXT NOT NULL,
  renderer TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, template_id, version)
);

CREATE TABLE jobs (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  kind TEXT NOT NULL,
  dedupe_key TEXT,
  state TEXT NOT NULL CHECK (state IN ('queued','running','succeeded','failed','cancelled')),
  progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  payload_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}',
  error_code TEXT,
  error_message TEXT,
  attempts INTEGER NOT NULL DEFAULT 0,
  available_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE settings (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  key TEXT NOT NULL,
  value_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (workspace_id, key)
);
```

SQLite 连接必须开启 `PRAGMA foreign_keys = ON`。SQLite 不支持标准 `COMMENT ON TABLE/COLUMN`。为满足数据库层面可见的注释要求，每个 migration 必须同时写入 `schema_comments`：

```sql
CREATE TABLE schema_comments (
  object_type TEXT NOT NULL CHECK (object_type IN ('table','column')),
  object_name TEXT NOT NULL,
  comment TEXT NOT NULL,
  PRIMARY KEY (object_type, object_name)
);
```

`schema_comments` 的每一行对应一张表或一个字段，管理工具和 `inkhub doctor` 必须能够查询并展示它；新增字段没有对应 comment 时 migration 验证失败。`schema_migrations` 和 `schema_comments` 自身的表及字段也必须登记 comment。

## 5. 索引、约束与一致性

必须创建以下索引：

```sql
CREATE INDEX idx_articles_workspace_state_path
  ON articles(workspace_id, deleted_at, relative_path);
CREATE INDEX idx_articles_content_hash ON articles(content_hash);
CREATE INDEX idx_publications_article_state
  ON publications(article_id, state);
CREATE INDEX idx_publication_events_publication_time
  ON publication_events(publication_id, created_at);
CREATE INDEX idx_jobs_runnable
  ON jobs(state, available_at, created_at);
CREATE UNIQUE INDEX idx_jobs_active_dedupe
  ON jobs(workspace_id, dedupe_key) WHERE dedupe_key IS NOT NULL AND state IN ('queued','running');
```

应用层还必须验证：

- 同一 workspace 只能有一个 Obsidian、一个 Hugo 和一个 WeChat Provider 实例。
- `stable_id` 重复时文章进入阻断状态，不得审核或发布。
- `articles` 软删除后保留 `publications` 和事件历史。
- `Publication` 和对应 `publication_events` 在同一短事务中提交。
- 禁止跨 workspace 关联文章、Provider、模板和任务。

## 6. Editorial 状态机

```text
draft → incomplete → pending_review → approved
  │          │             │             │
  └──────────┴─────────────┴─────────────┴→ changed
                                           │
                                           └→ pending_review
```

- `draft`：已索引但尚未完成最小信息。
- `incomplete`：缺少必填元数据或存在解析问题。
- `pending_review`：检查已完成，等待人工确认。
- `approved`：当前 `content_hash` 和 `frontmatter_hash` 已被人工确认。
- `changed`：影响输出的正文或元数据发生变化，原审核不再适用。
- `blocked`：重复 ID、严重解析问题或不可恢复的安全/路径问题。

只有 `approved` 才允许进入 Publish Provider；渠道特有阻断问题由渠道任务单独报告，不修改全局审核为失败。

合法转移：首次扫描创建 `draft`；缺少必填信息或通用检查阻断时进入 `incomplete`；检查通过后进入 `pending_review`；人工确认进入 `approved`；影响输出的正文或元数据变化从 `approved` 进入 `changed`，重新检查后回到 `pending_review`；重复 ID、路径越权或不可解析 frontmatter 进入 `blocked`，修复问题后只能回到 `incomplete`，不得自动恢复审核通过。

## 7. Publication 状态与过期判定

每个 Article 与 Provider Instance 只有一条当前投影记录，所有变化追加到 `publication_events`。

`publications` 是渠道当前状态投影，`publication_events` 是发布成功和最终失败的审计事实。任务自动重试的中间失败不追加事件；耗尽重试或不可重试错误按 `job_id + attempt` 生成幂等失败事件，并将当前投影更新为 `failed`。失败写入不得清空最后一次成功的 `provider_revision`。

- `never`：从未处理。
- `prepared`：已生成渠道结果，但尚未复制/写入目标。
- `copied`：结果已交付到剪贴板或目标 staging。
- `confirmed`：用户确认人工草稿或目标结果已保存。
- `published`：Provider 明确完成发布动作；MVP 微信不会自动进入此状态。
- `outdated`：当前文章 `content_hash` 与记录 hash 不同；这是查询时派生的展示状态，不作为持久化状态写入。
- `failed`：最近一次处理失败，可重试。

过期判定是确定性的：当 `articles.content_hash != publications.content_hash` 且 publication 不是 `never` 时，状态显示为 `outdated`；任务失败优先显示为 `failed`，但仍保留 hash 差异供重试判断。成功处理必须写入实际使用的 hash，禁止用当前 hash 盲写。数据库中的 `state` 不包含 `outdated`，查询层按上述规则派生用户状态。

## 8. 任务模型

任务种类包括 `scan`、`analyze`、`taxonomy_refresh`、`hugo_preview`、`hugo_deliver`、`wechat_prepare` 和 `template_install`；`hugo_sync` 仅用于兼容升级前已持久化的任务。任务 payload 只保存可重建参数和目标 ID，不保存正文或 Secret。

任务执行规则：

1. 外部操作前以短事务创建 `queued` 任务并记录意图。
2. Runner 领取任务时原子更新为 `running` 并递增 `attempts`。
3. 可重试错误按退避时间写回 `queued`；不可重试错误写为 `failed`。
4. 成功结果与 Publication/Event 更新在一个短事务中提交。
5. 应用启动时，超时的 `running` 任务转为可恢复的 `queued`；无法安全恢复的任务标记 `failed`。
6. `dedupe_key` 防止同一文章、Provider 和 hash 同时执行冲突任务。
7. 用户重试最终失败的 Hugo Preview/Deliver 时，只允许匹配任务 ID、工作区、类型和 `failed` 状态的原任务原子重排为 `queued`；保留 attempts、payload 和确定性 ID，清理本次运行时间与错误，不创建第二个 Deliver。

文章级当前工作流查询只匹配最近工作区、请求文章、启用的 Hugo Provider 和当前内容 hash，Deliver 状态优先于 Preview。统一发布历史按 `created_at DESC, id DESC` 排序并使用绑定工作区与文章的不可见 cursor；非法、跨工作区或跨文章 cursor 必须拒绝。对外 DTO 只返回领域预览 ID、相对路径、阶段、进度和安全摘要，不返回 Job ID、数据库事件 ID、内容 hash、`result_json` 或绝对路径。

MVP 每个工作区每种 Provider 类型最多一个实例；该限制适用于 Obsidian、AI、Hugo 和 WeChat，后续多实例扩展需单独放宽唯一约束和默认实例规则。

## 9. Migration、备份与恢复

Migration 文件按 `0001_init.sql`、`0002_...sql` 递增，`schema_migrations` 记录版本、名称、SHA-256 checksum 和执行时间。启动时只允许顺序执行，已应用 migration 的 checksum 变化直接报错；`PRAGMA foreign_keys`、comment 完整性和 schema checksum 必须在启动检查中验证。

迁移流程：

1. 获取数据库迁移锁并检查可用磁盘空间。
2. 在同一数据目录生成带时间戳的数据库备份。
3. 开启数据库事务，执行 DDL、约束、索引和 `schema_comments`。
4. 验证 comment 完整性、外键和 schema checksum。
5. 提交 migration；失败则回滚并保留备份。

自动备份保留最近 7 至 14 份，手工备份不自动删除。恢复先复制到临时数据库执行完整性检查，再原子替换正式数据库；正文、Hugo 内容和 Vault 文件不由数据库恢复操作修改。

## 10. Repository 与事务边界

Application 只依赖以下 Repository：`WorkspaceRepository`、`ArticleRepository`、`EditorialRepository`、`TaxonomyCacheRepository`、`ProviderInstanceRepository`、`PublicationRepository`、`SuggestionRepository`、`TemplateRepository` 和 `JobRepository`。Repository 返回领域对象，不返回 SQL row、裸 JSON map 或 driver 类型。

网络请求、文件写入、剪贴板、Hugo build 和图片上传都在事务外执行。它们开始前保存任务意图，完成后以短事务提交结果；文件和数据库之间使用 staging、原子替换、幂等目标路径和失败补偿保证最终一致。

## 11. 测试验收

- 空库执行全部 migration，表、索引、外键和 comments 完整。
- 重复执行 migration 不产生变化，checksum 变化会失败。
- 文章移动/改名保持 stable ID，重复 ID 阻止审核。
- 正文或发布元数据变化使审核变为 `changed`，查询层将对应渠道派生为 `outdated`。
- 同一任务 dedupe key 不能并发运行，失败可重试，重启可恢复。
- Publication 与 Event 原子提交；模拟提交失败时二者都不更新。
- 备份恢复后审核记录、发布历史、微信确认和 Provider 配置仍存在。
- 重新扫描可以重建文章、taxonomy 和 tag 统计，但不能伪造人工审核和发布确认。

## 12. Taxonomy 权威来源与投影

Hugo 配置中的 `taxonomies` 定义 singular/plural 映射，文章 frontmatter 提供实际使用关系，`content/<plural>/<term>/_index.md` 提供可选 term 元数据。这些 Hugo 标准资源共同构成权威来源。

`taxonomy_snapshots` 和 `taxonomy_terms` 是按 Provider instance 隔离的持久化投影。刷新成功时在单个事务中替换完整 term 集合并更新 revision；刷新失败只记录同步错误，不删除最近成功数据。`kind` 不使用封闭枚举，以保留 Hugo 自定义 taxonomy 以及未来发布平台的原生分类名称。
