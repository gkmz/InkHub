# 文章 Tags 多选与 AI 建议设计

## 1. 目标

文章审核页将 Tags 从逗号分隔的自由输入框改为可搜索、可创建的多选编辑器。编辑器查询 InkHub 已持久化的 Hugo taxonomy 快照，展示标准 Tag 和对应文章数量；用户可以选择已有 Tag，也可以直接创建新 Tag。

AI 在手工编辑能力之上生成结构化 Tag 候选，用户逐项采用或忽略。AI 不覆盖当前草稿，也不成为编辑 Tags 的前置条件。

## 2. 产品决策

本设计替代 PRD 和交互文档中原有的“新 Tag 准入”“仅本次使用”和低频豁免流程：

- 新 Tag 不需要审批、白名单或低频豁免。
- 新 Tag 不单独创建 Hugo taxonomy term page。
- 用户保存文章后，新 Tag 先进入知识库文章；文章发布到 Hugo 后进入博客 frontmatter，下一次 taxonomy 刷新时自然入库。
- 3 至 6 个 Tags 是编辑建议，不是保存阻断规则。
- Tag 治理后续只用于发现低频、重复、近义和无 Tag 内容，不阻断日常文章编辑。

该决策不改变 Hugo 的权威来源地位。InkHub 的 SQLite 仍是可重建的持久化投影，不成为第二套 taxonomy 权威数据。

## 3. 数据架构

Hugo Taxonomy Provider 从 Hugo 配置、文章 frontmatter 和 taxonomy term page 发现 Tags，生成包含名称、标识、使用次数和元数据的完整快照。Taxonomy Service 在刷新成功后用单个事务替换当前 Provider 的 SQLite 投影；刷新失败时保留最近一次成功快照。

文章页通过现有 `/api/taxonomy` 接口读取 SQLite 快照，不在每次打开文章时直接扫描 Hugo：

```text
Hugo 配置 + 文章 frontmatter + Tag term pages
                    ↓ Discover
            Hugo Taxonomy Provider
                    ↓ Snapshot
         SQLite taxonomy_terms
                    ↓ /api/taxonomy
          文章 Tags 编辑器与 AI 候选
```

`TaxonomyTerm.usage_count` 直接用于候选列表的“X 篇文章”文案。文章当前值不在快照中时继续保留，并标记“博客中未发现”。

## 4. TagMultiSelect

新增无 API 依赖的 `TagMultiSelect` 组件。组件接收当前值、Tag 候选、taxonomy 状态和变更回调，不负责请求、文章草稿或持久化。

建议接口：

```ts
export interface TagOption {
  key: string;
  name: string;
  usageCount: number;
}

export interface TagMultiSelectProps {
  value: string[];
  options: TagOption[];
  state: TaxonomyFieldState;
  onChange: (value: string[]) => void;
}
```

### 4.1 已选项

- 每个 Tag 显示为可删除项，删除按钮具有明确的无障碍名称。
- 快照外旧值正常显示，并标记“博客中未发现”。
- 按大小写不敏感比较，避免同一个 Tag 重复出现。
- 若用户选择的名称与快照 key 或名称匹配，使用快照中的标准显示名称。

### 4.2 搜索与创建

- 输入时按名称和 key 搜索当前快照。
- 候选显示名称和文章数量，例如 `Go · 18 篇文章`。
- 已选 Tag 不再出现在候选中。
- 上下方向键移动高亮项，回车采用高亮项，Esc 关闭候选。
- 没有精确匹配时提供 `创建“输入值”`；回车采用该新 Tag。
- 输入框为空时按退格删除最后一个 Tag。
- taxonomy 未连接或请求失败时仍允许创建和删除，但明确提示无法提供已有 Tag 建议与文章数量。

### 4.3 数量提示

- 少于 3 个时显示“建议至少选择 3 个 Tag”。
- 超过 6 个时显示“建议最多选择 6 个 Tag”。
- 提示不禁用保存按钮，不改变后端保存契约。

## 5. 文章页集成

`ArticlePage` 保持一次 taxonomy 请求，从快照筛选 `kind === "tag"` 的 terms，并映射为 `TagOption`。`MetadataForm` 使用 `TagMultiSelect` 更新自己的文章草稿；用户点击“保存到文章”后才调用现有元数据保存接口。

创建新 Tag 不调用 `/taxonomy/terms/preview` 或 `/taxonomy/terms/apply`，也不隐式写入 Hugo。保存前的字段级变更摘要继续将 Tags 作为数组比较。

taxonomy 状态沿用 Category/Series 的 `loading`、`ready`、`unavailable` 和 `not_enabled`，但 Tag 组件提供符合自身语义的中文反馈。

## 6. AI Tag 建议

AI 只返回建议名称和理由；`existing` 与使用次数必须由 Application 根据本次请求使用的 SQLite taxonomy 快照补充，不能信任模型判断。页面使用的结构化数据为：

```ts
export interface AITagSuggestion {
  name: string;
  existing: boolean;
  usageCount: number;
  reason: string;
}
```

AI Provider 的原始结构只包含 `name` 和 `reason`。Application 先完成空值、重复和数量校验，再按大小写不敏感的 name/key 匹配快照，生成 `AITagSuggestion`。因此页面展示的“已有 Tag”和文章数量始终来自 InkHub 持久化投影，而不是模型输出。

生成请求包含文章标题、摘要、用户允许的正文范围，以及 Application 从 SQLite 快照召回的现有 Tag 候选。AI 优先选择候选，也可以提出新 Tag。

页面行为：

- 已有 Tag 显示文章数量，新 Tag 显示“新 Tag”。
- 用户逐项采用或忽略。
- 采用只追加到当前 Tags 草稿，不清空已有选择，不重复添加，也不立即保存文章。
- 文章内容版本变化后，旧建议标记失效并禁止采用。
- 未配置 AI 时不显示生成入口，但手工 Tag 编辑始终可用。
- AI 请求失败时保留表单草稿，提供可重试反馈。

第一版不增加向量数据库或语义检索。候选召回先使用现有 taxonomy 名称、文章信息和使用次数，保持结果可解释，并为以后替换召回策略保留 Application 层接口。

## 7. 领域规则调整

`ValidateTags` 保留以下确定性行为：

- 去除首尾空格。
- 空值不进入结果。
- 按大小写不敏感去重。
- 命中现有标准 Tag 时使用标准名称。

删除 `Allowed` 白名单拒绝新 Tag 的产品约束。数量范围只产生检查建议，不由元数据保存接口拒绝。alias 自动合并不在本阶段实现，避免在没有用户可见治理关系时静默改变文章值。

## 8. 错误与边界

- taxonomy 加载中：保留文章 Tags，可继续删除或输入，显示加载状态。
- taxonomy 未配置：保留文章 Tags，允许手工编辑，提示尚未连接博客标签。
- taxonomy 请求失败：保留文章 Tags，允许手工编辑，提示博客标签暂不可用。
- 快照包含重复名称：按大小写不敏感名称去重，保留首个稳定候选。
- 用户重复输入：不追加，并清空本次输入。
- AI 返回重复或空名称：Application 校验后丢弃无效项；模型不能提供或覆盖使用次数，结果不能直接进入草稿。
- 新 Tag 保存成功但尚未发布：继续作为快照外值显示；发布并刷新 taxonomy 后自动变为标准候选。

## 9. 测试与验收

### 9.1 组件测试

- 展示已选 Tag、使用次数和快照外标记。
- 搜索候选，隐藏已选项。
- 鼠标选择、键盘高亮、回车创建、Esc 关闭和退格删除。
- 大小写不敏感去重与标准名称回填。
- 未配置和失败状态不阻止手工编辑。
- 3 至 6 个数量提示不禁用保存。

### 9.2 页面测试

- 文章页从 taxonomy 快照筛选 Tag 并显示文章数量。
- 调整 Tags 后只更新草稿，点击保存才写回文章。
- 创建新 Tag 不调用 taxonomy term 变更接口。
- taxonomy 请求失败不阻止文章打开和 Tags 编辑。
- AI Tag 建议逐项采用、忽略、去重和失效保护。

### 9.3 回归

- Category 与 Series 选择和创建保持不变。
- Keywords 仍是独立的标准字段，不复用 Tag 组件。
- 运行前端全量测试、类型检查、lint 和生产构建。
- 运行 `go test ./...` 与 `go vet ./...`。

## 10. 非目标

- Tag term page 的创建、编辑和删除。
- alias 自动合并、近义词推荐和批量治理。
- 低频豁免、核心 Tag 和准入审批。
- 向量数据库、embedding 或语义搜索。
- 发布前自动修改用户选择的 Tags。
