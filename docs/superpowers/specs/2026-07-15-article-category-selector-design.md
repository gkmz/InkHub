# 文章 Category 选择器设计

## 1. 目标

文章审核页的 Category 不再依赖自由输入，而是优先从当前工作区 Taxonomy Provider 的 `category` 快照中选择。用户找不到合适类目时，可以在不离开文章的情况下复用 Hugo 原生文件预览、确认创建流程；创建成功后新类目自动成为当前文章草稿值，但仍需用户点击“保存到文章”才写回 Obsidian。

本阶段不改造 Series 和 Tags，不改变文章元数据写回协议，也不把 taxonomy term 存入文章之外的新位置。

## 2. 方案选择

### 方案 A：ArticlePage 统一加载并下发 taxonomy（采用）

`ArticlePage` 与文章详情并行读取 taxonomy 快照，将 category 候选、可写状态和创建回调传给 `MetadataForm`。`MetadataForm` 只维护文章草稿，不直接调用 taxonomy API。创建成功后 `ArticlePage` 更新快照，表单通过回调把新 term 名称写入草稿。

优点是请求与业务状态集中、数据流单向，表单可以继续独立测试；同一页面不会出现多个 taxonomy 请求或 revision 副本。

### 方案 B：MetadataForm 自行读取和创建

组件内部请求 taxonomy 并打开创建对话框。实现文件较少，但元数据表单会同时承担文章草稿、Provider 状态和文件变更三类职责，后续 Series/Tags 接入时更难维护，因此不采用。

### 方案 C：只跳转到类目管理页

文章页只提供“管理类目”链接，用户创建后返回并重新加载。实现最简单，但中断审核流程，且无法可靠恢复未保存草稿，因此不采用。

## 3. 组件与数据流

### 3.1 ArticlePage

- 页面加载时并行请求文章详情与 `GET /api/v1/taxonomy`，taxonomy 失败不能阻止文章打开。
- 分别保存 `TaxonomyOverview | null` 和显式加载状态；请求结束后无论成功或失败都不能继续显示“正在读取”。
- 从快照筛选 `kind === "category"` 的 term，按 Provider 返回顺序传入表单。
- Provider 非只读且同时存在 `provider_id`、`revision` 时允许新建。`failed` 状态若保留了最近成功快照和 revision，仍允许 Provider 在预览时重新校验；真正冲突由服务端拒绝。
- 打开现有 `CreateTaxonomyTermDialog`，创建成功后更新快照，并把新 term 名称回填到表单草稿。

### 3.2 MetadataForm

新增可选属性：

```ts
interface CategoryOption {
  key: string;
  name: string;
}

interface MetadataFormProps {
  categoryOptions?: CategoryOption[];
  categoryState?: "loading" | "ready" | "unavailable";
  canCreateCategory?: boolean;
  onCreateCategory?: (select: (name: string) => void) => void;
}
```

Category 控件使用原生 `select`，保留键盘操作和移动端可用性。选项顺序如下：

1. “未分类”空值。
2. 当前文章值不在快照中时，增加“当前值（博客中未发现）”，避免打开旧文章后静默清空。
3. Provider 快照中的全部 category；按 `name` 去重，当前旧值与快照同名时只显示一次。
4. 可写时在控件旁显示带加号图标的“新建类目”按钮。

`onCreateCategory` 接收一次性 `select(name)` 回调。对话框创建成功后调用该回调，只修改表单 draft；文章源文件仍由原有“保存到文章”按钮统一写回。

### 3.3 CreateTaxonomyTermDialog

复用现有 Provider 预览和确认逻辑，新增可选 `onCreated(termName)` 回调。应用成功顺序：更新页面 taxonomy 快照、回填表单草稿、显示成功 Toast、关闭对话框。

## 4. 状态与错误处理

- taxonomy 加载中：Category 暂时显示当前值并禁用新建按钮，不阻止编辑其他字段。
- taxonomy 请求失败：状态切换为 `unavailable`，保留当前值并显示“博客类目暂不可用”，不能永久停留在加载状态。
- 未配置 Hugo：保留当前值，显示“尚未连接博客类目”，不显示新建入口。
- 快照刷新失败但存在旧 terms：仍可选择旧 terms；页面不在文章审核流程中自动刷新。
- 当前值不在快照：明确标记为旧值，用户可以保留或改选，禁止自动替换。
- 新建预览或应用失败：保留对话框和表单草稿，错误由全局 Toast 展示。
- revision 冲突：沿用服务端“请刷新后重试”提示，不写回文章。
- source changed：沿用现有保存禁用规则；选择或新建类目不能绕过该规则。

## 5. 测试与验收

1. `MetadataForm` 展示 category 下拉并能选择快照中的类目。
2. 当前 category 不在快照时仍显示并可保留。
3. 未配置或 taxonomy 请求失败时，文章页仍可打开并编辑其他元数据。
4. 点击“新建类目”先展示 Hugo 文件预览，确认后新 term 自动回填 Category 草稿。
5. 创建类目后不会立即调用文章 metadata 写回接口；只有点击“保存到文章”才写回。
6. source changed 时保存仍禁用。
7. 桌面和移动端控件不溢出，按钮有明确图标、名称和反馈。

## 6. 非目标

- 不在本阶段实现 Series 选择器。
- 不在本阶段实现 Tags 多选、alias 或准入治理。
- 不自动创建文章中已有但 Hugo 未发现的 category。
- 不改变 Hugo term page、SQLite snapshot 或文章 frontmatter 的现有数据模型。
