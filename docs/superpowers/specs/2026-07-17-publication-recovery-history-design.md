# 发布任务恢复与统一历史设计

## 1. 目标

补齐文章发布流程的可恢复性和可追溯性：页面刷新、重新打开文章或应用重启后，用户仍能看到当前 Hugo 预览与交付进度，并能继续确认、重新生成或处理失败；文章详情同时展示 Hugo 与微信的统一发布历史。

恢复和历史都以服务端持久化状态为权威来源，不依赖浏览器保存 Job ID。页面不得展示 content hash、Job ID、数据库 ID、Vault/Hugo/staging 绝对路径或原始 `result_json`。

## 2. 范围

本阶段包含：

- 按当前工作区、文章和当前内容版本发现最新 Hugo Preview/Deliver。
- 页面刷新后恢复 Preparing、Ready、Delivering、Failed、Expired 和 Published 状态。
- 展示真实 Job 进度、阶段和安全错误说明。
- 统一展示 Hugo 与微信成功、确认和失败历史。
- 历史按时间倒序稳定分页。
- 服务重启后验证 Preview/Deliver 的安全重排和幂等恢复。
- 桌面与移动端文章页交互和布局验证。

本阶段不包含：

- 新增 Workflow 数据表或新的 Job 状态值。
- 任务取消和暂停。
- 发布历史导出、删除或筛选。
- Hugo 跨 Section 移动。
- 微信图片上传和公众号后台自动化。

## 3. 方案选择

### 3.1 服务端权威恢复与统一只读投影（采用）

后端从 `jobs`、`publications` 和 `publication_events` 构造文章级安全视图。活动流程从 Job 恢复，成功与终态失败都从 Event 展示。该方案跨刷新、浏览器和应用重启有效，不新增重复状态源。

### 3.2 浏览器保存 Job ID（不采用）

在 session storage 保存 Preview/Deliver ID，实现较少，但清理缓存、换浏览器或服务端任务被恢复后会丢失关联，不能满足本地工作台的可靠性目标。

### 3.3 新增 Workflow 表（不采用）

单独持久化流程状态可以表达更复杂的暂停和分支，但当前会重复 `jobs`、`publications` 和 `publication_events`。本阶段没有暂停、多人协作或长事务需求，不引入第五个状态源。

## 4. 状态来源与优先级

### 4.1 当前流程

当前 Hugo 流程只匹配：

- 最近使用的当前工作区。
- 请求文章 ID。
- 文章当前 `content_hash`。
- 当前工作区启用的 Hugo Provider instance。
- `hugo_preview` 或 `hugo_deliver` Job。

服务端先查当前内容版本的最新 Deliver，再查最新 Preview。显示优先级为：

```text
Deliver running/queued
  > Deliver failed
  > Deliver succeeded / Publication published
  > Preview running/queued
  > Preview failed
  > Preview ready/expired
  > 无当前流程
```

旧内容版本的任务不恢复到当前操作区，但继续保留在诊断和历史数据中。客户端不能提交 Provider ID、Workspace ID 或 Job ID 来改变文章级查询范围。

### 4.2 发布历史

统一时间线由两类事实组成：

- `publication_events`：`prepared`、`copied`、`confirmed`、`published` 等成功业务事实。
- `failed` Publication Event：发布类 Job 在最终失败且不再自动重试时追加的失败事实。

Job 不直接进入历史，避免任务状态和业务事件重复。Runner 仅在发布类任务耗尽自动重试或遇到不可重试错误后，通过受控失败回调保存 `failed` 投影和 Event；中间重试不写历史。失败 Event 的幂等身份包含 Job ID 和当前 attempt，同一次终态失败不会重复写，重排后再次终态失败可以追加新事实。Event 只保存稳定错误码和安全说明，不保存 payload、result 或底层错误链。

## 5. Application 读模型

新增只读 `PublicationWorkflowService`，依赖最小 Store 接口，不依赖 HTTP：

```go
type WorkflowStore interface {
    FindCurrentHugoJobs(ctx context.Context, workspaceID, articleID, providerID, contentHash string) (HugoJobSnapshot, error)
    ListPublicationTimeline(ctx context.Context, query TimelineQuery) (TimelinePage, error)
}
```

服务职责：

- 校验文章、当前工作区和 Provider 关系。
- 调用现有 `HugoPreviewService.Find` 构造 Preview 安全视图。
- 将 Deliver Job 映射为有限状态、进度、阶段和错误。
- 将不同渠道的 Publication Event 归一化为统一时间线。
- 生成和校验不透明分页 cursor。

该服务不创建任务、不读取文件、不调用 Deliver，也不修改 Publication。

## 6. 安全 DTO

### 6.1 当前工作流

```ts
interface PublicationWorkflowView {
  article_id: string;
  content_version: string;
  hugo?: {
    state: "preparing" | "ready" | "expired" | "failed" | "delivering" | "published";
    progress: number;
    stage: string;
    error?: string;
    preview?: HugoPreviewView;
    delivery_job?: {
      state: "queued" | "running" | "succeeded" | "failed";
      progress: number;
      stage: string;
      error?: string;
    };
  };
}
```

`preview` 继续复用现有逐字段脱敏结构。`stage` 使用稳定中文文案，不暴露内部 Kind。Job ID 只在服务端用于轮询；文章级恢复后前端不需要把它展示或持久化。

### 6.2 统一历史

```ts
interface PublicationHistoryItem {
  id: string;
  channel: "hugo" | "wechat";
  state: "prepared" | "copied" | "confirmed" | "published" | "failed";
  title: string;
  detail: string;
  occurred_at: string;
}

interface PublicationHistoryPage {
  items: PublicationHistoryItem[];
  next_cursor?: string;
}
```

`id` 是响应内稳定 key，不代表数据库主键；使用服务端摘要生成。`detail` 只包含模板、渠道动作、结果或安全失败说明，不包含 hash、路径和任务标识。

## 7. HTTP API

### 7.1 恢复当前工作流

```text
GET /api/v1/articles/{article_id}/publication-workflow
```

返回当前文章版本的安全工作流视图。没有任务时返回 `200` 和空渠道对象，不使用 `404`。文章不属于当前工作区时返回统一未找到。

### 7.2 查询发布历史

```text
GET /api/v1/articles/{article_id}/publication-history?cursor=<opaque>&limit=20
```

`limit` 默认 20，范围 1-50。cursor 绑定文章和当前工作区，使用时间与稳定 ID 组成 keyset；非法或跨文章 cursor 返回稳定的 `request.cursor_invalid`。

两个端点均为只读请求，不接受 Workspace、Provider、content hash 或路径参数。

## 8. 前端交互

### 8.1 Hugo 流程恢复

`HugoPublishFlow` 挂载后先请求文章级 Workflow：

- 无流程：加载 Section，进入新建预览状态。
- Preparing：展示真实进度并轮询 Workflow。
- Ready：直接恢复目标、文件清单和确认按钮。
- Expired：展示已过期并允许重新生成。
- Delivering：禁用生成和确认，展示交付进度并轮询。
- Failed：展示失败阶段和说明；Preview 失败允许重新生成，Deliver 失败允许重新确认同一有效 Artifact。
- Published：刷新文章详情和历史，收起操作区。

轮询间隔固定为 800ms；组件卸载或文章切换时必须取消后续请求和计时器。重复点击期间按钮禁用，服务端确定性任务继续作为最终幂等边界。

### 8.2 发布历史

文章详情右栏增加默认折叠的“发布历史”区段：

- 标题行显示已有记录数量或“暂无记录”。
- 展开后按时间倒序显示渠道、自然语言动作、结果和时间。
- 首屏只加载 20 条；有下一页时显示 `加载更多`。
- 加载失败保留已显示记录，并提供 `重新加载`。
- 发布成功后刷新第一页，不在前端伪造历史项。

移动端历史位于“发布”标签内；使用无框时间线，不嵌套卡片，不产生横向滚动。

## 9. 任务阶段映射

Job 进度映射为有限阶段：

| Kind | 进度 | 页面文案 |
| --- | ---: | --- |
| `hugo_preview` | 0-19 | 正在加载文章 |
| `hugo_preview` | 20-44 | 正在执行发布检查 |
| `hugo_preview` | 45-84 | 正在构建 Hugo 预览 |
| `hugo_preview` | 85-99 | 正在保存预览结果 |
| `hugo_deliver` | 0-44 | 正在校验发布内容 |
| `hugo_deliver` | 45-84 | 正在更新 Hugo 内容 |
| `hugo_deliver` | 85-99 | 正在记录发布结果 |

失败时保留最后阶段，并显示 Provider 或 Application 已脱敏消息。通用 Job API 仍不返回 `result_json`。

## 10. 错误与边界

- 当前文章变化：旧流程不恢复，页面进入新预览状态；旧失败可出现在历史中。
- Preview Artifact 过期：恢复为 Expired，不自动 Prepare。
- Deliver 失败且 Artifact 有效：确认操作将同一确定性 Deliver Job 从 `failed` 显式重排，复用同一 Artifact；Artifact 无效时要求重新生成。重排前已经追加的失败 Event 不删除。
- Preview 失败后重新生成：相同内容和 Section 的确定性 Preview Job 可以从 `failed` 显式重排；过期的成功 Preview 仍创建新内容版本或由清理策略移除后重建，不篡改成功历史。
- 服务重启：Runner 恢复可重试任务；页面只观察持久化状态。
- 工作区切换：旧工作区任务与历史不可见。
- 文章软删除：普通文章页不可访问；历史仍保留在数据库，供未来诊断或导出。
- 历史 payload 损坏：使用通用安全说明，不把原始 JSON 返回页面。
- 时间相同：使用稳定 ID 作为第二排序键，避免分页重复或漏项。

## 11. 测试与验收

### 11.1 后端

- 只恢复当前工作区、文章、Provider 和 content hash 的任务。
- Deliver 状态优先于 Preview，旧内容任务不会覆盖当前操作。
- Preview 安全视图不包含绝对路径或原始结果。
- 发布任务只有最终失败才追加一次 `failed` Event；自动重试过程不产生重复历史。
- 不同渠道 Event 合并后顺序稳定且不重复。
- 失败 Preview/Deliver 重排复用同一确定性 Job，重复点击只产生一次活动任务。
- cursor 不能跨文章或工作区复用。
- 通用 Job API 继续不返回 `result_json`。
- Runner 重启恢复后，Preview/Deliver 保持幂等。

### 11.2 前端

- 刷新时恢复 Preparing、Ready、Delivering、Failed、Expired 和 Published。
- Ready 恢复后可以确认，Delivering 不允许重复确认。
- 内容版本变化后不恢复旧预览。
- 组件卸载后停止轮询，不产生状态更新警告。
- 历史默认折叠、稳定分页、失败后可重试。
- Hugo 与微信历史使用一致结构和自然语言。

### 11.3 真实页面

在 1440×1000 和 390×844 验证：

- 刷新文章页后恢复 Ready Preview。
- 刷新交付中页面后继续显示真实进度。
- 发布成功后轨道和历史同步更新。
- 历史展开、加载更多和失败反馈可操作。
- 页面无 console error、文字遮挡或横向溢出。

## 12. 完成标准

- 用户刷新或重启应用后不需要重新猜测当前 Hugo 发布状态。
- 用户可以从同一时间线理解 Hugo 与微信的处理结果。
- 所有恢复和历史数据来自服务端持久化事实。
- 页面和 API 不泄露内部路径、hash、Job ID、Secret 或原始任务结果。
- 全量 Go/React 测试、类型检查、lint、构建、桌面与移动端 E2E 通过。
