# 小红书 AI 一键改写 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供一次点击、两阶段执行的小红书 AI 改写流程，完整覆盖原文知识点并原样保留图片、Mermaid 和表格。

**Architecture:** 前端依次调用知识提取和笔记改写接口，因此可以显示真实阶段。后端使用专用模块锁定原文素材、编排两类 AI 任务、验证知识点与素材覆盖，再复用现有草稿版本和分页流程。

**Tech Stack:** Go 1.24、`golang.org/x/net/html`、现有 OpenAI-compatible Provider、React、TypeScript、Vite。

## Global Constraints

- 常规文章目标为 6～10 张卡片，内容完整性优先于固定页数。
- 每个知识点必须被声明覆盖，图片、Mermaid 和表格必须恰好恢复一次。
- AI 失败或校验失败不得覆盖当前草稿或保存半成品。
- 不增加数据库字段、风格选择器或自动发布能力。
- 按用户要求不新增或运行自动化测试代码；每个任务使用编译、类型检查和最终真实页面操作验证。
- 关键 Go/TypeScript 逻辑使用中文注释，公开方法保留文档注释。

---

### Task 1: 增加两类小红书 AI Provider 任务

**Files:**
- Modify: `internal/provider/contracts/ai.go`
- Modify: `internal/provider/ai/openai/provider.go`
- Modify: `internal/provider/ai/openai/client.go`

**Interfaces:**
- Produces: `contracts.AITaskXiaohongshuOutline`、`contracts.AITaskXiaohongshuRewrite`。
- Produces: Provider suggestions `knowledge_points`、`title`、`body_html`、`covered_point_ids`、`topics`、`source_note`、`comment_copy`。

- [ ] **Step 1: 定义独立任务类型**

在 `internal/provider/contracts/ai.go` 的任务常量中保留旧任务兼容性，并新增：

```go
AITaskXiaohongshuOutline AITask = "xiaohongshu_outline"
AITaskXiaohongshuRewrite AITask = "xiaohongshu_rewrite"
```

- [ ] **Step 2: 按任务构建专用中文指令**

在 `buildRequest` 中把小红书指令拆为独立函数：

```go
func xiaohongshuInstruction(task contracts.AITask) string {
	switch task {
	case contracts.AITaskXiaohongshuOutline:
		return "你是技术文章知识编辑。请穷举理解和复述原文必需的核心观点、事实、步骤、注意事项、案例与结论；合并重复表达；source_evidence 必须来自原文；素材标记不是普通文字。"
	case contracts.AITaskXiaohongshuRewrite, contracts.AITaskXiaohongshu:
		return "你是小红书技术笔记编辑。必须覆盖输入知识清单中的每个 ID，忠于原文事实、术语、链接和主要顺序；只合并重复内容和缩短铺垫；使用短段落、小标题和列表；每个素材标记必须原样出现一次；禁止虚构经历、事实和夸张承诺。"
	default:
		return ""
	}
}
```

基础指令继续要求只返回合法 JSON，并把专用指令追加到请求消息。

- [ ] **Step 3: 解码两类结构化响应**

在 `decodeResponse` 中分别路由到 `decodeXiaohongshuOutlineResponse` 和 `decodeXiaohongshuRewriteResponse`。知识提取响应必须仅含 `knowledge_points`；改写响应必须包含六个必需字段，并将数组原样保存为 suggestion：

```go
[]string{
	"title", "body_html", "covered_point_ids", "topics", "source_note", "comment_copy",
}
```

两个解码器均使用 `DisallowUnknownFields`，空知识清单、空标题和空正文返回 `openai.response_invalid`。

- [ ] **Step 4: 编译 Provider 层**

Run: `go build ./internal/provider/...`

Expected: exit code `0`。

- [ ] **Step 5: 提交 Provider 任务**

```bash
git add internal/provider/contracts/ai.go internal/provider/ai/openai/provider.go internal/provider/ai/openai/client.go
git commit -m "feat: 增加小红书两阶段 AI 任务"
```

### Task 2: 实现知识清单和锁定素材模块

**Files:**
- Create: `internal/transport/http/xiaohongshu_rewrite.go`

**Interfaces:**
- Consumes: Task 1 的两类 AI 任务和 `contracts.AIProvider`。
- Produces: `XiaohongshuKnowledgePoint`、`XiaohongshuOutlineView`。
- Produces: `prepareXiaohongshuRewriteSource(string) (xiaohongshuRewriteSource, error)`。
- Produces: `restoreXiaohongshuMedia(string, []xiaohongshuLockedMedia) (string, error)`。
- Produces: `validateXiaohongshuCoverage([]XiaohongshuKnowledgePoint, []string) error`。

- [ ] **Step 1: 定义 HTTP 数据结构和输出 schema**

```go
type XiaohongshuKnowledgePoint struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Summary        string `json:"summary"`
	SourceEvidence string `json:"source_evidence"`
}

type XiaohongshuOutlineView struct {
	ContentHash    string                       `json:"content_hash"`
	KnowledgePoints []XiaohongshuKnowledgePoint `json:"knowledge_points"`
}

type xiaohongshuRewriteInput struct {
	ContentHash     string                       `json:"content_hash"`
	KnowledgePoints []XiaohongshuKnowledgePoint `json:"knowledge_points"`
}
```

outline schema 约束 `knowledge_points` 至少一项、ID 匹配 `^kp-[1-9][0-9]*$`；rewrite schema 允许 `h2`、`h3`、`p`、`strong`、`em`、`code`、`blockquote`、`ul`、`ol`、`li`、`a` 和素材标记。

- [ ] **Step 2: 使用 HTML parser 锁定顶层素材块**

使用 `html.ParseFragment` 解析正文。递归检测包含 `img`、`table` 或 `pre > code.language-mermaid/lang-mermaid` 的顶层元素，将完整顶层元素序列化保存为：

```go
type xiaohongshuLockedMedia struct {
	Token string
	HTML  string
}
```

在模型输入中用 `<p>{{INKHUB_MEDIA:media-N}}</p>` 替换。素材 token 按原文顺序生成，不能依赖 AI 返回的新编号。

- [ ] **Step 3: 校验知识点集合**

`validateXiaohongshuKnowledgePoints` 必须拒绝空字段、重复 ID、跳号 ID 和未知 kind。允许的 kind 固定为：`claim`、`fact`、`step`、`warning`、`example`、`conclusion`。

`validateXiaohongshuCoverage` 将知识点 ID 与 `covered_point_ids` 转为集合，要求数量和成员完全一致，并拒绝重复覆盖 ID。

- [ ] **Step 4: 校验并恢复素材**

用正则 `\{\{INKHUB_MEDIA:media-[1-9][0-9]*\}\}` 收集输出 token。要求每个预期 token 恰好出现一次、没有未知 token，然后使用请求内保存的原始 HTML 替换；错误信息明确指出缺失、重复或未知素材。

- [ ] **Step 5: 编译 HTTP 模块**

Run: `go build ./internal/transport/http`

Expected: exit code `0`。

- [ ] **Step 6: 提交素材与校验模块**

```bash
git add internal/transport/http/xiaohongshu_rewrite.go
git commit -m "feat: 增加小红书知识与素材校验"
```

### Task 3: 编排两阶段 HTTP 接口并保存 v3 草稿

**Files:**
- Modify: `internal/transport/http/xiaohongshu.go`
- Modify: `internal/transport/http/xiaohongshu_rewrite.go`

**Interfaces:**
- Consumes: `POST .../xiaohongshu/drafts/outline` 空 JSON 请求。
- Produces: `XiaohongshuOutlineView`。
- Consumes: `POST .../xiaohongshu/drafts/rewrite` 的 `xiaohongshuRewriteInput`。
- Produces: `XiaohongshuDraftView`，`prompt_version` 为 `xiaohongshu-v3`。

- [ ] **Step 1: 增加两个路由**

在 `xiaohongshu` switch 中增加：

```go
case request.Method == http.MethodPost && suffix == "drafts/outline":
	h.xiaohongshuOutline(response, request, articleID)
case request.Method == http.MethodPost && suffix == "drafts/rewrite":
	h.xiaohongshuRewrite(response, request, articleID)
```

保留 `drafts/generate` 作为兼容入口，由服务端顺序执行同一组内部函数。

- [ ] **Step 2: 抽取 AI Provider 构建函数**

把现有 Provider 配置读取与 `BuildAI` 逻辑移入：

```go
func (h *runtimeHandler) buildXiaohongshuAIProvider(ctx context.Context, workspaceID string) (contracts.AIProvider, error)
```

两个接口与兼容入口共用该函数，保留 `ai.not_configured`、配置损坏和 Provider 构建错误语义。

- [ ] **Step 3: 实现 outline 接口**

读取文章标题、内容哈希和渲染 HTML，锁定素材后调用 `AITaskXiaohongshuOutline`。解析并校验知识清单，返回当前 `content_hash` 和清单，不写数据库。

- [ ] **Step 4: 实现 rewrite 接口**

解析输入后重新读取当前文章，先比较 `input.ContentHash`，再校验知识清单。将清单 JSON 与带 token 的原文一起发送给 `AITaskXiaohongshuRewrite`；验证 covered IDs 和媒体 token，恢复原始素材 HTML 后再创建草稿。

草稿字段保持现有结构：

```go
PromptVersion: "xiaohongshu-v3",
State:         xiaohongshu.DraftStateDraft,
```

审计事件 payload 记录 `source=ai`、最终模型、`knowledge_point_count` 和 `media_count`。

- [ ] **Step 5: 保证失败不产生草稿**

所有校验和素材恢复必须发生在 `repo.SaveDraft` 之前。文章哈希变化返回 `content.stale`；覆盖或素材错误统一返回可读的永久 AI 响应错误。

- [ ] **Step 6: 编译后端**

Run: `go build ./...`

Expected: exit code `0`。

- [ ] **Step 7: 提交 HTTP 编排**

```bash
git add internal/transport/http/xiaohongshu.go internal/transport/http/xiaohongshu_rewrite.go
git commit -m "feat: 实现小红书两阶段改写接口"
```

### Task 4: 增加前端一键改写流程

**Files:**
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx`

**Interfaces:**
- Produces: `XiaohongshuKnowledgePoint`、`XiaohongshuRewriteOutline` TypeScript 类型。
- Produces: `outlineXiaohongshuDraft(articleID)` 和 `rewriteXiaohongshuDraft(articleID, outline)`。
- Consumes: Task 3 的两个 HTTP 接口。

- [ ] **Step 1: 增加前端类型和 API 方法**

```ts
export interface XiaohongshuKnowledgePoint {
  id: string;
  kind: "claim" | "fact" | "step" | "warning" | "example" | "conclusion";
  summary: string;
  source_evidence: string;
}

export interface XiaohongshuRewriteOutline {
  content_hash: string;
  knowledge_points: XiaohongshuKnowledgePoint[];
}
```

outline API 发送 `{}`；rewrite API 发送 `{ content_hash, knowledge_points }`，公开方法添加中文文档注释。

- [ ] **Step 2: 用真实阶段状态替换 generating 布尔值**

```ts
type RewriteStage = "idle" | "outline" | "rewrite";
const [rewriteStage, setRewriteStage] = useState<RewriteStage>("idle");
```

一次点击后的顺序必须是：设置 `outline` → 请求清单 → 设置 `rewrite` → 请求草稿 → `ensurePages` → 更新历史 → 恢复 `idle`。任一步失败只更新 message，不改变当前 draft。

- [ ] **Step 3: 更新用户可见文案**

- 顶部操作和空状态按钮：`AI 一键改写`。
- 确认提示：`AI 会先提取原文知识点，再改写为适合小红书阅读的笔记，并创建新版本。当前版本会保留在历史中。继续吗？`
- 阶段按钮：`正在提取知识点`、`正在改写笔记`。
- 成功反馈：`已生成小红书笔记，原版本仍保留在历史中`。
- 旧版提示：`这是原文分页版本，可使用“AI 一键改写”生成适合小红书传播的笔记。`

- [ ] **Step 4: 运行前端静态验证**

Run: `npm run typecheck`

Expected: exit code `0`。

Run: `npm run build`

Expected: exit code `0`，仅允许现有的大 chunk 告警。

- [ ] **Step 5: 提交前端流程**

```bash
git add web/app/src/api/types.ts web/app/src/api/client.ts web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx
git commit -m "feat: 增加小红书 AI 一键改写"
```

### Task 5: 真实页面回归与交付检查

**Files:**
- Verify only; no source changes unless检查发现问题。

**Interfaces:**
- Consumes: 完整的 outline/rewrite API 和现有卡片分页、历史、保存、导出流程。

- [ ] **Step 1: 启动最新应用**

Run: `go run ./cmd/inkhub/main.go`

Expected: `127.0.0.1:8080` 启动成功。

- [ ] **Step 2: 浏览器验证成功路径**

打开目标文章的小红书页面，点击“AI 一键改写”，确认按钮依次出现“正在提取知识点”和“正在改写笔记”；成功后标题、话题和正文卡片切换为新版本，历史中仍可选择旧版本。

- [ ] **Step 3: 检查内容与素材**

逐页确认正文结构清晰、没有 HTML/Markdown 源码、没有缩放或裁切；统计图片、Mermaid 和表格数量与原文一致；检查知识清单要求的事实、步骤、警告和结论均在正文出现。

- [ ] **Step 4: 检查失败路径**

对 Provider 未配置或可安全触发的失败状态确认错误可读、按钮恢复可点击且当前草稿不变。不得为了验证而删除用户配置或草稿。

- [ ] **Step 5: 最终静态检查**

Run: `npm run typecheck`

Run: `npm run build`

Run: `go build ./...`

Run: `git diff --check`

Expected: 所有命令 exit code `0`，工作区仅包含本功能预期文件。

- [ ] **Step 6: 聚合提交（仅在前面未按任务提交时使用）**

```bash
git add internal/provider internal/transport/http web/app/src docs/superpowers/plans/2026-08-06-xiaohongshu-ai-rewrite.md
git commit -m "feat: 增加小红书 AI 一键改写"
```
