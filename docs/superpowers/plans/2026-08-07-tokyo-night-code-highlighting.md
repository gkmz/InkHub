# Tokyo Night Code Highlighting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在审核、微信和小红书三条展示链路中恢复 old 分支的 Tokyo Night 深色语法高亮，并让微信与小红书的最终颜色由各自模板控制。

**Architecture:** 统一 Markdown 渲染器改为输出 Chroma 语义 class，不再写入固定主题的行内颜色。审核页用固定 CSS 消费这些 class；微信模板在发布准备时把模板 token 规则内联到节点；小红书模板对象携带代码主题，由卡片预览和 PNG 快照共同生成带作用域的 CSS。

**Tech Stack:** Go 1.25、Goldmark、goldmark-highlighting、Chroma v2、React 19、TypeScript 5.7、Vite 5。

## Global Constraints

- 只对围栏代码块应用 Tokyo Night；行内代码继续使用现有浅色强调样式。
- 主题色值逐项复用 `old/markdown/chroma_style.go`，背景固定为 `#1a1b26`，正文固定为 `#c0caf5`。
- 不增加主题选择器，不启用语言猜测，不执行数据库迁移。
- Mermaid 继续渲染为图表或图片，不能被 Tokyo Night 规则显示为源码。
- 微信复制结果必须使用行内 style 保留 token 颜色；模板安全边界仍限定在 `.inkhub-root` 内。
- 小红书卡片预览、分页测量和 PNG 导出保持相同的代码块尺寸、字体、换行策略和颜色。
- 按用户要求不新增测试代码；只执行编译检查、前端类型检查、生产构建和页面人工验证。
- 关键逻辑使用简短中文注释，公开方法保留明确文档注释。
- 提交时只暂存本计划列出的文件，不包含工作区已有的 Hugo、设置或 editorial 改动。

---

### Task 1: Markdown 输出 Chroma 语义 class

**Files:**
- Modify: `internal/content/markdown/render.go`

**Interfaces:**
- Consumes: `github.com/alecthomas/chroma/v2/formatters/html.WithClasses`
- Produces: `NewRenderer() goldmark.Markdown`，围栏代码 HTML 包含 `chroma`、`line`、`k`、`s` 等语义 class，不包含主题 token 的 `style`

- [ ] **Step 1: 切换代码高亮输出格式**

增加 Chroma HTML formatter 导入，将固定的 `highlighting.WithStyle("github")` 替换为 class 输出；不设置 `WithGuessLanguage`。

```go
import (
	"bytes"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
)

// NewRenderer 创建 InkHub 统一 Markdown 渲染器。
//
// GFM 覆盖表格、任务列表和删除线等 Obsidian 常用语法；Chroma 只输出
// 语义 class，最终 token 配色由审核页或渠道模板决定。
func NewRenderer() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
	)
}
```

- [ ] **Step 2: 执行 Go 编译检查**

Run: `go test -run '^$' ./internal/content/markdown ./internal/provider/publish/wechat`

Expected: 两个包完成编译；不运行现有行为测试，也不新增或修改测试代码。

- [ ] **Step 3: 提交语义输出改动**

```bash
git add internal/content/markdown/render.go
git commit -m "refactor(markdown): 输出语义化代码高亮标记"
```

---

### Task 2: 微信模板内联 Tokyo Night token

**Files:**
- Create: `internal/domain/template/tokyo_night.go`
- Modify: `internal/domain/template/builtin.go`
- Modify: `internal/provider/publish/wechat/render.go`

**Interfaces:**
- Consumes: Task 1 生成的 Chroma class；`Validated.CSS`
- Produces: `tokyoNightCodeCSS string`；`matchesSimpleSelector` 支持安全的简单 class；微信输出中的 Chroma span 带 Tokyo Night 行内 style

- [ ] **Step 1: 把完整主题规则放入微信模板模块**

创建 `tokyo_night.go`。规则必须位于 `.inkhub-root` 下，使用 Chroma v2 标准 class 分组，并逐项对应 old 主题：

```go
package template

// tokyoNightCodeCSS 是微信模板使用的旧版 Tokyo Night Night token 配色。
const tokyoNightCodeCSS = `
.inkhub-root .chroma { background-color: #1a1b26; color: #c0caf5; }
.inkhub-root .w { color: #3b4261; }
.inkhub-root .err { color: #f7768e; }
.inkhub-root .c, .inkhub-root .ch, .inkhub-root .cm, .inkhub-root .c1, .inkhub-root .cs { color: #565f89; font-style: italic; }
.inkhub-root .cp, .inkhub-root .cpf { color: #7aa2f7; }
.inkhub-root .k, .inkhub-root .kp, .inkhub-root .kr { color: #bb9af7; }
.inkhub-root .kc { color: #ff9e64; }
.inkhub-root .kd { color: #7aa2f7; font-weight: bold; }
.inkhub-root .kn { color: #7aa2f7; }
.inkhub-root .kt { color: #2ac3de; }
.inkhub-root .o { color: #89ddff; }
.inkhub-root .ow { color: #89ddff; font-weight: bold; }
.inkhub-root .p, .inkhub-root .n, .inkhub-root .nx { color: #c0caf5; }
.inkhub-root .na, .inkhub-root .ni, .inkhub-root .py { color: #73daca; }
.inkhub-root .nb, .inkhub-root .bp, .inkhub-root .nc, .inkhub-root .nd, .inkhub-root .nf, .inkhub-root .fm, .inkhub-root .nt { color: #7aa2f7; }
.inkhub-root .no, .inkhub-root .ss { color: #ff9e64; }
.inkhub-root .ne, .inkhub-root .nv, .inkhub-root .vc, .inkhub-root .vg, .inkhub-root .vi, .inkhub-root .vm { color: #f7768e; }
.inkhub-root .nl { color: #7dcfff; }
.inkhub-root .nn { color: #7aa2f7; }
.inkhub-root .l, .inkhub-root .s, .inkhub-root .sa, .inkhub-root .sb, .inkhub-root .sc, .inkhub-root .dl, .inkhub-root .s2, .inkhub-root .sh, .inkhub-root .sx, .inkhub-root .s1 { color: #9ece6a; }
.inkhub-root .ld, .inkhub-root .si { color: #e0af68; }
.inkhub-root .sd { color: #565f89; font-style: italic; }
.inkhub-root .se { color: #bb9af7; }
.inkhub-root .sr { color: #b4f9f8; }
.inkhub-root .m, .inkhub-root .mb, .inkhub-root .mf, .inkhub-root .mh, .inkhub-root .mi, .inkhub-root .il, .inkhub-root .mo { color: #ff9e64; }
.inkhub-root .gd, .inkhub-root .gr, .inkhub-root .gt { color: #f7768e; }
.inkhub-root .ge { font-style: italic; }
.inkhub-root .gh, .inkhub-root .gu { color: #7aa2f7; font-weight: bold; }
.inkhub-root .gi { color: #9ece6a; }
.inkhub-root .go { color: #565f89; }
.inkhub-root .gp { color: #2ac3de; font-weight: bold; }
.inkhub-root .gs { font-weight: bold; }
`
```

- [ ] **Step 2: 将主题并入墨绿模板**

在 `BuiltinDefaultID` 分支的基础 CSS 字符串赋值结束后追加主题常量。保留现有 `pre`、`pre code` 的尺寸和换行规则，确保行内 code 规则不受影响。

```go
css += tokyoNightCodeCSS
```

- [ ] **Step 3: 扩展微信模板的简单选择器匹配**

`matchesSelector` 的末端也调用 `matchesSimpleSelector`，不能再假定末端一定是标签。`matchesSimpleSelector` 只接受标签或单个 `.class`；复杂选择器仍由模板校验拒绝。

```go
func matchesSelector(node *html.Node, selector string) bool {
	parts := strings.Fields(selector)
	if len(parts) == 0 || parts[0] != ".inkhub-root" {
		return false
	}
	if len(parts) == 1 {
		return hasClass(node, "inkhub-root")
	}
	if !matchesSimpleSelector(node, parts[len(parts)-1]) {
		return false
	}
	parent := node.Parent
	for index := len(parts) - 2; index >= 0; index-- {
		for parent != nil && !matchesSimpleSelector(parent, parts[index]) {
			parent = parent.Parent
		}
		if parent == nil {
			return false
		}
		parent = parent.Parent
	}
	return true
}

func matchesSimpleSelector(node *html.Node, selector string) bool {
	if strings.HasPrefix(selector, ".") {
		class := strings.TrimPrefix(selector, ".")
		return class != "" && !strings.Contains(class, ".") && hasClass(node, class)
	}
	return node.Type == html.ElementNode && node.Data == selector
}
```

- [ ] **Step 4: 验证模板编译和 CSS 安全校验**

Run: `gofmt -w internal/domain/template/tokyo_night.go internal/domain/template/builtin.go internal/provider/publish/wechat/render.go`

Run: `go test -run '^$' ./internal/domain/template ./internal/provider/publish/wechat`

Expected: 编译成功；内置模板初始化不会返回 selector 安全错误。

- [ ] **Step 5: 提交微信模板改动**

```bash
git add internal/domain/template/tokyo_night.go internal/domain/template/builtin.go internal/provider/publish/wechat/render.go
git commit -m "feat(wechat): 恢复 Tokyo Night 代码模板"
```

---

### Task 3: 审核页固定 Tokyo Night

**Files:**
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: Task 1 的 Chroma class
- Produces: `.prose pre.chroma` 和 `.prose .chroma .<token>` 固定样式

- [ ] **Step 1: 替换审核页浅色代码块并加入 token 规则**

保留 `.prose pre` 的现有尺寸，改为深色容器；在其后增加与 Task 2 相同分组的 scoped token 规则。所有选择器以 `.prose .chroma` 或 `.prose pre.chroma` 开头，不编写裸 `.k` 等全局规则。

```css
.prose pre {
  overflow: auto;
  padding: 15px;
  border: 1px solid #2a2d3e;
  border-radius: 4px;
  background: #1a1b26;
  color: #c0caf5;
}
.prose pre.chroma code { color: inherit; font-size: 13px; line-height: 1.72; }
.prose .chroma .w { color: #3b4261; }
.prose .chroma .err, .prose .chroma .ne, .prose .chroma .nv, .prose .chroma .vc, .prose .chroma .vg, .prose .chroma .vi, .prose .chroma .vm, .prose .chroma .gd, .prose .chroma .gr, .prose .chroma .gt { color: #f7768e; }
.prose .chroma .c, .prose .chroma .ch, .prose .chroma .cm, .prose .chroma .c1, .prose .chroma .cs, .prose .chroma .sd { color: #565f89; font-style: italic; }
.prose .chroma .cp, .prose .chroma .cpf, .prose .chroma .kd, .prose .chroma .kn, .prose .chroma .nb, .prose .chroma .bp, .prose .chroma .nc, .prose .chroma .nd, .prose .chroma .nf, .prose .chroma .fm, .prose .chroma .nt, .prose .chroma .nn, .prose .chroma .gh, .prose .chroma .gu { color: #7aa2f7; }
.prose .chroma .k, .prose .chroma .kp, .prose .chroma .kr, .prose .chroma .se { color: #bb9af7; }
.prose .chroma .kc, .prose .chroma .no, .prose .chroma .ss, .prose .chroma .m, .prose .chroma .mb, .prose .chroma .mf, .prose .chroma .mh, .prose .chroma .mi, .prose .chroma .il, .prose .chroma .mo { color: #ff9e64; }
.prose .chroma .kt, .prose .chroma .gp { color: #2ac3de; }
.prose .chroma .o, .prose .chroma .ow { color: #89ddff; }
.prose .chroma .na, .prose .chroma .ni, .prose .chroma .py { color: #73daca; }
.prose .chroma .nl { color: #7dcfff; }
.prose .chroma .l, .prose .chroma .s, .prose .chroma .sa, .prose .chroma .sb, .prose .chroma .sc, .prose .chroma .dl, .prose .chroma .s2, .prose .chroma .sh, .prose .chroma .sx, .prose .chroma .s1, .prose .chroma .gi { color: #9ece6a; }
.prose .chroma .ld, .prose .chroma .si { color: #e0af68; }
.prose .chroma .sr { color: #b4f9f8; }
.prose .chroma .go { color: #565f89; }
.prose .chroma .kd, .prose .chroma .ow, .prose .chroma .gh, .prose .chroma .gp, .prose .chroma .gs, .prose .chroma .gu { font-weight: bold; }
.prose .chroma .ge { font-style: italic; }
```

现有行内 code 选择器保持原样；如果审核页尚无独立行内规则，只允许增加 `.prose :not(pre) > code`，不能把 `.prose code` 设置为块级元素。

- [ ] **Step 2: 执行前端类型检查**

Run: `cd web/app && npm run typecheck`

Expected: 退出码为 `0`。

- [ ] **Step 3: 暂不单独提交 app.css**

`app.css` 还会在 Task 4 调整小红书代码块，两个页面的 CSS 在 Task 4 完成后一起提交，避免拆分同一文件的未完成状态。

---

### Task 4: 小红书模板驱动预览与导出主题

**Files:**
- Create: `web/app/src/pages/xiaohongshu/xiaohongshuCodeTheme.ts`
- Modify: `web/app/src/pages/xiaohongshu/xiaohongshuLayout.ts`
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuCardEditor.tsx`
- Modify: `web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx`
- Modify: `web/app/src/styles/app.css`

**Interfaces:**
- Consumes: `XiaohongshuTemplate.codeTheme`、Task 1 的 Chroma class
- Produces: `XiaohongshuCodeTheme`、`TOKYO_NIGHT_CODE_THEME`、`xiaohongshuCodeThemeCSS(theme, scope): string`

- [ ] **Step 1: 创建小红书模板代码主题模块**

主题接口保存容器色和 scoped token 规则；生成器拒绝空 scope，避免意外生成全局 `.k` 规则。token 分组和值与 Task 2 完全一致。

```ts
/** XiaohongshuCodeTheme 描述模板内代码块容器和 Chroma token 色值。 */
export interface XiaohongshuCodeTheme {
  backgroundColor: string;
  borderColor: string;
  textColor: string;
  rules: ReadonlyArray<{ selectors: readonly string[]; declarations: string }>;
}

/** TOKYO_NIGHT_CODE_THEME 精确复用 old 分支的 Tokyo Night Night 配色。 */
export const TOKYO_NIGHT_CODE_THEME: XiaohongshuCodeTheme = {
  backgroundColor: "#1a1b26",
  borderColor: "#2a2d3e",
  textColor: "#c0caf5",
  rules: [
    { selectors: ["w"], declarations: "color:#3b4261" },
    { selectors: ["err", "ne", "nv", "vc", "vg", "vi", "vm", "gd", "gr", "gt"], declarations: "color:#f7768e" },
    { selectors: ["c", "ch", "cm", "c1", "cs", "sd"], declarations: "color:#565f89;font-style:italic" },
    { selectors: ["cp", "cpf", "kn", "nb", "bp", "nc", "nd", "nf", "fm", "nt", "nn"], declarations: "color:#7aa2f7" },
    { selectors: ["kd"], declarations: "color:#7aa2f7;font-weight:bold" },
    { selectors: ["k", "kp", "kr", "se"], declarations: "color:#bb9af7" },
    { selectors: ["kc", "no", "ss", "m", "mb", "mf", "mh", "mi", "il", "mo"], declarations: "color:#ff9e64" },
    { selectors: ["kt"], declarations: "color:#2ac3de" },
    { selectors: ["gp"], declarations: "color:#2ac3de;font-weight:bold" },
    { selectors: ["o"], declarations: "color:#89ddff" },
    { selectors: ["ow"], declarations: "color:#89ddff;font-weight:bold" },
    { selectors: ["na", "ni", "py"], declarations: "color:#73daca" },
    { selectors: ["nl"], declarations: "color:#7dcfff" },
    { selectors: ["l", "s", "sa", "sb", "sc", "dl", "s2", "sh", "sx", "s1", "gi"], declarations: "color:#9ece6a" },
    { selectors: ["ld", "si"], declarations: "color:#e0af68" },
    { selectors: ["sr"], declarations: "color:#b4f9f8" },
    { selectors: ["go"], declarations: "color:#565f89" },
    { selectors: ["gh", "gu"], declarations: "color:#7aa2f7;font-weight:bold" },
    { selectors: ["ge"], declarations: "font-style:italic" },
    { selectors: ["gs"], declarations: "font-weight:bold" },
  ],
};

/** xiaohongshuCodeThemeCSS 为指定模板容器生成带作用域的代码主题。 */
export function xiaohongshuCodeThemeCSS(theme: XiaohongshuCodeTheme, scope: string): string {
  if (!scope.trim()) throw new Error("小红书代码主题缺少作用域");
  const container = `${scope} pre.chroma{background:${theme.backgroundColor};border-color:${theme.borderColor};color:${theme.textColor}}`;
  const tokens = theme.rules.map((rule) => `${rule.selectors.map((selector) => `${scope} .chroma .${selector}`).join(",")}{${rule.declarations}}`).join("");
  return container + tokens;
}
```

- [ ] **Step 2: 将主题挂到唯一模板**

在 `xiaohongshuLayout.ts` 导入类型和值，给 `XiaohongshuTemplate` 增加 `codeTheme: XiaohongshuCodeTheme`，并给 `XIAOHONGSHU_DEFAULT_TEMPLATE` 设置 `codeTheme: TOKYO_NIGHT_CODE_THEME`。不得增加第二个模板。

- [ ] **Step 3: 卡片预览使用当前模板主题**

在 `XiaohongshuCardEditor` 中取得完整模板并注入 scoped style。样式标签放在编辑器 section 的第一个子节点，现有工具栏、页面列表和导航保持其后原顺序；规则只作用于 `.xiaohongshu-card-content`。

```tsx
import { getXiaohongshuTemplate } from "./xiaohongshuLayout";
import { xiaohongshuCodeThemeCSS } from "./xiaohongshuCodeTheme";

const selectedTemplate = getXiaohongshuTemplate(template);

<style>{xiaohongshuCodeThemeCSS(selectedTemplate.codeTheme, ".xiaohongshu-card-content")}</style>
```

- [ ] **Step 4: PNG 快照使用同一模板主题**

在 `XiaohongshuPage.tsx` 导入 `xiaohongshuCodeThemeCSS`。在 `xiaohongshuSnapshotStyles` 当前模板字符串最后的 Mermaid SVG 规则之后、结束反引号之前，直接插入模板主题字符串；同时将该字符串中现有浅色 `pre` 规则的背景、前景、边框改为模板代码主题值。

```ts
${xiaohongshuCodeThemeCSS(template.codeTheme, ".xiaohongshu-snapshot-content")}
```

上面的表达式是插入到现有模板字符串末尾的完整内容，不能放到反引号之外。

- [ ] **Step 5: 更新卡片基础代码块布局**

`app.css` 的 `.xiaohongshu-card-content pre` 保留 `max-width`、padding、字号、行高、`white-space: pre-wrap` 和 `overflow-wrap: anywhere`，基础背景、前景与边框改为 Tokyo Night 值。动态模板 CSS继续覆盖相同颜色，使当前唯一模板和未来模板都能正确工作。`:not(pre) > code` 浅色规则不变。

```css
.xiaohongshu-card-content pre {
  max-width: 100%;
  margin: 13px 0;
  overflow: hidden;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 11px;
  border: 1px solid #2a2d3e;
  border-radius: 5px;
  background: #1a1b26;
  color: #c0caf5;
  font: 12px/1.65 ui-monospace, SFMono-Regular, Menlo, monospace;
}
```

- [ ] **Step 6: 执行前端检查与构建**

Run: `cd web/app && npm run typecheck`

Expected: 退出码为 `0`，模板接口的所有调用点都已补全。

Run: `cd web/app && npm run build`

Expected: Vite 生产构建成功，不出现 CSS 或 React 编译错误。

- [ ] **Step 7: 提交审核页和小红书模板改动**

```bash
git add web/app/src/styles/app.css web/app/src/pages/xiaohongshu/xiaohongshuCodeTheme.ts web/app/src/pages/xiaohongshu/xiaohongshuLayout.ts web/app/src/pages/xiaohongshu/XiaohongshuCardEditor.tsx web/app/src/pages/xiaohongshu/XiaohongshuPage.tsx
git commit -m "feat(web): 统一 Tokyo Night 代码高亮"
```

---

### Task 5: 整体构建和页面验收

**Files:**
- Verify only: Tasks 1-4 中列出的文件

**Interfaces:**
- Consumes: Tasks 1-4 的完整渲染链路
- Produces: 可供用户从浏览器验收的本地服务地址

- [ ] **Step 1: 复查改动边界**

Run: `git diff 916a38d..HEAD -- internal/content/markdown/render.go internal/domain/template/tokyo_night.go internal/domain/template/builtin.go internal/provider/publish/wechat/render.go web/app/src/styles/app.css web/app/src/pages/xiaohongshu`

Expected: 只有 Tokyo Night 语义输出、模板 token、预览和导出相关改动；没有 Hugo、设置页、数据库或 editorial 改动。

- [ ] **Step 2: 执行全项目编译和前端生产构建**

Run: `go test -run '^$' ./...`

Expected: 所有 Go 包和现有测试文件完成编译，不运行测试用例。

Run: `cd web/app && npm run typecheck && npm run build`

Expected: 两条命令退出码均为 `0`。

- [ ] **Step 3: 启动本地服务**

先读取 README 中的启动命令和当前端口；若端口已被占用，选择相邻空闲端口。保持服务运行并把 URL 提供给用户。

- [ ] **Step 4: 浏览器检查三条页面链路**

使用一篇包含 Go 或 TypeScript 围栏代码、标题内行内代码和 Mermaid 的文章检查：

- 审核页代码块背景为 `#1a1b26`，注释、关键字、字符串、数字、函数名颜色可区分；
- 标题和正文中的行内 code 仍为浅色并继承所在字号；
- 微信预览颜色正确，点击复制有反馈，手工复制 HTML 中的 token span 带行内 `style`；
- 小红书每页预览颜色正确，长行不溢出，Mermaid 显示图片而不是源码；
- 导出的 PNG 与预览色值、换行和内容边界一致。

- [ ] **Step 5: 报告结果和残余风险**

向用户提供提交哈希、实际执行的验证命令、本地 URL。若浏览器受剪贴板权限限制，明确区分浏览器权限失败与生成 HTML 丢失样式；不声称未完成的人工复制目标已经验证。
