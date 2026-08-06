# 小红书中文悦读模板 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将小红书发布页收敛为唯一的中文护眼长文阅读模板，并保证预览与 ZIP 导出视觉一致。

**Architecture:** 保留现有 `mobile-clean` 模板 ID 兼容历史数据，在模板注册表中删除其他入口。视觉 token 由模板定义提供给导出流程，页面 CSS 负责预览样式，二者共享同一套颜色、字体和结构规则。

**Tech Stack:** React、TypeScript、CSS、Vite、JSZip、Go 静态资源服务

## Global Constraints

- 不新增或运行测试代码，只运行构建、静态检查和页面验证。
- 不提交代码，由用户自行处理提交。
- 关键逻辑使用中文注释，公开方法保留文档注释。
- 不修改或回滚工作区中的微信相关改动。

---

### Task 1: 收敛模板注册与视觉 token

**Files:**
- Modify: `web/app/src/pages/xiaohongshu/xiaohongshuLayout.ts`
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx`

**Interfaces:**
- Consumes: `getXiaohongshuTemplate(templateID)` 和现有 `mobile-clean` 历史 ID。
- Produces: 唯一的 `XiaohongshuTemplate`，包含预览和导出使用的颜色与字体 token。

- [ ] **Step 1:** 扩展 `XiaohongshuTemplate` 的背景、文字、强调色和字体字段。
- [ ] **Step 2:** 删除 `mobile-paper` 注册项，将唯一模板标签改为“中文悦读”。
- [ ] **Step 3:** 在 `snapshotPage` 和导出样式中读取模板 token，移除模板 ID 条件分支。
- [ ] **Step 4:** 运行 `npm run build`，预期 Vite 构建退出码为 0。

### Task 2: 实现中文悦读预览样式并验证导出

**Files:**
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: `.template-mobile-clean`、`.xiaohongshu-card-content`、`.xiaohongshu-table-card` 等现有结构类。
- Produces: 中文宋体正文、墨绿标题、朱砂结构强调和护眼纸张背景。

- [ ] **Step 1:** 更新卡片背景、标题、正文、引用、图片、代码、表格卡片和页码样式。
- [ ] **Step 2:** 重启 Go 服务并打开小红书发布页，确认模板选择器只有“中文悦读”。
- [ ] **Step 3:** 检查所有卡片宽度、底部溢出、表格字段卡片和 Mermaid 渲染。
- [ ] **Step 4:** 点击“导出图片集”，确认单个 ZIP 包含全部 PNG。
- [ ] **Step 5:** 运行 `npm run build`、`go build ./...` 和 `git diff --check`，预期退出码均为 0。
