# InkHub

InkHub 是面向 Markdown 内容创作者的本地优先内容工作台，用于文章审核、AI 辅助元数据治理、SEO 检查，以及 Hugo 和微信公众号发布准备。

## 代码边界

```text
cmd/inkhub/   新产品 CLI 入口
internal/     新产品后端实现
docs/         产品、架构和实施文档
testdata/     新产品测试样例
old/          旧 markdown-preview 项目，只作为迁移参考
```

新功能只能进入 `cmd/inkhub`、`internal` 和后续的 `web/app`。禁止继续修改 `old/` 中的业务行为；需要迁移旧能力时，先补黄金测试，再在新架构中重新实现。

## 当前开发状态

当前处于 MVP Release 1 开发阶段。实施顺序和验证要求见 `docs/plans/mvp-implementation-plan.md`。

```bash
go test -race ./...
go build ./cmd/inkhub
```

## 功能质量门禁

每完成一个功能，必须在进入下一功能前完成：

1. 审查领域边界、错误路径、并发、事务和安全行为。
2. 检查所有公开 Go 方法都有中文文档注释，关键事务、补偿、原子写入和安全逻辑有简短中文注释。
3. 为审查发现的问题先补失败测试，再修复实现。
4. 运行相关聚焦测试、`go test -race ./...`、`go vet ./...`、新旧入口构建和 `git diff --check`。
5. 未通过审查或验证时，不得将该功能标记为完成。

旧应用仍可独立构建，用于迁移期间对照行为：

```bash
go build -o /tmp/inkhub-old ./old
```
