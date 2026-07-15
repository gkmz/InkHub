# Provider Runtime、Taxonomy 与模板抽象实施计划

> **执行要求：** 按连续开发模式逐阶段执行；每个阶段使用测试驱动开发，并在提交前完成 reflection 和验证。

## 目标

在不引入动态插件的前提下，让 Provider Registry 成为真实业务主链路，并用 Hugo 标准资源实现可持久化的 taxonomy 管理边界。

## 全局约束

- 采用 Go、SQLite 和现有 React 技术栈，不新增运行时插件系统。
- 每个行为先写失败测试并确认失败，再实现最小代码。
- 关键代码使用中文注释，所有公开方法使用中文文档注释。
- 每个阶段完成后执行 reflection、相关测试、全量测试和 `git diff --check`，通过后按 Conventional Commits 提交。

## 阶段 1：统一设计文档

- 修订 PRD、架构、数据模型、Provider 契约和 handoff。
- 明确 Hugo 标准资源权威、SQLite 持久化投影、TaxonomyProvider 和模板 target。
- 验证：冲突文本扫描、Markdown diff 检查。

## 阶段 2：Provider Runtime 主链路

- 为持久化 Source 和 Publish 实例建立统一解析器。
- bootstrap 注册内置工厂并注入扫描、发布任务。
- 将交付模式移入 Publish Descriptor，消除 Hugo/微信类型分支。
- 验证：Registry、workspace scan、publication runner 单元测试及全量 Go 测试。

## 阶段 3：TaxonomyProvider 和 Hugo 标准发现

- 新增 taxonomy 契约、Registry 注册和构建能力。
- 实现 Hugo 配置发现、frontmatter term 汇总、term page 元数据和 revision。
- 移除 Hugo Publish Provider 对 `data/taxonomy.yaml` 的依赖。
- 验证：TOML/YAML/JSON 配置 fixture、默认 taxonomy、重复 term、外部修改冲突测试。

## 阶段 4：SQLite taxonomy 持久化

- 新增 additive migration、数据库 comment 和 repository。
- 实现快照事务替换、失败保留旧数据和启动/手工刷新用例。
- 验证：migration、repository 原子性、重启读取和失败恢复测试。

## 阶段 5：模板目标与 Renderer

- 扩展 Manifest、TemplateRef 和校验器。
- 迁移内置微信模板默认字段，发布预检校验 target 与 renderer。
- 验证：旧模板迁移、目标不匹配、摘要稳定性和微信渲染回归测试。

## 阶段 6：契约验证与整体回归

- 用最小 Generic Markdown Source 工厂验证新增 Source 不需要修改 Application。
- 检查业务包对具体 Provider import、类型 switch 和私有 taxonomy 文件依赖。
- 运行全量 Go/React 测试、构建、`git diff --check` 和工作区状态审查。
