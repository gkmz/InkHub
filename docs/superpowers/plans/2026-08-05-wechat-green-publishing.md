# WeChat Green Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 恢复唯一的墨绿色微信模板、链接引用和可选 Mermaid 样式，并让页面预览与复制产物一致。

**Architecture:** 保留 `inkhub-default` 作为兼容标识，将其 CSS 替换为旧版墨绿色排版并移除其他模板入口。文章级 Mermaid 样式通过 HTTP 请求、加密计划、任务载荷和 `PublishInput` 传递给 Provider；链接引用由后端 DOM 转换生成后再内联 CSS 和清理。

**Tech Stack:** Go、React、TypeScript、Mermaid、`golang.org/x/net/html`

## Global Constraints

- 不新增自动化测试代码，由用户从页面验收。
- 关键逻辑和公开方法保留明确中文注释。
- Mermaid 样式仅对当前文章本次微信准备生效，默认 `handdrawn`。
- 当前产品只暴露一个墨绿色微信模板。

---

### Task 1: 唯一墨绿色模板与链接引用

**Files:**
- Modify: `internal/domain/template/builtin.go`
- Modify: `internal/provider/publish/wechat/render.go`
- Modify: `internal/app/bootstrap/provider_runtime.go`
- Modify: `internal/app/bootstrap/wechat_plan_api.go`
- Modify: `internal/transport/http/wechat_settings.go`

**Interfaces:**
- Produces: `template.BuiltinDefaultID` 对应唯一墨绿色模板。
- Produces: `appendLinkReferences(root *html.Node)`，在微信 HTML 内联与清理前生成引用节点。

- [x] **Step 1: 用旧版安全 CSS 子集替换默认模板**

将墨绿色、标题左边线、深色代码块、浅绿引用块和蓝灰表格样式写入 `BuiltinDefaultID`；删除 minimal/classic 分支。旧配置值统一经 `templateID` 和 `templateIDValue` 回退到 `BuiltinDefaultID`。

- [x] **Step 2: 在 DOM 渲染链路生成引用链接**

在 `Render` 中解析 Markdown 后调用：

```go
if err := appendLinkReferences(contextNode); err != nil {
    return "", err
}
```

该函数跳过锚点、`javascript:`、图片链接和 `pre/code` 后代，为普通链接追加带内联样式的 `sup`，并在根节点末尾追加“引用链接”章节。

- [x] **Step 3: 格式化并检查后端构建**

Run: `gofmt -w internal/domain/template/builtin.go internal/provider/publish/wechat/render.go internal/app/bootstrap/provider_runtime.go internal/app/bootstrap/wechat_plan_api.go internal/transport/http/wechat_settings.go`

Run: `go build ./...`

Expected: exit code 0。

### Task 2: Mermaid 样式贯穿准备任务

**Files:**
- Modify: `internal/provider/contracts/publish.go`
- Modify: `internal/provider/publish/wechat/provider.go`
- Modify: `internal/provider/publish/wechat/mermaid.go`
- Modify: `internal/app/publication/service.go`
- Modify: `internal/app/publication/wechat_plan.go`
- Modify: `internal/app/bootstrap/wechat_plan_api.go`
- Modify: `internal/app/bootstrap/runtime_api.go`
- Modify: `internal/app/bootstrap/publication_runner.go`
- Modify: `internal/transport/http/wechat_plan.go`

**Interfaces:**
- Produces: `PublishInput.MermaidTheme string`。
- Produces: `NormalizeMermaidTheme(value string) (string, error)`。
- Changes: `MermaidRenderer.Render(ctx, source, digest, theme string)`。
- Changes: `WeChatPlanAPI.Plan(ctx, articleID, templateID, mermaidTheme string)`。

- [x] **Step 1: 定义并校验 Mermaid 样式**

仅允许 `handdrawn` 和 `modern`，空值归一化为 `handdrawn`。`MermaidInkRenderer` 在源码没有 init 指令时注入旧版对应主题配置，并用“主题 + 源码”计算稳定摘要。

- [x] **Step 2: 放宽 Mermaid 围栏识别**

使用兼容大小写、CRLF 和无尾部换行的表达式识别代码块，并将 `input.MermaidTheme` 传给图片渲染器。

- [x] **Step 3: 把选择写入计划和任务**

在 `wechatPlanToken`、`WeChatPlanView`、`JobIntent`、`publicationPayload` 中增加 `MermaidTheme`。任务 ID 和去重键加入该字段，Runner 从载荷恢复到 `PublishInput.MermaidTheme`，不再依赖工作区默认值。

- [x] **Step 4: 扩展 HTTP 请求和响应**

`POST /wechat-plans` 接收 `mermaid_theme` 并返回归一化后的值；非法值返回参数错误。

- [x] **Step 5: 格式化并检查后端构建**

Run: `gofmt -w internal/provider/contracts/publish.go internal/provider/publish/wechat/provider.go internal/provider/publish/wechat/mermaid.go internal/app/publication/service.go internal/app/publication/wechat_plan.go internal/app/bootstrap/wechat_plan_api.go internal/app/bootstrap/runtime_api.go internal/app/bootstrap/publication_runner.go internal/transport/http/wechat_plan.go`

Run: `go build ./...`

Expected: exit code 0。

### Task 3: 微信页面内容区设置与 Mermaid 预览

**Files:**
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/components/MarkdownPreview.tsx`
- Modify: `web/app/src/pages/wechat-preview/WeChatPlan.tsx`
- Modify: `web/app/src/pages/wechat-preview/WeChatPreviewPage.tsx`
- Modify: `web/app/src/pages/setup/SetupPage.tsx`
- Modify: `web/app/src/pages/settings/SettingsPage.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Produces: `type MermaidTheme = "handdrawn" | "modern"`。
- Changes: `getWeChatPlan(articleID, templateID, mermaidTheme, signal)`。
- Changes: `MarkdownPreview({ html, mermaidTheme })`。

- [x] **Step 1: 扩展前端计划类型和请求**

请求体发送：

```ts
{ template_id: "default", mermaid_theme: mermaidTheme }
```

并读取响应中的 `mermaid_theme`。

- [x] **Step 2: 把设置移动到下方内容区**

移除 `PublicationPageFrame.toolbarContent` 的模板选择器。在准备清单上方显示固定“墨绿模板”和“手绘 / 现代”分段控件；准备或已生成内容时禁用设置，重新进入未准备状态后可修改。

- [x] **Step 3: 让前端 Mermaid 预览使用同一主题**

`MarkdownPreview` 根据主题初始化 Mermaid；手绘使用 `look: "handDrawn"` 和旧版暖色变量，现代使用蓝灰变量。渲染失败时用带 `role="alert"` 的明确错误节点替换源码代码块。

- [x] **Step 4: 清除其他模板产品入口并补充响应式样式**

设置页和初始化页固定提交 `default`，不再展示 minimal/classic；在 `app.css` 中添加内容区设置行、分段控件和移动端换行规则。

- [x] **Step 5: 检查前端构建**

Run: `npm run build --prefix web/app`

Expected: TypeScript 和 Vite 构建成功。

### Task 4: 整体页面验证

**Files:**
- Review: all modified files

- [x] **Step 1: 检查改动范围和格式**

Run: `git diff --check`

Run: `git status --short`

Expected: 无空白错误，只有本需求相关文件。

- [x] **Step 2: 启动应用**

按项目现有开发命令启动后端和前端；若 `5173` 已占用则复用现有服务或选择空闲端口。

- [x] **Step 3: 提供页面地址和人工验收项**

向用户提供微信页面 URL，并说明模板、Mermaid、引用链接和复制粘贴四个检查点。自动化验证不新增测试文件。
