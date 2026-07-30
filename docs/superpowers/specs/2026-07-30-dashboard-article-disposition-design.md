# 工作台与文章批量处置设计

## 1. 目标

修复工作台接口始终返回空数组的问题，并为已有内容迁移场景补齐文章批量处置能力。用户可以在内容库将多篇文章标记为已经在 Hugo、微信或两个渠道发表，也可以长期忽略不需要管理的文章；工作台只保留真正需要处理的内容。

本设计保持三个事实彼此独立：

- `editorial_reviews` 表示 InkHub 内的审核结果。
- `publications` 与 `publication_events` 表示各渠道的发布投影和事件。
- `article_dispositions` 表示用户对文章当前管理方式的明确决定。

处置状态只保存在本机 SQLite，不写回 Markdown frontmatter，不把“外部已发表”伪装成“InkHub 审核通过”。

## 2. 范围

本阶段包含：

- `/api/v1/dashboard` 从当前工作区真实读取数据。
- 工作台展示“处理失败、内容已更新、需要审核、最近处理”四个互斥区段。
- 内容库支持当前已加载文章的批量选择。
- 批量标记当前版本已在 Hugo、微信或两个渠道发表。
- 批量长期忽略文章，以及查看和恢复被忽略文章。
- 内容库按审核状态和处置状态筛选。
- 内容库、工作台和批量命令严格隔离当前工作区。
- 批量操作的版本冲突、幂等、事务和错误反馈。
- 桌面与移动端的批量选择、确认对话框和工作台验证。

本阶段不包含：

- 跨分页、跨搜索结果或全库一键全选。
- 定时取消忽略、忽略原因或自定义处置标签。
- 将处置状态写入 Markdown。
- 批量编辑元数据、批量审核或批量执行真实渠道发布。
- 多工作区切换界面。

## 3. 方案选择

### 3.1 独立文章处置投影（采用）

新增 `article_dispositions` 保存当前处置决定。“已发表”绑定操作时的文章内容版本，内容变化后自动失效；“忽略”是文章级长期决定，不随内容变化失效。标记已发表时，同时更新所选渠道的 Publication 投影和 Event。

该方案不改变审核状态机，也不把渠道状态塞入文章表。工作台、内容库和文章详情可以在读取时组合三个事实源，语义明确且可以独立演进。

### 3.2 扩展审核状态（不采用）

在 `editorial_reviews.state` 增加 `published` 和 `ignored` 改动较少，但会把审核、发布和管理范围混成一个状态机。多渠道文章可能同时“已审核、Hugo 已发表、微信未发表、已忽略”，单一状态无法准确表达。

### 3.3 写入 Markdown frontmatter（不采用）

处置状态随文件迁移较方便，但会修改用户源文件，并违反现有数据模型中“审核状态、渠道状态和时间戳不得写回文章”的边界。

## 4. 数据模型

新增 migration `0006_article_dispositions.sql`：

```sql
CREATE TABLE article_dispositions (
  article_id TEXT PRIMARY KEY REFERENCES articles(id),
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  kind TEXT NOT NULL CHECK (kind IN ('published','ignored')),
  content_hash TEXT NOT NULL DEFAULT '',
  cleared_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id)
);

CREATE INDEX idx_article_dispositions_workspace_kind
  ON article_dispositions(workspace_id, kind, cleared_at, updated_at);
```

字段语义：

- `article_id`：被处置文章的稳定内部标识。
- `workspace_id`：用于查询和写入时强制工作区隔离。
- `kind`：当前决定为外部已发表或忽略。
- `content_hash`：做出决定时的内容版本；`published` 必须保存，`ignored` 也保存用于审计但有效性不依赖它。
- `cleared_at`：恢复管理的时间；非空记录不参与当前处置判断。
- `created_at`：首次创建当前记录的时间。
- `updated_at`：最近处置或恢复时间，也是“最近处理”的排序来源。

迁移必须通过现有 `schema_comments` 机制为新表和每个字段提供数据库层面可见的中文 comment。

### 4.1 有效性

- 有效已发表：`kind='published' AND cleared_at IS NULL AND content_hash=articles.content_hash`。
- 过期已发表：记录仍保留，但 `content_hash<>articles.content_hash`；页面不再把当前版本显示为已发表。
- 有效忽略：`kind='ignored' AND cleared_at IS NULL`，不比较内容版本。
- 已恢复：`cleared_at IS NOT NULL`，不影响工作台和内容库当前状态。

同一文章只有一个当前投影。将已忽略文章标记为已发表，或将已发表文章改为忽略时，更新同一记录并清空 `cleared_at`。发布事件继续保留，不因处置切换或恢复而删除。

## 5. Application 边界

新增两个职责单一的 Application 模块：

### 5.1 Dashboard 查询

`dashboard.Service` 依赖只读 Store，返回安全的分组视图：

```go
type View struct {
    Failed          []ArticleSummary
    Changed         []ArticleSummary
    NeedsReview     []ArticleSummary
    RecentlyHandled []ArticleSummary
}
```

Store 只接受服务端解析出的当前工作区 ID。查询不得接收客户端提交的 Workspace ID。

### 5.2 批量处置命令

`disposition.Service` 接受：

```go
type Command struct {
    Operation string
    Articles  []ArticleVersion
    Channels  []string
}

type ArticleVersion struct {
    ID             string
    ContentVersion string
}
```

服务负责：

- 校验操作、文章数量和内容版本，并对文章 ID 与渠道做稳定去重。
- 在当前工作区解析文章及启用的 Provider。
- 验证整批文章仍是用户选择时看到的版本。
- 在单个数据库事务内更新 Disposition、Publication 和 Event。
- 对重复命令返回成功且不追加重复事件。
- 返回实际改变、保持不变和处理总数，不返回数据库路径或 hash。

HTTP Handler 只解码请求、调用一个用例并映射类型化错误，不直接拼接处置 SQL。

## 6. 工作台分组

工作台只查询最近使用的当前工作区，所有文章最多进入一个区段。分类顺序如下：

1. 有效忽略：完全排除。
2. 当前内容版本存在 `failed` Publication 或审核状态为 `blocked`：进入“处理失败”。标记某渠道已发表会覆盖该渠道的失败投影；其他未选择渠道的当前失败仍然可见。
3. 已发表处置因内容变化而过期、审核状态为 `changed`，或渠道状态相对当前内容版本为 `outdated`：进入“内容已更新”。
4. 当前版本有效已发表：进入“最近处理”，不因默认待审核状态重复出现。
5. 审核状态为 `draft`、`incomplete`、`pending_review` 或没有审核记录：进入“需要审核”。
6. 当前内容版本审核通过：进入“最近处理”。

“处理失败、内容已更新、需要审核”组内按文章修改时间倒序；“最近处理”按 Disposition 或 Review 的处理时间倒序，最多 10 篇。软删除文章不进入任何区段。

API 返回：

```text
GET /api/v1/dashboard
```

```ts
interface DashboardView {
  failed: ArticleSummary[];
  changed: ArticleSummary[];
  needs_review: ArticleSummary[];
  recently_handled: ArticleSummary[];
}
```

没有待办但存在最近处理时仍显示“最近处理”。四组均为空时显示“目前没有需要处理的文章”和 `浏览内容库`。

## 7. 内容库查询

现有文章列表查询改为结构化 `ArticleListQuery`，统一处理：

- 当前工作区。
- 标题搜索。
- 审核状态筛选。
- 处置状态筛选。
- 稳定 cursor 和分页大小。

默认查询返回未忽略文章，包括未处置和当前版本已发表文章。处置筛选支持：

- `unresolved`：没有有效已发表或忽略处置。
- `published`：当前版本有效已发表。
- `ignored`：当前有效忽略。

审核状态与处置状态同时提交时使用 AND 语义。Article Summary 增加不直接展示的 `content_version` 和可选 `disposition`；前端把版本标识原样带回批量命令，不在页面显示。

列表状态显示优先级为“已忽略、已发表、原审核状态”。Hugo 和微信列继续来自各自的 Publication 投影，不根据总处置状态伪造未选择渠道的状态。

## 8. 批量 HTTP API

```text
POST /api/v1/articles/batch-disposition
Content-Type: application/json
```

标记已发表：

```json
{
  "operation": "published",
  "articles": [
    {"id": "article_1", "content_version": "opaque-version"}
  ],
  "channels": ["hugo", "wechat"]
}
```

忽略或恢复使用同一文章结构，`channels` 省略：

```json
{
  "operation": "ignored",
  "articles": [
    {"id": "article_1", "content_version": "opaque-version"}
  ]
}
```

```json
{
  "operation": "restore",
  "articles": [
    {"id": "article_1", "content_version": "opaque-version"}
  ]
}
```

约束：

- 每批 1 至 100 篇；文章 ID 和渠道去重。
- `published` 必须选择至少一个已配置并启用的渠道。
- `ignored` 和 `restore` 不接受渠道。
- `restore` 只恢复当前有效忽略；重复恢复同一文章按幂等成功处理，不能用于撤销已发表记录。
- 所有文章必须存在、未软删除且属于当前工作区。
- 所有内容版本必须仍与列表版本一致。
- 任一校验失败则整个命令失败，不修改任何记录。
- 写请求继续执行现有同源和 JSON Content-Type 校验。

成功返回：

```json
{"processed": 12, "changed": 10, "unchanged": 2}
```

### 8.1 发布投影和事件

标记已发表时，每个所选渠道使用当前工作区启用的 Provider：

- Publication 状态更新为 `published`。
- Publication `content_hash` 更新为当前文章版本。
- 追加 `marked_published` Event，payload 只记录 `source: user` 和 `external: true`。
- 相同文章、Provider 和内容版本已经是外部标记发表时不重复写 Event。

未选择渠道保持原状。恢复“已忽略”只清除 Disposition，不回滚历史 Publication 或 Event。

## 9. 内容库交互

### 9.1 选择

- 每行增加有明确可访问名称的 checkbox。
- 表头 checkbox 选择或取消当前已经加载且符合当前搜索、筛选的文章。
- 不提供跨未加载分页的全选。
- 搜索或筛选变化后，选择集合只保留仍然可见的文章。
- 加载更多不会自动选中新加载文章，也不会清除原选择。

选择后显示稳定高度的批量操作栏，包含已选数量、`标记已发表`、`忽略` 和 `取消选择`。处置筛选为“已忽略”时，批量栏改为 `恢复管理` 和 `取消选择`。

### 9.2 标记已发表

确认对话框展示文章数量和 Hugo、微信 checkbox：

- 已启用渠道可选。
- 未配置渠道禁用，并提供进入设置的明确动作。
- 至少选择一个渠道后确认按钮才可用。
- 提交期间禁用重复点击。

### 9.3 忽略与恢复

忽略前显示文章数量并二次确认，说明未来内容变化也不会自动重新进入工作台。恢复管理也显示数量，成功后文章回到普通内容库，并按当前审核、渠道和内容版本重新参与工作台分类。

### 9.4 结果反馈

- 成功后清空选择、刷新当前列表并显示处理数量。
- 失败时保留选择和现有列表，显示服务端安全错误。
- 版本冲突提示“部分文章已更新，请刷新后重新选择”。
- 对话框关闭后焦点返回触发按钮；Escape 可取消，提交期间不可误关。

## 10. 工作台与文章详情交互

工作台使用四个不嵌套区段，空区段不渲染。每行继续展示标题、原因、修改时间、Hugo/微信自然语言状态和唯一操作。工作台不提供 checkbox 或批量处置入口。

文章详情 DTO 增加可选处置状态：

```ts
interface ArticleDispositionView {
  kind: "published" | "ignored";
  channels: Array<"hugo" | "wechat">;
}
```

`ignored` 的 `channels` 固定为空数组；`published` 的渠道从当前版本 Publication 投影派生，不返回数据库 Provider ID。

- 当前版本已发表：显示“当前版本已标记为外部发表”提示，并列出已发表渠道。
- 已忽略：显示“此文章已忽略，可在内容库恢复”提示。
- 已发表处置过期：不显示已发表提示，按内容更新后的审核和渠道状态工作。

文章详情原有审核和渠道操作保持可用，用户仍可从内容库主动打开已发表文章并处理未选择渠道。处置和恢复入口只放在内容库，避免多个页面产生不一致的批量语义。

## 11. 并发、幂等与错误

- 客户端提交的内容版本只用于乐观并发校验；服务端从数据库读取真实 hash 和 Provider，不信任客户端构造渠道关系。
- 版本冲突返回 `409 disposition.content_changed`。
- 文章不存在、软删除或跨工作区统一返回 `404 resource.not_found`，不泄露其他工作区存在性。
- 渠道未配置返回 `422 disposition.channel_unavailable`。
- 数量、操作或字段错误返回 `400 request.invalid`。
- 数据库写入失败回滚 Disposition、Publication 和 Event 的全部变化。
- 重复忽略、重复恢复或重复标记相同版本和渠道返回成功；`unchanged` 反映未产生新事实的文章数。
- 同一文章在批量请求内重复只计一次；空 ID、空版本或未知渠道拒绝整个请求。
- 内容变化不会清除旧已发表记录，只通过版本比较使其失效，保留可诊断事实。

## 12. 测试与验收

### 12.1 数据库与领域

- 空库和已有数据库均可应用 migration。
- 新表和每个字段都有 `schema_comments` 可见 comment。
- 已发表绑定当前内容版本，内容变化后失效并进入“内容已更新”。
- 忽略跨内容变化持续有效，恢复后重新参与分类。
- 同一文章只有一个当前处置投影。
- Publication、Event 和 Disposition 在同一事务提交或回滚。
- 多渠道标记分别更新正确 Provider，未选择渠道不改变。
- 重复命令不产生重复 Event。

### 12.2 Application 与 HTTP

- Dashboard 四组互斥、优先级稳定、组内排序正确。
- 最近处理最多 10 篇，忽略文章不会出现。
- 内容库和 Dashboard 只读取最近工作区。
- 搜索、审核状态、处置状态和 cursor 可以组合。
- 跨工作区、软删除、版本冲突、无渠道和超过 100 篇整批拒绝。
- 写请求继续通过本机同源保护。
- 响应不暴露绝对路径、数据库错误或内部 Provider 配置。

### 12.3 React

- 行选择和表头选择只覆盖当前已加载结果。
- 搜索、筛选、加载更多后选择集合符合可见范围规则。
- 批量工具栏在普通和已忽略筛选下提供正确操作。
- 渠道支持多选；未配置渠道禁用；未选择渠道不能提交。
- 忽略和恢复确认展示正确数量并阻止重复提交。
- 成功清空选择并刷新；失败保留选择和列表。
- 工作台正确渲染四区段、最近处理和完全空状态。
- 文章详情正确显示当前有效处置提示。

### 12.4 真实页面

在 1440×900、1024×768、390×844 和 320×568 验证：

- 内容库 checkbox、列表列、批量栏和筛选无重叠或横向溢出。
- 标记已发表、忽略和恢复对话框可完整操作，最长文案不溢出。
- 工作台四区段可以扫描和进入文章详情。
- 键盘焦点、Escape、减少动态效果和移动端底栏正常。
- 页面无 console error、严重可访问性违规或请求死循环。

## 13. 完成标准

- 内容库已有文章可以批量标记为 Hugo、微信或两个渠道已发表。
- 当前版本已发表文章不再作为普通待审核文章出现在工作台；内容变化后重新出现。
- 被忽略文章不会因重新扫描或内容变化进入工作台，并可从内容库批量恢复。
- 工作台不再使用硬编码空响应，四个区段来自当前工作区真实持久化状态。
- 批量操作具备工作区隔离、版本校验、全有或全无事务和幂等行为。
- Go/React 全量测试、race、typecheck、lint、build 和桌面/移动端浏览器验证通过。
