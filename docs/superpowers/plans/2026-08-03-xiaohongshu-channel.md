# 小红书独立发布渠道实施计划

## 目标

在不影响 Hugo 和微信公众号流程的前提下，为文章增加小红书内容工作流：一次生成完整内容草稿，用户整体编辑并保存，选择手机 HTML 模板预览后导出图片集，最后手动确认已发布。所有草稿、渲染和人工确认均保留历史。

## 功能点与完成标准

### 1. 领域模型与持久化

- 新增 `xiaohongshu_drafts`、`xiaohongshu_renders`、`xiaohongshu_events` 表及字段注释。
- 新增 SQLite Repository，支持按文章查询当前/历史草稿、保存新版本、保存渲染记录和审计事件。
- 文章内容变化时通过 `source_content_hash` 派生 stale 状态，不修改原文章。
- 验证：Repository 单元测试覆盖首次保存、重复生成保留旧版本、历史分页和过期判定。

### 2. HTTP API 与独立渠道状态

- 增加 `/articles/{id}/xiaohongshu/*` 读写接口。
- 文章详情和列表增加 `xiaohongshu_state`，可与 Hugo/微信分别显示。
- 增加手动 `published` 确认接口，确认时校验当前内容版本。
- 验证：HTTP 测试覆盖参数校验、版本冲突、重复生成和旧渠道状态不受影响。

### 3. 前端内容中心

- 文章详情增加“小红书”独立入口和渠道状态。
- 新增小红书编辑页：标题、正文 HTML、话题、来源说明、评论文案整体编辑；展示历史版本并可切换只读查看。
- AI 生成按钮二次确认；每次生成创建新草稿，不覆盖旧版本；保存明确写回草稿而非文章。
- 验证：Vitest 覆盖生成确认、保存、历史切换和空状态。

### 4. 手机模板渲染与导出

- 提供固定手机视口模板（默认、简洁），将 HTML 分页显示。
- 代码使用自动换行；表格在视口中检查宽度，超出时转换为结构化文本并提示诊断。
- 使用浏览器端 Canvas 将每页导出 PNG，下载包仅包含图片文件。
- 验证：纯函数测试覆盖代码换行、宽表格转文本；Playwright 检查移动端预览非空和导出按钮状态。

### 5. 整体质量检查

- 运行 Go 全量测试、前端 Vitest/typecheck/lint/build、Playwright 回归。
- 复查 `git diff`、`git status`、中文注释、空状态和失败路径。
- 通过后创建一个 Conventional Commit 聚合提交。

## 主要接口草案

```text
GET  /api/v1/articles/{id}/xiaohongshu
POST /api/v1/articles/{id}/xiaohongshu/drafts/generate
POST /api/v1/articles/{id}/xiaohongshu/drafts
GET  /api/v1/articles/{id}/xiaohongshu/history
POST /api/v1/articles/{id}/xiaohongshu/renders
POST /api/v1/articles/{id}/xiaohongshu/published
```

所有写请求均使用同源校验；服务端只返回脱敏后的内容和稳定标识，不暴露本地文件路径或后台任务 ID。
