# Hugo Manual Success Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Hugo 同步失败后允许用户确认外部发布已经完成，并让当前文章版本稳定进入成功终态。

**Architecture:** 复用现有 `batchDisposition` API，将单篇文章的当前内容版本标记为 Hugo 已发表；`HugoPage` 从文章详情计算人工发表状态并传给发布流程，使它优先于旧失败 Job。确认交互拆成独立组件，Hugo 流程只负责打开弹窗、提交命令和刷新父页面。

**Tech Stack:** React 19、TypeScript 5.7、Lucide React、现有 InkHub HTTP API 与 CSS 设计变量。

## Global Constraints

- 手动标记只记录外部发布事实，不生成预览，不写入 Hugo 文件。
- 仅在当前 Hugo 流程失败时显示入口。
- 提交当前 `articleID`、`content_version`，渠道固定为 `hugo`。
- 当前版本的人工成功状态必须高于旧失败 Job；内容版本变化后自动失效。
- 不修改 Go 后端、数据库结构或现有 HTTP 契约。
- 按用户要求不新增测试代码；执行类型检查、生产构建和页面人工验证。
- 关键逻辑使用简短中文注释，公开组件保留明确文档注释。

---

### Task 1: Hugo 人工成功确认组件

**Files:**
- Create: `web/app/src/components/HugoManualSuccessDialog.tsx`

**Interfaces:**
- Consumes: `busy: boolean`、`onClose: () => void`、`onConfirm: () => void`
- Produces: `HugoManualSuccessDialog` 公开 React 组件

- [x] **Step 1: 创建专用确认对话框**

实现 `HugoManualSuccessDialog`，复用 `dialog-backdrop`、`disposition-dialog` 和 `disposition-dialog-body` 样式；标题为“手动标记 Hugo 成功”，正文为“仅记录当前版本已在外部完成发布，不会重新写入 Hugo 文件。”，操作为“取消”和“确认标记”。

```tsx
import { Check, X } from "lucide-react";
import { useEffect, useRef } from "react";

interface HugoManualSuccessDialogProps {
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}

/** HugoManualSuccessDialog 在记录外部 Hugo 发布事实前取得用户明确确认。 */
export function HugoManualSuccessDialog({ busy, onClose, onConfirm }: HugoManualSuccessDialogProps) {
  const closeButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    closeButton.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) onClose();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [busy, onClose]);

  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => {
    if (event.target === event.currentTarget && !busy) onClose();
  }}>
    <section className="disposition-dialog" role="dialog" aria-modal="true" aria-labelledby="hugo-manual-success-title">
      <header><h2 id="hugo-manual-success-title">手动标记 Hugo 成功</h2><button ref={closeButton} type="button" aria-label="关闭" disabled={busy} onClick={onClose}><X size={18} /></button></header>
      <div className="disposition-dialog-body"><p>仅记录当前版本已在外部完成发布，不会重新写入 Hugo 文件。</p></div>
      <footer><button className="secondary" type="button" disabled={busy} onClick={onClose}>取消</button><button className="primary" type="button" disabled={busy} onClick={onConfirm}><Check size={16} />{busy ? "正在处理…" : "确认标记"}</button></footer>
    </section>
  </div>;
}
```

- [x] **Step 2: 执行组件级静态验证**

Run: `cd web/app && npm run typecheck`

Expected: 命令退出码为 `0`，没有 TypeScript 错误。

---

### Task 2: 失败态提交与人工成功终态

**Files:**
- Modify: `web/app/src/components/HugoPublishFlow.tsx`
- Modify: `web/app/src/pages/hugo/HugoPage.tsx`

**Interfaces:**
- Consumes: Task 1 的 `HugoManualSuccessDialog`；现有 `batchDisposition(command)`；文章详情的 `disposition?: { kind; channels }`
- Produces: `HugoPublishFlowProps.manualPublished?: boolean`；失败态“手动标记成功”入口

- [x] **Step 1: 从文章详情传入当前版本的人工 Hugo 发表状态**

在 `HugoPage` 中计算状态并传给流程。文章详情接口已经只返回当前内容版本有效的 published disposition，因此内容变化后该值自然恢复为 `false`。

```tsx
const manualPublished = article.disposition?.kind === "published" && article.disposition.channels.includes("hugo");

<HugoPublishFlow
  articleID={article.id}
  contentHash={article.content_version}
  manualPublished={manualPublished}
  onPublished={async () => {
    setRefreshKey((value) => value + 1);
    await load();
  }}
/>
```

- [x] **Step 2: 让人工成功状态优先于旧失败工作流**

将属性定义为可选并默认 `false`，避免影响其他调用方。发布流程仍先读取 Hugo Workflow；只有 `manualPublished` 为真且 Hugo Workflow 为失败或不存在时，才清除旧 Workflow/Preview 显示状态并进入 `published` 终态。正常同步成功继续扫描真实 Hugo Bundle，不能被其他渠道的人工处置绕过。把现有 Effect 的清理逻辑提取为 Effect 内部函数，让人工终态和正常请求共用同一套取消行为。

```tsx
interface HugoPublishFlowProps {
  articleID: string;
  contentHash: string;
  manualPublished?: boolean;
  onPublished: () => void | Promise<void>;
}
```

将函数签名改为 `export function HugoPublishFlow({ articleID, contentHash, manualPublished = false, onPublished }: HugoPublishFlowProps) {`，函数体保持原结构。

在现有 `useEffect` 的 `controller.current = new AbortController();` 后加入以下代码，并将原来的 `setPublished(false)` 删除：

```tsx
const cleanup = () => {
  mounted.current = false;
  controller.current?.abort();
  if (timer.current !== null) window.clearTimeout(timer.current);
};
setPublished(false);
setPublishedTarget("");
```

在 `pollWorkflow` 读取 `value` 并确认组件仍挂载后加入：

```tsx
// 仅让人工确认覆盖旧失败任务；正常同步成功仍需校验真实 Hugo Bundle。
if (manualPublished && (!value.hugo || value.hugo.state === "failed")) {
  setWorkflow(null);
  setPreview(null);
  setFilesystemStale(false);
  setPublished(true);
  setLoading(false);
  return;
}
```

删除 Effect 尾部现有的匿名清理函数，改为 `return cleanup;`，并将依赖数组精确改为 `[articleID, contentHash, loadSections, manualPublished, showError]`。不得在组件渲染阶段调用 setter。

- [x] **Step 3: 提交单篇 Hugo published 处置**

导入 `APIError`、`batchDisposition` 和 `HugoManualSuccessDialog`，增加 `manualDialogOpen` 状态，并实现确认函数：

```tsx
const markManualSuccess = async () => {
  if (busy) return;
  setBusy(true);
  try {
    await batchDisposition({
      operation: "published",
      articles: [{ id: articleID, content_version: contentHash }],
      channels: ["hugo"],
    });
    setManualDialogOpen(false);
    setPreview(null);
    setWorkflow(null);
    setPublished(true);
    setPublishedTarget("");
    toast.show({ kind: "success", message: "已手动标记为 Hugo 同步成功" });
    await onPublished();
  } catch (reason) {
    const message = reason instanceof APIError && reason.status === 409
      ? "文章内容已更新，请刷新页面后重新确认"
      : reason instanceof Error ? reason.message : "手动标记 Hugo 成功失败";
    toast.show({ kind: "error", message });
  } finally {
    if (mounted.current) setBusy(false);
  }
};
```

- [x] **Step 4: 在 Hugo 失败卡片中接入入口和确认框**

没有可用 Artifact 时，把重试和人工操作统一放进 `hugo-failure-actions`，主操作保持“重新生成预览”，次操作为“手动标记成功”。Deliver 失败但仍有 Ready/Expired Artifact 时，保留原有重新生成和重新确认操作，并在其下方增加全宽人工入口。

```tsx
{failed && <div className="hugo-failure-actions">
  <button type="button" className="primary compact-button" disabled={busy} onClick={() => void prepare()}>
    {busy ? <LoaderCircle className="spin" size={16} /> : <CloudUpload size={16} />}重新生成预览
  </button>
  <button type="button" className="secondary compact-button" disabled={busy} onClick={() => setManualDialogOpen(true)}>
    <Check size={16} />手动标记成功
  </button>
</div>}
{manualDialogOpen && <HugoManualSuccessDialog busy={busy} onClose={() => setManualDialogOpen(false)} onConfirm={() => void markManualSuccess()} />}
```

- [x] **Step 5: 执行类型检查**

Run: `cd web/app && npm run typecheck`

Expected: 命令退出码为 `0`，所有现有调用方继续通过类型检查。

---

### Task 3: 紧凑布局、构建与页面验收

**Files:**
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: Task 2 的 `hugo-failure-actions`
- Produces: 桌面和移动端均不溢出的双按钮布局

- [x] **Step 1: 增加失败态操作布局**

在 Hugo 发布样式区增加两列布局；按钮继承现有 `primary`、`secondary` 和 `compact-button` 视觉，不新增颜色体系。

```css
.hugo-failure-actions {
  min-width: 0;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}
.hugo-failure-actions .compact-button {
  width: 100%;
  min-width: 0;
  padding-inline: 7px;
  white-space: normal;
}
.hugo-manual-success-button {
  width: 100%;
  min-width: 0;
  padding-inline: 7px;
  white-space: normal;
}
```

- [x] **Step 2: 执行生产构建**

Run: `cd web/app && npm run build`

Expected: Vite 构建成功并输出 `dist` 资源，没有 TypeScript 或打包错误。

- [x] **Step 3: 启动或复用本地服务进行页面验收**

Run: `go run ./cmd/inkhub`

Expected: 服务启动后可访问 `http://127.0.0.1:8080`；如果端口已被现有 InkHub 服务占用，则直接复用该服务。

在一篇 Hugo 状态为失败的文章上核对：

1. 页面同时显示“重新生成预览”和“手动标记成功”。
2. 点击人工入口会弹出明确的“不写入 Hugo 文件”确认说明。
3. 取消不会改变失败状态；确认期间按钮不可重复触发。
4. 确认成功后出现 Toast，操作区切换为“当前版本已同步到 Hugo”，历史新增“已标记为 Hugo 已发表”。
5. 刷新页面后仍保持成功终态，不重新显示旧失败 Job。

- [x] **Step 4: 复查改动并提交**

Run: `git diff --check && git status --short && git diff -- web/app/src/components/HugoManualSuccessDialog.tsx web/app/src/components/HugoPublishFlow.tsx web/app/src/pages/hugo/HugoPage.tsx web/app/src/styles/app.css`

Expected: 仅包含规格内文件和本实施计划，没有空白错误或意外生成物。

```bash
git add docs/superpowers/plans/2026-08-06-hugo-manual-success.md \
  web/app/src/components/HugoManualSuccessDialog.tsx \
  web/app/src/components/HugoPublishFlow.tsx \
  web/app/src/pages/hugo/HugoPage.tsx \
  web/app/src/styles/app.css
git commit -m "feat: 支持手动标记 Hugo 同步成功"
```
