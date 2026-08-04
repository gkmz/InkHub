# 小红书分页卡片编辑器 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将小红书发布页改造成可左右浏览、每页独立所见即所得编辑、按模板分页并可逐页导出的卡片编辑器。

**Architecture:** 保留现有文章来源和 AI 草稿生成链路，在草稿层增加可序列化的页面块模型。前端用纯函数把旧 `body_html` 或 AI 正文解析成块，并基于模板尺寸测量分页；卡片编辑器直接修改页面块，保存时同时保留兼容的正文 HTML。代码块、图片和表格是不可拆分块，表格在测量失败时降级为结构化文本。

**Tech Stack:** Go 1.25、SQLite migrations、React 19、TypeScript、DOMParser/DOMPurify、Vitest、CSS Grid/Flexbox。

## Global Constraints

- 每段关键代码必须有明确中文注释，公开方法必须有文档注释。
- 预览和导出必须使用同一份分页结果，不能维护两套排版逻辑。
- 图片最大宽度不能超过卡片内容宽度，并且在卡片内水平居中。
- 代码块不得被截断；当前页空间不足时整体移动到下一页。
- 表格可完整容纳时保持表格，否则转换为可读普通文本。
- 旧草稿必须可读取；旧 `body_html` 在首次打开时转换为页面块，不覆盖历史版本。
- 不新增第三方编辑器依赖；编辑器使用浏览器原生 `contentEditable` 和现有安全 HTML 工具。
- 每个任务完成后运行对应测试和 `git diff --check`；全部任务完成后再做全量回归。

---

### Task 1: 页面块模型与分页纯函数

**Files:**
- Create: `web/app/src/pages/xiaohongshu/xiaohongshuLayout.ts`
- Create: `web/app/src/pages/xiaohongshu/xiaohongshuLayout.test.ts`
- Modify: `web/app/src/pages/xiaohongshu/xiaohongshuAdapter.ts`

**Interfaces:**
- Produces `XiaohongshuBlock`, `XiaohongshuPage`, `XiaohongshuTemplateMetrics`、`parseXiaohongshuBlocks(html)`、`adaptTablesForXiaohongshu(blocks, metrics)`、`paginateXiaohongshuBlocks(blocks, metrics, measure)`。
- `XiaohongshuBlock.kind` 只能是 `paragraph | heading | image | code | table | text`；`splittable` 只有普通段落和文本为 `true`。
- `paginateXiaohongshuBlocks` 返回稳定的页面和块 ID，不执行网络请求、不写 DOM 外部状态。

- [ ] **Step 1: Write failing tests for block parsing and page boundaries**

```tsx
it("把正文解析为标题、图片、代码和表格块", () => {
  const blocks = parseXiaohongshuBlocks("<h2>标题</h2><p>正文</p><img src=\"cover.png\"><pre><code>go test</code></pre><table><tr><td>A</td></tr></table>");
  expect(blocks.map((block) => block.kind)).toEqual(["heading", "paragraph", "image", "code", "table"]);
});

it("代码块放不下时整体移动到下一页", () => {
  const pages = paginateXiaohongshuBlocks([
    { id: "p1", kind: "paragraph", html: "<p>正文</p>", splittable: true },
    { id: "c1", kind: "code", html: "<pre><code>go test</code></pre>", splittable: false },
  ], { contentWidth: 320, contentHeight: 200 }, (item) => item.kind === "paragraph" ? 80 : 160);
  expect(pages.map((page) => page.blocks.map((item) => item.kind))).toEqual([["paragraph"], ["code"]]);
});

it("超宽表格转换成文本块", () => {
  const blocks = adaptTablesForXiaohongshu(parseXiaohongshuBlocks("<table><tr><td>这是一个超出卡片宽度的长字段</td><td>值</td></tr></table>"), { contentWidth: 320, contentHeight: 500 });
  expect(blocks[0].kind).toBe("text");
});
```

- [ ] **Step 2: Run the focused test and verify failure**

Run: `cd web/app && npm test -- --run src/pages/xiaohongshu/xiaohongshuLayout.test.ts`

Expected: FAIL because the layout module and its exported functions do not exist.

- [ ] **Step 3: Implement parsing and pagination**

Use `DOMParser` after `sanitizePreviewHTML`; preserve `img` attributes needed for rendering, normalize `pre > code` as one `code` block, and clone tables for width measurement. Use a `measure(block, availableWidth)` callback so tests can provide deterministic heights without a browser layout engine. When a block does not fit and is not splittable, start a new page; when a table exceeds width, replace it with a `text` block containing rows joined by ` ｜ `.

- [ ] **Step 4: Run focused tests and formatting checks**

Run: `cd web/app && npm test -- --run src/pages/xiaohongshu/xiaohongshuLayout.test.ts && npm run typecheck && npm run lint`

Expected: all layout tests pass, TypeScript has no errors, and ESLint exits 0.

### Task 2: 草稿页面模型持久化与 API 兼容

**Files:**
- Modify: `internal/domain/xiaohongshu/draft.go`
- Create: `internal/storage/sqlite/migrations/0009_xiaohongshu_pages.sql`
- Modify: `internal/storage/sqlite/repository/xiaohongshu.go`
- Modify: `internal/transport/http/xiaohongshu.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Test: `internal/storage/sqlite/repository/xiaohongshu_test.go`, `internal/transport/http/xiaohongshu_test.go`

**Interfaces:**
- Adds domain `Page`, `Block`, and `Draft.Pages []Page`.
- Adds HTTP `pages` field to `XiaohongshuDraftView`; `body_html` remains for backward compatibility.
- Adds `saveXiaohongshuDraft` payload field `pages` while accepting an omitted field from old clients.

- [ ] **Step 1: Add migration and failing repository tests**

Add nullable `pages_json TEXT NOT NULL DEFAULT '[]'` with a table/column comment matching repository migration conventions. Extend repository round-trip tests to save two pages with one code block and read them back unchanged; assert old rows with `pages_json='[]'` still return an empty page list.

- [ ] **Step 2: Run repository tests and verify the new field fails**

Run: `go test ./internal/storage/sqlite/repository ./internal/transport/http`

Expected: FAIL because the domain, migration, and scan/insert SQL do not yet include `pages_json`.

- [ ] **Step 3: Implement domain, migration, repository serialization, and API mapping**

Serialize pages with `json.Marshal`, reject malformed page data with a domain-facing error, and use `[]Page{}` for old rows. The HTTP view must return both the page model and the existing formatted topic string. Save requests with no pages keep the current pages when possible; requests with pages replace only the draft body/page content fields.

- [ ] **Step 4: Run backend tests**

Run: `gofmt -w internal/domain/xiaohongshu/draft.go internal/storage/sqlite/repository/xiaohongshu.go internal/transport/http/xiaohongshu.go && go test ./internal/storage/sqlite/repository ./internal/transport/http`

Expected: repository round-trip, migration compatibility, and API mapping tests pass.

### Task 3: 卡片编辑器与左右滑动画布

**Files:**
- Create: `web/app/src/pages/xiaohongshu/XiaohongshuCardEditor.tsx`
- Create: `web/app/src/pages/xiaohongshu/XiaohongshuCardEditor.test.tsx`
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- `XiaohongshuCardEditor` consumes `pages`, `template`, `onPagesChange`, and `onSelectionChange`.
- It produces page-level edits without exposing HTML source and emits stable page/block IDs.
- `XiaohongshuPage` owns draft loading/saving and passes only the current page model to the editor.

- [ ] **Step 1: Write failing component tests**

```tsx
it("按页码渲染卡片并支持左右滚动", () => {
  render(<XiaohongshuCardEditor pages={[page("1"), page("2")]} template="mobile-clean" onPagesChange={vi.fn()} onSelectionChange={vi.fn()} />);
  expect(screen.getByLabelText("第 1 页，共 2 页")).toBeInTheDocument();
  expect(screen.getByLabelText("第 2 页，共 2 页")).toBeInTheDocument();
});

it("编辑卡片正文时只更新当前页面", async () => {
  const onPagesChange = vi.fn();
  render(<XiaohongshuCardEditor pages={[page("1"), page("2")]} template="mobile-clean" onPagesChange={onPagesChange} onSelectionChange={vi.fn()} />);
  await userEvent.click(screen.getByLabelText("第 1 页正文"));
  await userEvent.type(screen.getByLabelText("第 1 页正文"), "新增");
  expect(onPagesChange.mock.lastCall?.[0][1].id).toBe("2");
});
```

- [ ] **Step 2: Run focused component tests and verify failure**

Run: `cd web/app && npm test -- --run src/pages/xiaohongshu/XiaohongshuCardEditor.test.tsx`

Expected: FAIL because the component and page model fixtures do not exist.

- [ ] **Step 3: Implement the editor**

Render each page with a fixed aspect ratio and centered content column. Use a horizontal scroll container with `scroll-snap-type: x mandatory`; use an uncontrolled `contentEditable` per page and synchronize only when the selected page changes, preventing cursor jumps. Render images centered with `max-width: 100%`; render code in a non-splittable block; render table blocks through the adapter output. Add keyboard-accessible page labels and page navigation buttons.

- [ ] **Step 4: Replace the split editor/preview layout**

In `XiaohongshuPage`, convert an incoming legacy `body_html` to pages once, keep pages in draft state, remove the raw HTML textarea and the separate phone preview, and pass the selected template to `XiaohongshuCardEditor`. Keep topic/source/comment controls in a compact publication-settings section. Preserve history, AI regenerate, save, and publish actions.

- [ ] **Step 5: Run component tests and visual layout checks**

Run: `cd web/app && npm test -- --run src/pages/xiaohongshu/XiaohongshuCardEditor.test.tsx && npm run typecheck && npm run lint`

Expected: component tests pass and the editor has no horizontal overflow outside its intended canvas.

### Task 4: 模板选择、分页测量和导出统一

**Files:**
- Modify: `web/app/src/pages/xiaohongshu/xiaohongshuLayout.ts`
- Modify: `web/app/src/pages/xiaohongshu/xiaohongshuAdapter.ts`
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx`
- Modify: `web/app/src/pages/xiaohongshu/xiaohongshuAdapter.test.ts`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- `XiaohongshuTemplate` contains `id`, `label`, `viewportWidth`, `pageHeight`, `padding`, and typography values.
- `renderPagesToImages(pages, template)` returns one PNG data URL per page and the exact HTML hash saved to the render API.
- The preview canvas and export function consume the same `pages` and `template` values.

- [ ] **Step 1: Add failing tests for template and export parity**

Assert that changing the template changes page metrics, a long code block moves rather than truncates, a fitting table remains a table, and an overflowing table becomes text. Add a render test that the number of generated images equals the displayed page count.

- [ ] **Step 2: Implement template metrics and DOM measurement**

Add the two existing templates (`mobile-clean`, `mobile-paper`) to a typed registry. Measure blocks in an offscreen container using the selected template CSS. Keep a fallback deterministic estimate for jsdom/tests. Re-paginate only after content or template changes and debounce measurement during typing.

- [ ] **Step 3: Reuse page output for PNG export**

Replace `snapshotPage(html, title, index, pages)` with a page-model renderer that produces exactly one SVG/PNG per `XiaohongshuPage`. Save the render hash from the concatenated sanitized page HTML and use the same page count shown in the canvas footer.

- [ ] **Step 4: Run adapter and full frontend tests**

Run: `cd web/app && npm test -- --run src/pages/xiaohongshu/xiaohongshuAdapter.test.ts src/pages/xiaohongshu/XiaohongshuCardEditor.test.tsx && npm run typecheck && npm run lint && npm run build`

Expected: all focused tests pass, the production build succeeds, and no template-specific overflow warnings are introduced.

### Task 5: 端到端回归、文档和交付检查

**Files:**
- Modify: `web/app/src/pages/xiaohongshu/xiaohongshuAdapter.test.ts` if regression coverage needs consolidation.
- Modify: `internal/transport/http/xiaohongshu_test.go` for stale/save/export API cases.
- Modify: `docs/superpowers/specs/2026-08-04-xiaohongshu-page-editor-design.md` only when implementation decisions differ from the approved design.

- [ ] **Step 1: Run the complete verification suite**

Run:

```bash
go test ./...
cd web/app
npm test -- --run
npm run typecheck
npm run lint
npm run build
```

Expected: all Go packages and 104+ frontend tests pass, TypeScript/lint pass, and Vite produces a production bundle.

- [ ] **Step 2: Check compatibility and edge paths**

Verify a legacy draft with only `body_html`, a draft with an image, a code block taller than one page, a fitting table, an overflowing table, an empty page, template switching, save/reload, stale source content, and export after an edit. Confirm API errors do not overwrite the current draft.

- [ ] **Step 3: Review the final diff and status**

Run: `git diff --check && git status --short && git diff --stat`

Confirm generated `web/dist` assets and unrelated user changes are not accidentally described as part of this feature; do not revert existing worktree changes.

- [ ] **Step 4: Commit the completed feature**

After every task check passes, stage only the files belonging to this feature and create one Conventional Commit:

```bash
git add internal/domain/xiaohongshu internal/storage/sqlite/migrations/0009_xiaohongshu_pages.sql internal/storage/sqlite/repository/xiaohongshu.go internal/transport/http/xiaohongshu.go web/app/src/pages/xiaohongshu web/app/src/api/types.ts web/app/src/api/client.ts web/app/src/styles/app.css
git commit -m "feat: add Xiaohongshu page card editor"
```
