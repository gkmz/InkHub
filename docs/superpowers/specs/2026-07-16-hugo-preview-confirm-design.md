# Hugo 发布预览与确认设计

## 1. 目标

将文章页当前“点击后直接创建 Hugo 发布任务”的行为改为可信的两阶段流程：InkHub 先在 staging 中准备并构建真实 Artifact，用户审核目标 Section、文件清单和诊断后，再确认将同一个 Artifact 原子交付到 Hugo `content/`。

预览和交付必须绑定同一文章内容版本。用户没有确认前，Hugo 正式目录不得发生变化。

## 2. 范围

本阶段包含：

- 扫描 Hugo `content/` 下可用的一级 Section。
- 新文章首次发布时选择一个已有 Section。
- 已发布文章按 `source_id` 自动定位原 bundle 和 Section。
- 后台执行 Preflight、转换、资源复制和 Hugo staging build。
- 持久化完整 Artifact，向页面返回脱敏摘要。
- 用户确认后交付同一个 Artifact。
- 页面刷新后的预览与交付状态恢复。
- Artifact 过期、文章变化、重复确认和失败恢复。
- 为后续微信流程保留通用的 Application Artifact 模型。

本阶段不包含：

- 新增、重命名或删除 Hugo Section。
- 将已有文章跨 Section 移动。
- Git commit、push 或部署。
- Hugo 全站预览服务器管理。
- 微信页面和微信 Artifact 流程改造。
- 通用 SEO 与内容质量检查。

## 3. 方案选择

### 3.1 两阶段 Artifact 流程（采用）

首次操作创建准备任务，依次执行 Preflight、转换、资源复制和 Hugo build，得到真实 `PreparedArtifact`。页面展示该 Artifact 的安全摘要；用户确认后，服务端读取同一个 Artifact 并执行 Deliver。

该方案保证预览内容就是最终交付内容。确认后的操作只负责版本校验和原子替换，不重新生成文件。

### 3.2 轻量预检后重新构建（不采用）

首次只估算路径和文件，确认后再运行完整 Prepare。实现较少，但估算结果可能与真实构建不同，用户确认后仍可能发现转换或 Hugo build 错误。

### 3.3 发布任务中途暂停（不采用）

现有任务在 Prepare 后进入“等待确认”，确认后恢复。该方案需要扩展 Job 状态机、恢复语义和超时处理，改动更大，也会把人工等待混入工作队列。

## 4. Section 发现

### 4.1 扫描规则

Hugo Provider 扫描当前实例根目录的 `content/`，只返回符合以下约束的一级子目录：

- 是真实目录，不是普通文件。
- 不是符号链接。
- 名称不以 `.` 开头。
- 名称通过现有安全路径段校验。
- 解析后的路径仍位于 Hugo `content/` 内。

每个 Section 返回名称和目录内 Markdown 内容数量。计数仅用于帮助用户识别目录，不影响发布规则。

```ts
interface HugoSection {
  name: string;
  article_count: number;
}
```

InkHub 不提供创建、重命名或删除 Section 的入口。没有可用 Section 时，页面提示用户先在 Hugo `content/` 下创建目录。

### 4.2 新文章

首次发布的新文章必须从扫描结果中选择 Section。只有一个候选时页面自动选中，但仍显示最终目标。浏览器不得提交扫描结果之外的 Section；服务端在 Prepare 前重新验证目录。

目标 bundle 为：

```text
content/<section>/<slug>/index.md
```

slug 为空时使用文章稳定 ID。图片和附件与 `index.md` 放在同一 page bundle。

### 4.3 已有文章

Provider 在整个 Hugo `content/` 范围内查找 `index.md` frontmatter 中与文章稳定 ID 相同的 `source_id`：

- 找到一个：自动使用该 bundle 的一级 Section，页面只读显示“继续更新 <section>”。
- 没有找到：按新文章处理。
- 找到多个：阻止预览，要求用户先在 Hugo 中清理重复 `source_id`。
- bundle 不位于合法一级 Section 下：阻止预览，不猜测目标。

本阶段禁止将已有 bundle 移动到其他 Section，避免 URL 变化、旧目录残留和重复内容。

## 5. Artifact 边界

### 5.1 准备

准备任务生成确定性 OperationID，并执行：

```text
加载当前文章与资源
  → 重新验证 Section
  → Preflight
  → 复制 Hugo 站点到 staging
  → 转换正文和 frontmatter
  → 复制并重写资源
  → Hugo build
  → 保存 PreparedArtifact
```

完整 Artifact 只保存在服务端 Job 结果和 Provider staging manifest 中，包含交付所需的绝对内部位置。浏览器只得到安全摘要。

通用 `GET /api/v1/jobs/{id}` 只返回任务状态、进度和安全阶段文案，禁止序列化原始 `result_json`。只有 Hugo preview 专用 API 可以读取完整 Artifact，并在逐字段转换后返回下述安全摘要。

### 5.2 安全摘要

```ts
interface HugoPreview {
  id: string;
  article_id: string;
  content_hash: string;
  section: string;
  target_path: string;
  change: "added" | "updated";
  files: Array<{
    relative_path: string;
    media_type: string;
    size: number;
  }>;
  diagnostics: Array<{
    code: string;
    level: "blocking" | "recommended" | "optional" | "passed";
    message: string;
  }>;
  preview_url?: string;
  expires_at: string;
  state: "preparing" | "ready" | "failed" | "expired" | "stale" | "delivering" | "published";
  job_id?: string;
}
```

`target_path` 和文件路径全部相对 Hugo 根目录，例如 `content/posts/my-slug/index.md`。响应不得包含 Hugo 根目录、Vault 路径、staging 路径、Artifact Location、Secret 或内部 manifest 内容。

### 5.3 文件清单

Provider 在 Prepare 完成后枚举 staged bundle，记录相对 bundle 路径、媒体类型、大小和 SHA-256。客户端不提交文件内容或清单。服务端确认时使用持久化 Artifact，不根据客户端摘要重建。

## 6. API

### 6.1 Section 查询

```text
GET /api/v1/articles/{article_id}/hugo-sections
```

返回：

- 可选 Sections。
- 文章已有 bundle 所属 Section（如存在）。
- `selection_locked`，表示已有文章不能更换 Section。

查询按最近工作区、文章和 Hugo Provider instance 隔离。

### 6.2 创建预览

```text
POST /api/v1/articles/{article_id}/hugo-previews
```

请求：

```json
{
  "section": "posts",
  "content_hash": "current-content-hash"
}
```

返回准备任务 ID 和预览 ID。相同文章、Provider、content hash 与 Section 使用确定性 ID；重复请求返回同一个任务或已有结果。

### 6.3 查询预览

```text
GET /api/v1/hugo-previews/{preview_id}
```

服务端从 Job 和 Artifact manifest 生成安全视图。页面刷新后通过文章 ID 与 content hash 查询最近预览，不依赖浏览器内存保存完整 Artifact。

### 6.4 确认交付

```text
POST /api/v1/hugo-previews/{preview_id}/confirm
```

确认前必须重新校验：

- Preview 属于当前工作区、文章和 Hugo Provider。
- 当前文章 content hash 与预览一致。
- Artifact 未过期且 manifest、staged bundle 仍存在。
- Section 与目标仍安全。
- Artifact 尚未被其他内容版本替代。

确认成功后创建交付任务。相同 preview ID 重复确认返回同一个交付任务，不重复 Deliver。

现有 `/api/v1/publications` 继续服务微信和兼容已排队任务；文章页不得再通过该接口直接启动新的 Hugo 发布。

## 7. Application 与 Job 设计

新增两种任务职责：

- `hugo_preview`：只执行 Preflight 和 Prepare，不修改 Hugo 正式目录。
- `hugo_deliver`：只加载并校验已准备 Artifact，然后执行 Deliver 和保存 Publication/Event。

任务 payload 只保存文章 ID、Provider ID、content hash、Section 和 preview ID。完整 Artifact 作为服务端 result 持久化，不接受客户端构造。

预览和交付的 dedupe key 分别包含：

```text
hugo_preview + article_id + provider_id + content_hash + section
hugo_deliver + preview_id
```

确认创建交付任务后，预览状态为 `delivering`。交付成功保存 `published` Publication；失败保存 Job 错误，但不把 Publication 标为成功。

## 8. Provider 调整

Hugo Provider 增加以下内部能力，但保持 `PublishProvider` 通用接口不感知 UI：

- 枚举合法 Sections。
- 在整个 `content/` 范围查找唯一 `source_id` bundle。
- 根据新文章选择或已有 bundle 推导最终 Section。
- PreparedArtifact 记录目标、文件 manifest、预览 URL和过期时间。
- Deliver 继续使用 staging + 备份 + 原子替换 + 最终 build 确认。

Provider 不访问 SQLite，不判断当前工作区，不生成页面文案。Application 负责持久化、幂等和安全视图转换。

## 9. 页面交互

文章审核页使用内联区域：

1. 用户点击 `同步到 Hugo`。
2. 页面展开 Section 选择器和说明。
3. 用户点击 `生成发布预览`。
4. 页面显示准备任务阶段。
5. 成功后显示目标路径、增加/更新状态、文件清单、诊断和过期时间。
6. 用户点击 `开始同步`。
7. 页面显示交付任务状态。
8. 成功后更新发布轨道，并提供博客 URL或 `打开预览`。

不使用全屏向导或嵌套卡片。移动端在“发布”标签内展示相同流程。

## 10. 状态与错误

- 文章变化：预览标记 `stale`，禁用确认，提供重新生成。
- Artifact 过期：标记 `expired`，提供重新生成。
- Section 被删除：准备失败并重新加载 Sections。
- Preflight 阻断：展示诊断，不调用 Prepare。
- Hugo staging build 失败：正式目录不变，展示失败阶段和摘要。
- Deliver 失败：Provider 恢复旧 bundle，Job 失败，Publication 不标记成功。
- 重复点击：由确定性 ID 和 dedupe key 返回同一任务。
- 页面刷新：恢复最近匹配当前 content hash 的预览或交付任务。
- 用户离开页面：任务继续运行，返回后从 Job 状态恢复。

## 11. 微信关系

微信当前流程保持不变：Prepare HTML、复制到剪贴板、用户粘贴后人工确认。后续微信可以复用本阶段的 Artifact 持久化、安全视图、版本校验和任务恢复模型，但交付语义不同：

- Hugo 确认后原子写入博客目录。
- 微信确认后复制 HTML，并由用户在公众号后台完成草稿保存。

本阶段不修改微信 API、页面或 Provider。

## 12. 测试与验收

### 12.1 Provider

- Section 扫描排除隐藏目录、符号链接、文件和非法名称。
- 新文章按选定 Section 生成 bundle。
- 已有 `source_id` 自动使用原 Section。
- 重复 `source_id` 阻止预览。
- 文件 manifest 不遗漏 `index.md` 和资源。
- Deliver 成功、失败回滚和重复调用幂等。

### 12.2 Application 与 HTTP

- 预览与确认均限制当前工作区和 Provider。
- 客户端伪造 Section、路径、Artifact 或 content hash 被拒绝。
- 相同输入重复预览和确认返回相同任务。
- 文章变化、Artifact 过期和文件缺失拒绝确认。
- API 不泄露绝对路径、Secret 或 manifest 内部字段。
- 页面刷新能够恢复预览和交付状态。

### 12.3 前端

- Section 单选、唯一候选自动选择和已有 Section 锁定。
- 准备进度、文件清单、诊断、过期和 stale 状态。
- 未确认前不调用交付 API。
- 重复点击保护和失败反馈。
- 成功后发布轨道更新。
- 桌面和移动端无溢出、遮挡或控制台错误。

### 12.4 回归

- 微信现有准备、复制和人工确认行为不变。
- Category、Series、Tags 和 AI 建议不回归。
- 运行前端全量测试、类型检查、lint、生产构建。
- 运行 `go test ./...` 与 `go vet ./...`。
