# Zap 结构化日志实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 InkHub 增加由 `.env` 配置、支持文件轮转和控制台输出的结构化日志。

**Architecture:** 日志构造封装在 `internal/platform/logging`，bootstrap 负责读取配置和生命周期，HTTP middleware 负责请求关联与访问日志。业务代码只接收 `*zap.Logger`，不感知输出实现。

**Tech Stack:** Go、Zap、lumberjack、godotenv、`net/http`

---

### Task 1: 配置解析

**Files:**
- Create: `.env.example`
- Create: `internal/platform/logging/config.go`
- Test: `internal/platform/logging/config_test.go`
- Modify: `internal/app/bootstrap/config.go`
- Modify: `internal/app/bootstrap/config_test.go`

- [ ] 先写测试，覆盖默认值、四个环境变量和非法输入。
- [ ] 运行 `go test ./internal/platform/logging ./internal/app/bootstrap`，确认测试因缺少实现失败。
- [ ] 实现配置解析与校验，并用 `godotenv.Load()` 加载 `.env`。
- [ ] 重跑测试，确认通过。

### Task 2: Zap 与文件轮转

**Files:**
- Create: `internal/platform/logging/logger.go`
- Test: `internal/platform/logging/logger_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] 先写文件 JSON 输出和级别过滤测试。
- [ ] 运行日志包测试，确认因构造器缺失失败。
- [ ] 使用 Zap core 和 lumberjack 实现文件、控制台双输出及 `Sync` 生命周期。
- [ ] 重跑日志包测试，确认通过。

### Task 3: HTTP 请求日志

**Files:**
- Create: `internal/transport/http/access_log.go`
- Test: `internal/transport/http/access_log_test.go`
- Modify: `internal/app/bootstrap/bootstrap.go`

- [ ] 先写 request ID、状态码、大小、耗时和不记录敏感内容的测试。
- [ ] 运行 HTTP 包测试，确认因 middleware 缺失失败。
- [ ] 实现 response writer 包装与访问日志 middleware，并在 bootstrap 顶层装配。
- [ ] 重跑 HTTP 包测试，确认通过。

### Task 4: 启动关键路径日志与回归

**Files:**
- Modify: `internal/app/bootstrap/bootstrap.go`
- Modify: `cmd/inkhub/main.go`

- [ ] 为启动、数据库、扫描、任务恢复、监听和退出增加结构化日志；关键代码添加中文注释。
- [ ] 执行 `gofmt`，检查 `git diff` 与 `git status`。
- [ ] 运行 `go test ./...`、`go test -race ./...`、`go vet ./...`。
- [ ] 反思配置边界、敏感信息、关闭路径和兼容性，发现问题后修复并重新验证。
- [ ] 使用 Conventional Commits 提交完整功能。
