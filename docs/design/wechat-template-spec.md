# InkHub 微信模板规范

> 规范版本 1.1，对应 PRD 1.5、架构设计 1.2 和 Provider 契约。

## 1. 目标与边界

InkHub 微信模板是无执行代码的静态资源包，用于将标准 Markdown 渲染结果转换为可预览、可复制到微信公众号后台的内联样式 HTML。

本规范保证：

- Default、Minimal 和第三方模板经过相同校验与渲染链路。
- 模板不能执行脚本、读取任意本地文件或扩大网络访问范围。
- 相同模板版本内容不可变，安装、更新和回滚可验证。
- 模板变量有限、类型安全，不演变成页面设计器。
- 输出可在 InkHub 预览，并能稳定复制到微信公众号编辑器。

模板只负责视觉样式和少量展示变量，不负责 Markdown 解析、图片上传、Mermaid 转换、剪贴板、文章状态或用户确认。

## 2. 模板包结构

模板包是 zip 文件，解压后必须只有一个以模板 ID 命名的根目录：

```text
inkhub-default/
├── template.yaml
├── styles.css
├── preview.md
├── preview.png
└── assets/
    └── logo.png
```

必需文件：`template.yaml`、`styles.css`、`preview.md`、`preview.png`。`assets/` 可为空。禁止符号链接、硬链接、设备文件、隐藏可执行文件和嵌套压缩包。

MVP 限制：

- 压缩包最大 10 MiB。
- 解压后最大 25 MiB、最多 200 个文件、单文件最大 8 MiB。
- 路径使用 UTF-8、正斜杠和相对路径；禁止绝对路径、`..`、空路径和大小写冲突。
- 允许扩展名：`.yaml`、`.css`、`.md`、`.png`、`.jpg`、`.jpeg`、`.gif`、`.webp`。MVP 不接收 SVG、字体文件和其他主动内容格式。

## 3. `template.yaml`

### 3.1 完整示例

```yaml
specVersion: "1.1"
target: wechat-html
format: css
renderer: wechat-html-v1
compatibility:
  providers: [wechat]
  rendererVersion: "1"
id: inkhub-default
name: InkHub Default
description: InkHub 默认微信公众号模板
author:
  name: InkHub
  url: https://github.com/inkhub
license: Apache-2.0
version: 1.0.0
inkhubVersion: ">=1.0.0 <2.0.0"
entry: styles.css
preview:
  markdown: preview.md
  image: preview.png
elements:
  - paragraph
  - heading-1
  - heading-2
  - heading-3
  - blockquote
  - unordered-list
  - ordered-list
  - table
  - link
  - image
  - inline-code
  - code-block
  - callout
  - divider
  - footnote
variables:
  accentColor:
    type: color
    label: 强调色
    default: "#1677ff"
  bodyFont:
    type: font-family
    label: 正文字体
    default: system-sans
    options: [system-sans, system-serif]
  bodySize:
    type: font-size
    label: 正文字号
    default: 16
    min: 14
    max: 18
    unit: px
  paragraphSpacing:
    type: spacing
    label: 段落间距
    default: 16
    min: 8
    max: 28
    unit: px
  codeTheme:
    type: enum
    label: 代码主题
    default: light
    options: [light, dark]
  showBranding:
    type: boolean
    label: 显示文末品牌
    default: false
    trueValue: block
assets:
  - path: assets/logo.png
    mediaType: image/png
    sha256: 6f5902ac237024bdd0c176cb93063dc4...
    source: https://github.com/inkhub
    license: Apache-2.0
files:
  - path: styles.css
    sha256: 60303ae22b9988617223c7c3c2b19fe...
  - path: preview.md
    sha256: 1f8ac10f23c5b5bc1167bda84b833e5c...
  - path: preview.png
    sha256: 2d711642b726b04401627ca9fbac32f5...
  - path: assets/logo.png
    sha256: 6f5902ac237024bdd0c176cb93063dc4...
```

示例中的摘要使用截断展示；实际文件必须是 64 位小写十六进制 SHA-256。

### 3.2 字段约束

| 字段 | 必需 | 约束 |
| --- | --- | --- |
| `specVersion` | 是 | 精确匹配 InkHub 支持的规范 major |
| `target` | 是 | 微信模板固定为 `wechat-html` |
| `format` | 是 | 微信模板固定为 `css` |
| `renderer` | 是 | 当前固定为 `wechat-html-v1` |
| `compatibility` | 是 | 必须包含 `wechat` Provider 和受支持 Renderer 版本 |
| `id` | 是 | `^[a-z][a-z0-9-]{2,63}$`，安装后不可修改 |
| `name` | 是 | 1-80 个 Unicode 字符，纯文本 |
| `description` | 是 | 1-240 个 Unicode 字符，纯文本 |
| `author.name` | 是 | 1-80 个字符 |
| `author.url` | 否 | 仅允许 HTTPS URL |
| `license` | 是 | SPDX license expression |
| `version` | 是 | SemVer 2.0.0，不允许构建元数据 |
| `inkhubVersion` | 是 | 受支持的 SemVer range |
| `entry` | 是 | 必须是根目录下唯一 CSS 入口 |
| `preview` | 是 | 指向包内 Markdown 和 PNG |
| `elements` | 是 | 非空、去重，只能使用标准元素枚举 |
| `variables` | 否 | 最多 20 个，变量名使用 lowerCamelCase |
| `assets` | 否 | 每项声明路径、mediaType 和 SHA-256 |
| `files` | 是 | 覆盖除 `template.yaml` 外的全部文件且不得包含额外路径 |

未知顶层字段在规范 1.x 中校验失败，防止拼写错误被静默忽略。

历史 1.0 模板没有目标字段，加载时按 `wechat-html`、`css`、`wechat-html-v1` 和 `wechat` Provider 规范化；InkHub 不改写原模板包。新发布模板必须使用 1.1 并显式声明这些字段。

YAML 只接受 UTF-8、单文档和 YAML 1.2 Core Schema。重复 key、锚点、alias、自定义 tag、多文档和超过 32 层的嵌套结构必须失败，避免类型歧义和 alias 扩展攻击。模板根目录名必须与 `id` 完全一致。

每个 asset 还必须声明 `source` 和 SPDX `license`；自有资源可将 `source` 写为作者主页。图片文件必须通过文件签名和完整解码校验，拒绝动画、超大像素尺寸和与扩展名不符的内容。

## 4. 模板变量

支持六种变量类型：

| 类型 | 值 | 必需约束 |
| --- | --- | --- |
| `color` | `#RRGGBB` 或 `#RRGGBBAA` | 不接受命名颜色、函数或 CSS 变量 |
| `font-family` | 预定义 token | 必须提供非空 `options` |
| `font-size` | 数字 | `min`、`max`、`unit` 必需，unit 仅 `px` |
| `spacing` | 数字 | `min`、`max`、`unit` 必需，unit 仅 `px` |
| `enum` | 字符串 token | `options` 2-20 项 |
| `boolean` | `true/false` | 不允许字符串替代，必须声明受控 `trueValue`，只能映射为 `display` 值 |

每个变量必须声明 `label` 和 `default`，默认值必须通过自身约束。用户值只保存在工作区配置中，不修改模板包。

`font-family` 的 options 只能引用 InkHub 规范定义的安全 token。规范 1.0 内置 `system-sans`、`system-serif` 和 `system-mono`，由渲染器映射为跨平台字体栈；模板不得携带或下载字体文件。

CSS 使用受控占位语法：

```css
.inkhub-root {
  color: {{ color.accentColor }};
  font-family: {{ font-family.bodyFont }};
  font-size: {{ font-size.bodySize }};
}
```

占位符只能作为完整属性值，不能出现在 selector、属性名、URL、注释或字符串片段中。InkHub 根据变量类型生成规范化 CSS token，不执行通用模板表达式。

`boolean` 占位符只能用于 `display` 属性，`true` 映射为模板声明的允许显示值，`false` 映射为 `none`；模板必须在变量中增加 `trueValue`，且只能是 `block`、`inline` 或 `inline-block`。它不能控制 HTML 结构或任意属性。

## 5. 标准元素和选择器

模板 CSS 只能以 `.inkhub-root` 为根选择器，避免影响微信公众号编辑器其他内容。标准元素包括：

```text
paragraph, heading-1, heading-2, heading-3,
blockquote, unordered-list, ordered-list, list-item,
table, table-head, table-row, table-cell,
link, image, inline-code, code-block, callout,
divider, footnote, strong, emphasis
```

渲染器为标准 HTML 元素添加稳定的 `data-inkhub-element` 属性。模板可以使用元素、class、`data-inkhub-element` 精确匹配和最多三层后代选择器；禁止 ID、通配符、其他属性匹配、全部伪类、全部伪元素和组合器 `+`、`~`、`>`。

模板必须至少覆盖 `elements` 中声明的每一种元素。Default 和 Minimal 必须覆盖全部标准元素。`preview.md` 禁用原始 HTML，必须包含全部声明元素；`preview.png` 必须是 1200×1600 至 2400×3200、sRGB、无动画的 PNG。

## 6. CSS 安全与允许列表

允许的 CSS 属性：

```text
color, background-color,
font-family, font-size, font-style, font-weight,
line-height, text-align, text-decoration,
white-space, word-break, overflow-wrap,
margin, margin-top, margin-right, margin-bottom, margin-left,
padding, padding-top, padding-right, padding-bottom, padding-left,
border, border-width, border-style, border-color,
border-top, border-right, border-bottom, border-left,
border-radius, width, max-width, min-width, height,
display, vertical-align, list-style-type,
border-collapse, border-spacing, box-sizing
```

属性值规则：

- 禁止 `!important`、`var()`、`env()`、`calc()`、`expression()` 和 vendor-specific hack。
- 禁止 `url()`，资源只能通过声明的 asset 引用并由渲染器转换。
- 禁止 `position`、`z-index`、`transform`、`animation`、`transition`、`filter` 和固定视口单位。
- `display` 只允许 `block`、`inline`、`inline-block`、`table`、`table-row`、`table-cell`、`none`。
- 长度单位只允许 `px`、`em`、`rem`、`%`；禁止负值，除非规范明确允许。
- 颜色只允许十六进制和 `rgb/rgba` 常量。

CSS 禁止 `@import`、`@font-face`、`@keyframes`、`@supports`、`@page` 和未知 at-rule。MVP 允许受控 `@media` 仅用于预览，不进入最终复制 HTML；模板校验器会分别验证预览 CSS 和可内联 CSS。

属性值必须通过与 `specVersion` 一同发布的逐属性允许表；未列出的 keyword、函数、单位或组合全部失败。InkHub 升级允许表时只能在相同 major 内增加向后兼容值，禁止静默放宽 URL、执行或定位能力。

## 7. HTML 输出安全

模板不包含 HTML 文件，也不能生成 HTML 结构。最终 HTML 由 InkHub 渲染器生成并清理：

- 删除 `script`、`iframe`、`object`、`embed`、`form`、`input`、`button`、`style` 和事件属性。
- URL scheme 只允许 `https`、`http`、`mailto` 和渲染阶段内部资源 token。
- 最终复制前不得存在本地路径、`file:`、`data:text/html`、JavaScript URL 或未解析占位符。
- 模板 CSS 内联完成后删除 class 和非必要 `data-*`，保留语义 HTML 与安全属性。
- 图片必须已经上传为公开 HTTPS URL；任一图片处理失败则不生成可交付 artifact。

## 8. 校验、安装和更新

校验顺序：

1. 在受限临时目录流式读取 zip，检查大小、数量、压缩比、路径和文件类型，不先整体解压到内存。
2. 解析 `template.yaml`，验证 schema、SemVer 和兼容范围。
3. 校验 `files` 完整覆盖除 manifest 外的文件和所有 SHA-256；manifest 由包级 SHA-256 保护，避免自引用摘要。
4. 校验 asset media type、实际文件签名和许可证声明。
5. 解析 CSS AST，验证 selector、属性、值、变量和 at-rule。
6. 渲染标准 `preview.md`，检查元素覆盖、未解析 token 和 HTML 安全。
7. 生成确定性渲染摘要，供安装和回归测试使用。

安装使用 staging：校验成功后原子重命名为不可变版本目录，再在 SQLite 短事务中切换工作区活动版本，不使用文件系统符号链接。更新失败继续使用旧版本。相同 `id + version` 只接受完全相同的包摘要；内容不同返回冲突。卸载当前模板前必须先切换工作区默认模板，内置 Default 不能卸载。

## 9. 模板仓库 `index.json`

```json
{
  "specVersion": "1.0",
  "generatedAt": "2026-07-13T04:00:00Z",
  "templates": [
    {
      "id": "inkhub-default",
      "name": "InkHub Default",
      "description": "InkHub 默认微信公众号模板",
      "author": "InkHub",
      "license": "Apache-2.0",
      "version": "1.0.0",
      "templateSpecVersion": "1.0",
      "inkhubVersion": ">=1.0.0 <2.0.0",
      "downloadURL": "https://example.org/inkhub-default-1.0.0.zip",
      "packageSHA256": "8f14e45fceea167a5a36dedd4bea2543...",
      "packageSize": 120345,
      "previewURL": "https://example.org/inkhub-default-1.0.0.png",
      "publishedAt": "2026-07-13T04:00:00Z"
    }
  ]
}
```

实际 SHA-256 必须是 64 位小写十六进制。索引条目按 `id`、SemVer 排序；相同 `id + version` 不得重复或变更。下载只允许 HTTPS；重定向后仍必须为 HTTPS，且最多 3 次。客户端限制响应大小和超时，并在解压前校验包摘要。索引不可用、解析失败或回退到更旧的 `generatedAt` 时继续使用最近一次已验证索引和已安装模板。

MVP 通过受控 Git 仓库 Pull Request 发布索引，不建立账号、评分、付费和远程执行系统。仓库 CI 必须执行模板校验、标准预览渲染和包不可变性检查。

MVP 的信任根是 InkHub 配置中内置的官方 HTTPS 索引地址和仓库发布权限。自定义索引属于高级设置，安装前必须明确显示来源；索引签名和多维护者签名属于 Release 1 之后的增强，不得以“已签名”误导用户。

## 10. 版本与兼容

- `specVersion` major 不匹配时拒绝安装；minor 高于当前支持版本时拒绝安装。
- 模板 patch 可修复样式但不能改变变量契约。
- 模板 minor 可以增加带默认值的变量或元素样式。
- 模板 major 可以删除变量或改变视觉契约，升级前必须展示变化并允许保留旧版本。
- InkHub 升级后重新校验已安装模板；不兼容版本保持安装但禁用，自动回退到最近兼容版本或 Default。

模板版本目录不可原地修改。回滚只切换当前版本指针，不重新下载已经校验的包。

## 11. Default 和 Minimal 验收

`InkHub Default` 和 `InkHub Minimal` 必须：

- 使用同一 manifest schema、安装器、校验器、变量解析器和 CSS 内联器。
- 覆盖全部标准元素和标准 `preview.md`。
- 在固定变量输入下产生确定性的 HTML 和渲染摘要。
- 输出不包含脚本、style 标签、本地路径、未声明资源或未解析变量。
- 支持中文、英文、长链接、宽表格、代码块、callout、脚注、图片和 Mermaid 结果。
- 复制到真实微信公众号后台后，正文层级、间距、代码、表格和图片可读且无明显错位。

Default 作为完整参考实现；Minimal 使用更少的装饰和变量，但不得绕过标准链路或减少安全检查。

## 12. 测试要求

- manifest 缺字段、未知字段、无效 SemVer、重复变量和错误默认值必须失败。
- Zip Slip、符号链接、压缩炸弹、扩展名欺骗和大小写冲突必须失败。
- 禁止 selector、属性、值、at-rule、脚本 URL 和外链资源必须失败。
- 文件缺失、额外文件、摘要错误和 media type 不匹配必须失败。
- 更新失败保持旧版本，回滚不改变模板包。
- Default、Minimal 和第三方 fixture 走相同黄金测试。
- 标准预览在固定渲染环境生成截图并与基准对比。
- 最终 HTML 通过清理器测试，并在真实微信公众号后台完成粘贴验收。
