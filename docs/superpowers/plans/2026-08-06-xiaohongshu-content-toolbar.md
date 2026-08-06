# 小红书内容工具栏 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将小红书 AI 改写和版本历史操作从顶部导航移入内容区域工具栏。

**Architecture:** 保持共享 `PublicationPageFrame` 不变，仅停止小红书页面向其注入工具按钮。在小红书已审核内容区域内增加独立工具栏，并把历史面板嵌入该工具栏作为定位上下文，现有状态和事件处理函数全部复用。

**Tech Stack:** React、TypeScript、CSS、Lucide React。

## Global Constraints

- 顶部区域只显示返回、文章路径、发布进度和渠道导航。
- 内容工具栏只承载“历史版本”和“AI 一键改写”。
- 保存、导出和标记发布按钮保持原位。
- 不修改 API、草稿结构、生成流程或其他发布渠道。
- 按用户要求不新增或运行自动化测试代码，只运行类型检查和生产构建。

---

### Task 1: 移动小红书版本操作并调整定位

**Files:**
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx:96`
- Modify: `web/app/src/styles/app.css:212`

**Interfaces:**
- Consumes: 现有 `showHistory`、`setShowHistory`、`generate`、`rewriteLabel` 和 `view.history`。
- Produces: `.xiaohongshu-content-toolbar`、`.xiaohongshu-content-summary` 和锚定于工具栏的 `.xiaohongshu-history`。

- [ ] **Step 1: 从顶部框架移除操作注入**

将 `PublicationPageFrame` 调用改为不传 `toolbarContent`：

```tsx
<PublicationPageFrame article={article} active="xiaohongshu" onNavigate={onNavigate}>
```

- [ ] **Step 2: 在已审核内容区域增加工具栏**

在 `.xiaohongshu-settings` 之前增加工具栏，并将现有历史面板移入工具栏：

```tsx
<section className="xiaohongshu-content-toolbar" aria-label="小红书内容工具">
  <div className="xiaohongshu-content-summary">
    <strong>内容版本</strong>
    <span>{draft ? `${draft.state}${draft.stale ? " · 原文已更新" : ""}` : "尚未生成草稿"}</span>
  </div>
  <div className="xiaohongshu-actions">
    <button className="secondary" type="button" aria-expanded={showHistory} onClick={() => setShowHistory((value) => !value)}>
      <History size={15} />历史版本
    </button>
    <button className="secondary" type="button" onClick={() => void generate()} disabled={rewriting}>
      <RefreshCw size={15} />{rewriteLabel}
    </button>
  </div>
  {showHistory && <aside className="xiaohongshu-history">
    <div className="tool-heading">
      <h2>版本历史</h2>
      <button className="back" type="button" onClick={() => setShowHistory(false)}>关闭</button>
    </div>
    {view.history.map((item) => <button key={item.id} type="button" className={`xiaohongshu-history-item${draft?.id === item.id ? " active" : ""}`} onClick={() => {
      setDraft(ensurePages(item, article.preview_html, template));
      setShowHistory(false);
    }}>
      <strong>{item.title || "未命名草稿"}</strong>
      <span>{item.state}{item.stale ? " · 内容已更新" : ""}</span>
      <time>{new Intl.DateTimeFormat("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(item.created_at))}</time>
    </button>)}
  </aside>}
</section>
```

历史条目继续复用现有版本选择逻辑，选择后关闭历史面板。

- [ ] **Step 3: 增加桌面端内容工具栏样式**

在小红书页面样式区增加：

```css
.xiaohongshu-content-toolbar { position: relative; min-height: 56px; display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 12px; padding: 10px 18px; border-block: 1px solid var(--line); background: var(--surface); }
.xiaohongshu-content-summary { min-width: 0; display: grid; gap: 3px; }
.xiaohongshu-content-summary strong { font-size: 12px; }
.xiaohongshu-content-summary span { color: var(--muted); font-size: 10px; }
.xiaohongshu-content-toolbar .xiaohongshu-history { top: calc(100% + 8px); right: 18px; }
```

保留现有按钮尺寸，将 `.xiaohongshu-history` 的全局定位规则改为只定义尺寸和视觉样式，由内容工具栏提供 `top`、`right` 和定位上下文。

- [ ] **Step 4: 增加移动端布局规则**

在现有移动端媒体查询中增加：

```css
.xiaohongshu-content-toolbar { align-items: flex-start; flex-wrap: wrap; padding: 10px 12px; }
.xiaohongshu-content-toolbar .xiaohongshu-actions { width: 100%; justify-content: flex-end; }
.xiaohongshu-content-toolbar .xiaohongshu-history { left: 12px; right: 12px; width: auto; }
```

- [ ] **Step 5: 运行前端静态验证**

Run: `npm run typecheck`

Expected: exit code `0`。

Run: `npm run build`

Expected: exit code `0`，仅允许现有的大 chunk 告警。

- [ ] **Step 6: 检查改动范围**

Run: `git diff --check`

Expected: exit code `0`，源码改动只包含小红书页面和公共样式文件。
