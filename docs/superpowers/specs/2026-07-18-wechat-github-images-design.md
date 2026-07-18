# 微信 GitHub 图片托管设计

## 1. 目标

为微信公众号人工发布流程补齐生产可用的本地图片处理：用户确认图片清单后，InkHub 将 Vault 内的本地图片上传到公开 GitHub 仓库，生成匿名可访问的 HTTPS 地址，重写微信 HTML 中的图片引用，再由用户复制到微信公众号后台。

本功能不改变微信发布边界。InkHub 不调用微信公众号草稿或发布接口；复制格式化内容、粘贴到微信公众号后台和确认草稿仍由用户完成。

## 2. 范围

本阶段包含：

- 保留通用 `AssetUploader` 契约，新增 GitHub 图片仓库实现。
- 配置公开 GitHub 仓库、目标分支和可选路径前缀。
- Token 使用系统 Secret Store 保存，不进入 SQLite、HTTP 响应或日志。
- 解析并校验 Vault 内本地图片，远程 HTTPS 图片保持不变。
- 在准备微信内容前展示图片清单并要求用户明确确认。
- 按内容摘要幂等上传、复用已存在资源并生成公开 Raw URL。
- 任一图片失败时停止准备，不生成可复制的残缺 HTML。
- 支持任务重试、部分成功后的资源复用、诊断和安全错误反馈。

本阶段不包含：

- 私有 GitHub 仓库。
- S3、OSS、R2 或其他图床实现。
- 微信公众号自动草稿、自动发布或后台 DOM 自动化。
- 独立图片资源管理后台、删除、迁移或垃圾回收。
- 对远程 HTTP 图片做代理、下载或重新托管。

## 3. 产品流程

### 3.1 设置

微信发布渠道增加“图片仓库”设置：

- GitHub Owner。
- Repository。
- Branch，默认 `main`。
- 路径前缀，默认 `inkhub`。
- GitHub Token，只允许写入，不回显明文。

用户保存后执行诊断：

1. 校验字段格式和路径前缀。
2. 使用 Token 查询仓库和分支。
3. 确认仓库为公开仓库。
4. 确认 Token 对目标仓库具备 Contents 写权限。
5. 确认生成的 Raw URL 属于 `raw.githubusercontent.com`。

诊断只使用“正常、需要处理、未启用”三种状态。私有仓库显示“需要处理：微信无法匿名读取私有仓库图片”。Token 不存在时显示“未启用”，不影响无本地图片文章的微信预览。

### 3.2 准备微信内容

用户点击“准备微信内容”后，InkHub 先返回准备计划，不立即上传：

- 当前模板。
- 本地图片数量。
- 每张图片的文章内相对引用、媒体类型、大小。
- 计划状态：`将上传` 或 `可复用`。
- 阻断问题及中文修复建议。

页面不显示 Vault 绝对路径、摘要、GitHub Token、API 响应或内部任务 ID。没有本地图片时直接显示“无需上传图片”，仍需用户确认模板后准备内容。

用户点击“确认并准备”后创建确定性 `wechat_prepare` 任务。任务上传或复用图片、转换 Mermaid、渲染模板、内联并清理 HTML，完成后进入现有微信预览页。用户随后点击“复制格式化内容”，粘贴到微信公众号后台，最后人工确认“草稿已保存”。

## 4. 架构边界

### 4.1 通用上传契约

微信 Provider 继续依赖通用 `AssetUploader`，不感知 GitHub API、仓库、分支或 Token。上传器输入为受信本地路径、内容摘要和媒体信息，输出为公开 HTTPS URL 与复用状态。

建议将契约提升为结构化类型：

```go
type AssetUploadRequest struct {
    LocalPath string
    Digest    string
    MediaType string
    Extension string
}

type AssetUploadResult struct {
    URL    string
    Reused bool
}

type AssetUploader interface {
    Inspect(ctx context.Context, request AssetUploadRequest) (AssetUploadResult, bool, error)
    Upload(ctx context.Context, request AssetUploadRequest) (AssetUploadResult, error)
}
```

`Inspect` 只判断确定性目标是否可复用，不写入外部系统；`Upload` 执行幂等写入。Application 的准备计划使用 `Inspect`，Job 使用 `Upload`。如果实现过程中现有 Provider 边界更适合拆成独立 `AssetPlanner`，允许分离只读规划与写入，但微信 Provider 仍不得依赖 GitHub 具体类型。

### 4.2 GitHub 上传器

新增 GitHub 图片上传 Infrastructure，使用 GitHub Contents API，不调用本地 Git CLI。配置包含：

```go
type Config struct {
    Owner      string
    Repository string
    Branch     string
    Prefix     string
    Token      string
}
```

目标路径为：

```text
<prefix>/<digest[0:2]>/<digest><canonical-extension>
```

例如：

```text
inkhub/4a/4a1f...9c.png
```

相同内容和媒体类型生成相同路径，文件名和源目录变化不会重复上传。不同内容即使原文件名相同也生成不同路径。扩展名由实际文件签名对应的规范媒体类型决定，不信任用户输入扩展名。

上传算法：

1. 查询目标路径和分支。
2. 不存在时使用 Contents API 创建文件。
3. 已存在时比较 GitHub blob SHA 或下载内容摘要。
4. 内容一致则返回 `Reused=true`。
5. 路径存在但内容不一致时返回稳定冲突错误，不覆盖未知内容。
6. 创建成功后生成固定 Raw URL，并执行有限次数匿名可访问性确认。

Raw URL 只允许：

```text
https://raw.githubusercontent.com/<owner>/<repository>/<encoded-branch>/<encoded-path>
```

Owner、仓库、分支和路径必须逐段编码并校验，禁止通过 `..`、反斜杠、控制字符或 URL 注入逃逸预期地址。

### 4.3 配置与 Secret

SQLite Provider 配置只保存 `owner`、`repository`、`branch`、`prefix` 和 Secret 引用状态。GitHub Token 使用现有系统 Secret Store，以工作区和微信 Provider 实例组成稳定 key。

HTTP 写接口接受 Token，但读接口只返回 `github_token_saved: boolean`。日志只记录 Provider ID、仓库的非敏感标识、稳定错误码和耗时；不得记录 Authorization header、Token、GitHub 原始错误 body、Vault 绝对路径或图片内容。

### 4.4 图片发现与校验

Source Provider 是本地资源解析的权威来源。准备计划和执行任务只使用 `PublishInput.ResourceRefs` 中已解析的本地图片，不通过正文字符串猜测磁盘路径。

本阶段支持静态 PNG、JPEG、GIF 和 WebP。校验包括：

- 文件位于授权 Vault 根目录内。
- 文件存在且是普通文件，不跟随越界符号链接。
- 大小非零且不超过配置上限；MVP 固定上限为 10 MiB。
- 文件签名、解码结果和媒体类型一致。
- 图片宽高均大于零，像素总数不超过 40,000,000。
- SVG、BMP、TIFF、动画图片和未知格式阻断微信渠道。

远程 `https://` 图片不上传，由 Renderer 原样保留；`http://`、`file:`、`data:` 和其他 scheme 在微信预检阶段阻断。

## 5. 数据与任务

图片不纳入内容版本，也不新增图片资产业务表。确定性远端路径由内容摘要派生，GitHub 仓库是资源事实来源，任务失败后重新规划即可恢复。

准备计划使用短期、服务端可验证的 opaque `plan_token` 绑定：

- 当前工作区。
- 文章 ID。
- 微信 Provider ID。
- 当前 content hash。
- 模板 ID 与版本。
- 图片相对引用、摘要、媒体类型和大小。
- 计划过期时间。

确认接口只接收 `plan_token`，不接收客户端提交的本地路径、远端路径或任意文件清单。服务端确认时重新读取文章并校验 content hash；内容或模板变化时返回计划失效，要求重新确认。

`wechat_prepare` 保持确定性任务。重试时 GitHub 上传器先检查摘要目标，已经成功的图片直接复用，只补齐未完成图片。自动重试的中间失败不写发布历史；最终失败沿用现有 attempt 级失败事件。

## 6. HTTP 与页面模型

新增文章级接口：

```text
POST /api/v1/articles/{id}/wechat-plans
POST /api/v1/articles/{id}/wechat-plans/confirm
```

计划响应示例：

```json
{
  "plan_token": "opaque-value",
  "template": { "id": "default", "name": "InkHub Default" },
  "images": [
    {
      "reference": "images/cover.png",
      "media_type": "image/png",
      "size": 182034,
      "state": "upload"
    }
  ],
  "diagnostics": [],
  "ready": true,
  "expires_at": "2026-07-18T12:00:00Z"
}
```

确认响应只返回安全 Job 状态，不返回 Job payload、绝对路径、摘要或 GitHub API 数据。非法、过期或跨文章 token 返回统一的 `request.plan_invalid`。

微信预览页面继续使用现有复制和人工确认接口。准备成功后历史新增“微信内容已准备”；复制成功新增“微信内容已复制”；人工确认新增“已确认保存微信草稿”。

## 7. 错误与恢复

稳定错误至少包括：

- `github.config_invalid`：仓库配置格式错误。
- `github.repository_private`：仓库不是公开仓库。
- `github.permission_denied`：Token 无 Contents 写权限。
- `github.rate_limited`：GitHub 限流。
- `github.asset_conflict`：确定性路径已有不同内容。
- `github.upload_failed`：GitHub 写入失败。
- `github.public_url_unavailable`：上传后匿名 URL 不可访问。
- `wechat.image_missing`：本地图片不存在。
- `wechat.image_unauthorized`：图片越过 Vault 授权边界。
- `wechat.image_invalid`：签名、解码、尺寸或格式不合规。
- `wechat.image_upload_failed`：图片上传失败。

用户错误使用持久页面反馈，不只显示短暂 Toast。网络、限流和 GitHub 5xx 可重试；配置、权限、私有仓库、路径越界和格式错误不可自动重试。失败后不生成或保留新的可交付 HTML；已有旧 content hash 的 Artifact 不作为当前内容继续复制。

## 8. 安全边界

- 只连接 `api.github.com` 与 `raw.githubusercontent.com`，禁止配置自定义 API Host。
- HTTP Client 设置连接、响应和总超时，限制响应 body 大小，不跟随到非允许 Host。
- GitHub Token 只通过 `Authorization: Bearer` header 发送给 `api.github.com`。
- 上传内容使用 Base64，但不得写入日志或错误详情。
- 对外错误不透传 GitHub 原始 body，日志也只保留稳定错误码和 HTTP status。
- 计划 token 必须具备完整性保护并绑定当前工作区和文章，不能作为可枚举数据库 ID。
- 所有本地文件操作继续经过授权根目录和符号链接检查。

## 9. 可观测性

关键路径使用现有 zap 记录：

- 配置诊断开始、结果和耗时。
- 计划中的本地图片数量、可复用数量和阻断数量。
- 上传开始、复用、成功、失败和耗时，只记录安全的摘要前缀。
- 任务最终状态和稳定错误码。

日志级别遵循现有 `.env` 配置。成功不记录文件内容、Token、绝对路径或完整 GitHub 响应。

## 10. 测试与验收

### 10.1 自动化测试

- 图片签名、规范扩展名、大小、像素、动画、空文件和越界路径。
- GitHub 配置格式、公开/私有仓库、分支不存在和权限不足。
- 新上传、相同内容复用、同名不同内容、目标冲突和并发创建竞争。
- 限流、网络失败、GitHub 5xx、Raw URL 暂未传播和取消请求。
- Token、Authorization、绝对路径、完整摘要和原始错误 body 不进入日志或 HTTP。
- 计划 token 过期、篡改、跨文章、跨工作区、内容变化和模板变化。
- 一张图片失败时不生成 Artifact；部分成功重试只补齐剩余图片。
- 无图片、远程 HTTPS 图片和未配置图床的差异行为。
- 准备、复制、人工确认三个事件继续独立。

### 10.2 页面验收

- 桌面 1440×1000 与移动 390×844 均可查看模板和图片清单。
- 长中文路径和文件名换行，不产生横向滚动。
- 未配置、诊断失败、计划失效、上传中、部分失败和成功均有明确反馈。
- 重复点击确认只创建一个任务。
- 页面刷新可恢复准备任务，完成后进入最终微信预览。
- 控制台无 error/warn，关键按钮均有真实绑定事件。

### 10.3 真实渠道验收

使用专用公开测试仓库和最小权限 Token：

1. 上传 PNG、JPEG 和 WebP 各一张，验证 Raw URL 匿名可读。
2. 重复准备同一文章，验证不产生重复文件。
3. 将最终 HTML 复制到真实微信公众号编辑器。
4. 验证图片、标题、正文、代码块和链接可见，保存草稿后人工确认。
5. 删除或撤销测试 Token 后，验证设置诊断和失败提示准确。

真实 Token 和仓库不写入 fixture、截图、日志、Git 历史或设计文档。

## 11. Reflection 检查

每个功能点完成后检查：

- 是否仍为人工复制粘贴，没有暗示自动发布。
- 是否只有公开 GitHub 仓库可用。
- 是否保持上传抽象，不让微信 Provider 依赖 GitHub 具体实现。
- 是否在用户确认前没有外部写入。
- 是否做到摘要幂等、失败不生成残缺 Artifact。
- 是否没有泄露 Token、绝对路径和内部 ID。
- 是否覆盖旧内容版本、重复点击、任务恢复和移动端。
