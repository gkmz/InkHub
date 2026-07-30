# InkHub 数据目录环境配置实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 支持通过 `.env` 的 `INKHUB_DATA_DIR` 明确配置 InkHub 数据目录，并保持 CLI 覆盖、路径校验、来源日志和旧用户兼容。

**Architecture:** 启动层使用已有 `Layer` 与 `MergeConfig` 合并默认、环境和 CLI 三层配置，通过 `flag.Visit` 只把显式命令行参数加入 CLI 层。合并后统一清理并校验数据目录，再以最终目录解析日志默认路径；业务模块不直接读取环境变量。

**Tech Stack:** Go 1.24、标准库 `flag`/`os`/`path/filepath`、godotenv、zap、Go testing。

## Global Constraints

- 配置优先级固定为：显式 `--data-dir` > `INKHUB_DATA_DIR` > `os.UserConfigDir()/InkHub`。
- 非空数据目录必须是绝对路径，配置值不展开 `~`。
- 未配置新变量时保持现有数据目录，不自动迁移数据库。
- 关键配置合并逻辑使用中文注释，公开方法保留中文文档注释。
- 连续开发模式下，两个任务全部通过和整体回归通过后只做一次聚合提交。

---

### Task 1: 数据目录配置合并与校验

**Files:**
- Modify: `internal/app/bootstrap/bootstrap.go`
- Modify: `internal/app/bootstrap/bootstrap_test.go`

**Interfaces:**
- Consumes: `MergeConfig(layers ...Layer) MergedConfig`、`platformlogging.ParseConfig(dataDir string, lookup LookupEnv)`。
- Produces: `parseServerConfig(args []string) (parsedConfig, error)` 返回合并后的绝对 `Config.DataDir`，并在 `parsedConfig.Origins["data_dir"]` 中记录 `default`、`environment` 或 `cli`。

- [ ] **Step 1: 写环境变量与优先级失败测试**

在 `bootstrap_test.go` 增加表驱动测试，使用绝对临时目录并验证来源：

```go
func TestParseServerConfigMergesDataDirByDocumentedPrecedence(t *testing.T) {
    environmentDir := filepath.Join(t.TempDir(), "environment")
    cliDir := filepath.Join(t.TempDir(), "cli")
    t.Setenv("INKHUB_DATA_DIR", environmentDir)

    fromEnvironment, err := parseServerConfig([]string{"inkhub"})
    if err != nil {
        t.Fatal(err)
    }
    if fromEnvironment.DataDir != filepath.Clean(environmentDir) || fromEnvironment.Origins["data_dir"] != "environment" {
        t.Fatalf("环境配置未生效: %+v", fromEnvironment)
    }

    fromCLI, err := parseServerConfig([]string{"inkhub", "--data-dir", cliDir})
    if err != nil {
        t.Fatal(err)
    }
    if fromCLI.DataDir != filepath.Clean(cliDir) || fromCLI.Origins["data_dir"] != "cli" {
        t.Fatalf("CLI 未覆盖环境配置: %+v", fromCLI)
    }
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./internal/app/bootstrap -run TestParseServerConfigMergesDataDirByDocumentedPrecedence`

Expected: FAIL，当前解析器忽略 `INKHUB_DATA_DIR` 或 `parsedConfig` 没有 `Origins`。

- [ ] **Step 3: 写相对路径拒绝测试**

```go
func TestParseServerConfigRejectsRelativeDataDir(t *testing.T) {
    t.Run("环境变量", func(t *testing.T) {
        t.Setenv("INKHUB_DATA_DIR", "./runtime")
        if _, err := parseServerConfig([]string{"inkhub"}); err == nil || !strings.Contains(err.Error(), "必须使用绝对路径") {
            t.Fatalf("相对环境路径应被拒绝: %v", err)
        }
    })
    t.Run("命令行", func(t *testing.T) {
        t.Setenv("INKHUB_DATA_DIR", "")
        if _, err := parseServerConfig([]string{"inkhub", "--data-dir", "runtime"}); err == nil || !strings.Contains(err.Error(), "必须使用绝对路径") {
            t.Fatalf("相对 CLI 路径应被拒绝: %v", err)
        }
    })
}
```

- [ ] **Step 4: 实现最小配置合并**

在 `parseServerConfig` 中：

1. 默认层设置系统数据目录，来源名为 `default`。
2. `INKHUB_DATA_DIR` 去除首尾空白后非空时加入环境层。
3. 将 `--data-dir` 默认值改为空字符串，通过 `flags.Visit` 判断是否显式传入。
4. 使用 `MergeConfig` 合并三层。
5. 对最终目录调用 `filepath.IsAbs` 和 `filepath.Clean`。
6. 使用最终目录调用 `platformlogging.ParseConfig`。

核心结构：

```go
merged := MergeConfig(
    Layer{Name: "default", Values: defaults},
    environmentLayer,
    cliLayer,
)
if !filepath.IsAbs(merged.Config.DataDir) {
    return parsedConfig{}, fmt.Errorf("数据目录必须使用绝对路径")
}
merged.Config.DataDir = filepath.Clean(merged.Config.DataDir)
```

- [ ] **Step 5: 运行启动配置测试**

Run: `go test ./internal/app/bootstrap -run 'TestParseServerConfig|TestMergeConfig|TestLoadDotEnv'`

Expected: PASS。

- [ ] **Step 6: 功能点 reflection**

检查空环境变量不会覆盖默认值、CLI 未显式传参不会覆盖环境层、错误信息不泄漏 Secret、Windows 绝对路径判断由目标平台标准库负责。发现问题先补测试再修复，不进入 Task 2。

---

### Task 2: 启动日志与用户配置说明

**Files:**
- Modify: `internal/app/bootstrap/bootstrap.go`
- Modify: `.env.example`
- Modify: `README.md`
- Test: `internal/app/bootstrap/bootstrap_test.go`

**Interfaces:**
- Consumes: Task 1 的 `parsedConfig.Origins["data_dir"]`。
- Produces: 启动日志字段 `data_dir` 与 `data_dir_source`；面向用户的 `INKHUB_DATA_DIR` 配置说明。

- [ ] **Step 1: 写日志来源传递失败测试**

抽取或扩展一个无副作用的小函数，使启动日志字段可测试：

```go
func TestStartupConfigLogFieldsIncludeDataDirectoryOrigin(t *testing.T) {
    fields := startupConfigLogFields(Config{DataDir: "/tmp/inkhub"}, "environment")
    if len(fields) != 2 || fields[0].Key != "data_dir" || fields[1].Key != "data_dir_source" {
        t.Fatalf("启动数据目录日志字段不完整: %+v", fields)
    }
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./internal/app/bootstrap -run TestStartupConfigLogFieldsIncludeDataDirectoryOrigin`

Expected: FAIL，`startupConfigLogFields` 尚不存在。

- [ ] **Step 3: 实现来源日志**

让 `Run` 把 `parsedConfig.Origins["data_dir"]` 传给 `serve`，并在 logger 初始化后的“InkHub 启动”日志中增加：

```go
zap.String("data_dir", config.DataDir),
zap.String("data_dir_source", dataDirSource),
```

`startupConfigLogFields` 只负责构造这两个稳定字段，便于测试，不承担日志输出。

- [ ] **Step 4: 更新 `.env.example`**

在日志配置之前增加：

```dotenv
# InkHub 数据目录，包含 SQLite、日志、备份和发布暂存文件；必须使用绝对路径
# 示例（请替换用户名）：/Users/your-name/Library/Application Support/InkHub
INKHUB_DATA_DIR=/Users/your-name/Library/Application Support/InkHub
```

- [ ] **Step 5: 更新 README**

将“日志配置”调整为“启动与日志配置”，说明：

- `.env` 支持 `INKHUB_DATA_DIR`。
- 优先级为 `--data-dir` > `INKHUB_DATA_DIR` > 系统默认目录。
- 必须使用绝对路径，不支持 `~`。
- 修改目录不会自动迁移旧数据库。
- 启动日志会记录最终目录及来源。

- [ ] **Step 6: 运行相关测试**

Run: `go test ./internal/app/bootstrap ./internal/platform/logging`

Expected: PASS。

- [ ] **Step 7: 功能点 reflection**

核对 `.env.example`、README、错误文案和实际行为一致；确认日志不含 Token，路径值只在本机日志出现；确认没有引入数据迁移或设置页改动。

---

### Task 3: 整体回归与聚合提交

**Files:**
- Review: `internal/app/bootstrap/bootstrap.go`
- Review: `internal/app/bootstrap/bootstrap_test.go`
- Review: `.env.example`
- Review: `README.md`
- Review: `docs/superpowers/specs/2026-07-30-env-data-dir-design.md`
- Review: `docs/superpowers/plans/2026-07-30-env-data-dir.md`

**Interfaces:**
- Consumes: Task 1 和 Task 2 的全部实现。
- Produces: 已验证的 `.env` 数据目录能力和一个 Conventional Commits 聚合提交。

- [ ] **Step 1: 格式化并检查差异**

Run: `gofmt -w internal/app/bootstrap/bootstrap.go internal/app/bootstrap/bootstrap_test.go`

Run: `git diff --check && git diff --stat && git status --short`

Expected: 无格式错误、无意外生成物、改动范围只覆盖计划文件。

- [ ] **Step 2: 运行完整后端验证**

Run: `go test ./...`

Run: `go test -race ./internal/app/bootstrap ./internal/platform/logging`

Run: `go vet ./...`

Run: `go build ./cmd/inkhub`

Expected: 全部退出码为 0。

- [ ] **Step 3: 最终 reflection**

复查优先级、空值、相对路径、旧数据兼容、日志默认路径、错误恢复和文档一致性。若发现问题，先添加或修正测试，再重复完整验证。

- [ ] **Step 4: 聚合提交**

```bash
git add internal/app/bootstrap/bootstrap.go internal/app/bootstrap/bootstrap_test.go .env.example README.md docs/superpowers/plans/2026-07-30-env-data-dir.md
git commit -m "feat(config): support data directory environment setting"
```
