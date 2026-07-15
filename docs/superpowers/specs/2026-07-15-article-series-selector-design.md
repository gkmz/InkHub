# 文章 Series 选择器与单值 Taxonomy 字段设计

## 1. 目标

文章审核页的 Series 从自由输入改为当前 Taxonomy Provider 快照的单值选择器，并允许用户在同一页面预览、确认创建 Hugo series term page。实现时抽出 Category 与 Series 共用的单值 taxonomy 字段，消除两套选项、旧值和状态逻辑，同时保持现有 Category 行为不变。

Series 创建成功只回填文章表单草稿；用户仍需点击“保存到文章”才写回 Obsidian。

## 2. 范围

本阶段包含：

- Category 与 Series 共用单值 taxonomy 字段组件。
- Series 快照选择、旧值兼容和状态提示。
- 可写 Provider 的 Series 创建预览和确认。
- Category 现有选择与创建流程无回归迁移。

本阶段不包含：

- Tags 多选、alias、相似词和准入治理。
- Series 删除、重命名或合并。
- Taxonomy 管理页开放 Series 创建。
- 后端 taxonomy API 或数据模型变更。

## 3. 方案选择

### 方案 A：单值字段与通用创建对话框（采用）

抽出 `SingleTaxonomyField`，由配置决定 label、当前值、候选、状态、空值文案和创建命令。`CreateTaxonomyTermDialog` 接收 term kind 与中文文案，不再把 `category`、“类目”写死。Category 和 Series 复用这两个边界，Tags 保持独立。

该方案在不建立通用表单框架的前提下消除真实重复，符合 category 和 series 都是“最多一个字符串值”的领域约束。

### 方案 B：复制 Category 代码

在 `MetadataForm` 和 `ArticlePage` 各复制一套 Series 状态与回调。初始改动较小，但错误状态、旧值兼容和创建时序会出现两份，后续修复容易漂移，因此不采用。

### 方案 C：统一全部 taxonomy 编辑

同时抽象 Category、Series 和 Tags。Tags 是多值集合，还涉及 alias 和人工准入，与单值字段的数据和交互模型不同；此时统一会产生条件分支和不稳定接口，因此不采用。

## 4. 组件设计

### 4.1 SingleTaxonomyField

新建 `web/app/src/components/SingleTaxonomyField.tsx`，公开接口：

```ts
export type TaxonomyFieldState = "loading" | "ready" | "unavailable" | "not_enabled";

export interface TaxonomyFieldOption {
  key: string;
  name: string;
}

interface SingleTaxonomyFieldProps {
  label: "Category" | "Series";
  noun: "类目" | "系列";
  value: string;
  options: TaxonomyFieldOption[];
  state: TaxonomyFieldState;
  emptyLabel: string;
  canCreate: boolean;
  onChange: (value: string) => void;
  onCreate?: (select: (name: string) => void) => void;
}
```

组件职责：

- 使用原生 `select` 展示空值、当前旧值和 Provider 候选。
- 按 `name` 去重；当前值与候选同名时只出现一次。
- 当前值不在快照中时显示“当前值（博客中未发现）”，不得清空。
- 根据 state 显示“正在读取博客类目”“尚未连接博客类目”“博客类目暂不可用”或“来自当前博客”。这里“博客类目”表示整个 taxonomy 来源，不因字段类型改变。
- 可创建时显示加号图标按钮，accessible name 分别为“新建类目”“新建系列”。
- 组件不请求 API、不维护文章草稿、不打开对话框。

`MetadataForm` 继续维护统一 draft，向两个字段传入 `update("category", value)` 和 `update("series", value)`。现有 `CategoryOption`、`CategoryState` 类型迁移为通用类型，不保留重复别名。

### 4.2 CreateTaxonomyTermDialog

对话框新增配置：

```ts
interface CreateTaxonomyTermDialogProps {
  kind?: "category" | "series";
  noun?: "类目" | "系列";
  // 其余现有属性不变
}
```

默认值保持 `kind="category"`、`noun="类目"`，保证类目管理页调用方无需修改即可保持现有行为。

参数化内容包括：

- 对话框标题：“新建类目”或“新建系列”。
- 表单标签：“类目名称/说明”或“系列名称/说明”。
- command.kind：`category` 或 `series`。
- 按钮、进度和成功 Toast 文案。

文件路径和 frontmatter 内容仍完全由 Taxonomy Provider 生成，客户端不能提交任意文件变更。

### 4.3 ArticlePage

`ArticlePage` 保持一次 taxonomy 请求和一个 `TaxonomyOverview` 状态。分别从 `terms` 筛选 `category`、`series`，传给 `MetadataForm`。

创建状态改为带 kind 的对象：

```ts
type PendingTaxonomySelection = {
  kind: "category" | "series";
  select: (name: string) => void;
} | null;
```

打开对话框时按 kind 提供 noun。创建成功后更新同一 taxonomy 快照，仅调用对应字段的 select 回调，禁止改变另一个字段。

Category 和 Series 的创建能力都由同一条件决定：Provider 非只读，且存在 `provider_id` 和 `revision`。`failed` 状态保留旧快照和 revision 时仍允许 Provider 在预览阶段重新校验。

## 5. 状态与错误处理

- taxonomy 加载中：两个字段保留当前值并显示加载状态，不显示创建按钮。
- 未连接博客：两个字段显示明确引导，仍允许清空或保留当前文章值。
- taxonomy 请求失败：文章正常打开，两个字段显示暂不可用，不阻止编辑其他元数据。
- 当前 Series 不在快照：显示旧值标记，禁止自动创建或替换。
- 创建 Series 失败或 revision 冲突：保留对话框与文章草稿，显示服务端错误。
- 创建 Series 成功：更新快照并回填 Series draft，不发送文章 metadata PUT。
- source changed：选择和创建 term 不绕过现有保存禁用规则。

## 6. 测试与验收

1. 通用字段对 Category、Series 都能选择候选并生成对应草稿变更。
2. 两个字段的快照外旧值均被保留和标记。
3. 候选按名称去重，不产生 React 重复 key。
4. Category 现有选择、新建、预览和确认测试继续通过。
5. Series 创建请求提交 `kind="series"`，并展示 Provider 返回的 Hugo series 文件路径。
6. Series 创建后只回填 Series，不改变 Category，也不提前保存文章。
7. 未配置和请求失败不阻止文章审核。
8. 桌面与移动端两个字段、创建按钮和状态文案不溢出。

## 7. 安全与兼容

- 不改变后端接口；继续使用服务端 revision 冲突检测和重新规划。
- 不允许浏览器提供目标文件路径或内容。
- 对话框默认 category 配置保证类目管理页向后兼容。
- 不改变 `ArticleMetadata.series` 的字符串类型、frontmatter 路径或内容版本规则。
