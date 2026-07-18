# 微信 GitHub 图片托管实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为微信公众号人工复制发布流程增加公开 GitHub 图片仓库、上传前图片清单、用户确认和可恢复的安全图片处理。

**Architecture:** Source Provider 继续提供授权后的 `ResourceRefs`；微信图片校验器生成规范媒体信息；通用 `AssetUploader` 负责只读检查与幂等上传；GitHub Infrastructure 通过 Contents API 实现该契约。Application 生成带完整性保护的短期准备计划，确认后才创建 `wechat_prepare`，HTTP 和 React 只展示相对引用与安全状态。

**Tech Stack:** Go、SQLite、系统 Secret Store、GitHub REST Contents API、React、TypeScript、Vitest、Playwright、zap。

## Global Constraints

- 微信发布仍为人工复制粘贴和草稿确认，不调用微信公众号草稿或发布接口。
- 只支持公开 GitHub 仓库，不支持其他图床、私有仓库或自定义 GitHub API Host。
- Token 只保存到系统 Secret Store，不进入 SQLite、HTTP 读响应、日志、fixture 或 Git 历史。
- 用户确认准备计划前不得上传图片或产生外部写入。
- 对外响应不得包含绝对路径、完整摘要、Job ID、数据库 ID、GitHub 原始响应或 Secret。
- 只处理 Source Provider 已授权的 Vault 内本地图片；远程 HTTPS 保持不变，其他 scheme 阻断。
- 支持静态 PNG、JPEG、GIF、WebP；最大 10 MiB、40,000,000 像素；动画和伪造类型阻断。
- 任一图片失败不得生成当前内容版本的可交付 HTML。
- 新增公开 Go 类型和方法使用中文文档注释，关键安全与幂等逻辑使用中文注释。
- 每个行为先确认 RED，再最小实现 GREEN；每个功能点完成后 reflection。
- 功能点之间不提交；全部回归通过后只创建一次实现提交。

---

### Task 1: 图片校验与结构化上传契约

**Files:**
- Create: `internal/provider/publish/wechat/image.go`
- Create: `internal/provider/publish/wechat/image_test.go`
- Modify: `internal/provider/publish/wechat/provider.go`
- Modify: `internal/provider/publish/wechat/provider_test.go`

**Interfaces:**
- Produces: `AssetUploadRequest{LocalPath,Digest,MediaType,Extension}`。
- Produces: `AssetUploadResult{URL,Reused}`。
- Modifies: `AssetUploader.Inspect(context.Context, AssetUploadRequest) (AssetUploadResult, bool, error)` 与 `Upload(context.Context, AssetUploadRequest) (AssetUploadResult, error)`。
- Produces: Provider 内部 `InspectImage(path string) (ImageInfo, error)`。

- [ ] **Step 1: 写图片校验失败测试**

用标准库生成 PNG/JPEG/GIF，增加 WebP fixture；覆盖空文件、扩展伪造、SVG、动画 GIF、超 10 MiB、超 40,000,000 像素、目录和不存在文件。断言稳定错误 `wechat.image_missing` 或 `wechat.image_invalid`，且消息不含绝对路径。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/provider/publish/wechat -run 'TestInspectImage|TestPrepareRejectsInvalidImage'`

Expected: FAIL，原因是图片校验器和结构化请求不存在。

- [ ] **Step 3: 实现图片校验器**

用文件签名确定规范类型和扩展，`image.DecodeConfig` 校验尺寸，解析 GIF 帧数拒绝动画，流式计算 SHA-256。只返回 `.png`、`.jpg`、`.gif`、`.webp`。

- [ ] **Step 4: 改造上传与正文重写**

`uploadResources` 只处理已授权 ResourceRef；同一摘要在单次 Prepare 中只上传一次。重写必须定位 Markdown/Wiki 图片目标，不能使用会误改普通正文的全局子串替换。上传或渲染失败前不得保存 Artifact。

- [ ] **Step 5: 验证并 reflection**

Run:

```bash
go test ./internal/provider/publish/wechat
go test ./internal/provider/source/obsidian ./internal/provider/source/markdown
git diff --check
```

检查签名、动画、重复引用、远程 HTTPS、上传器缺失、失败不写 Artifact 和路径脱敏。

---

### Task 2: GitHub Contents API 图片上传器

**Files:**
- Create: `internal/platform/githubassets/client.go`
- Create: `internal/platform/githubassets/client_test.go`
- Create: `internal/platform/githubassets/path.go`
- Create: `internal/platform/githubassets/path_test.go`

**Interfaces:**
- Consumes: Task 1 的 `wechat.AssetUploadRequest` 与 `wechat.AssetUploadResult`。
- Produces: `githubassets.Config{Owner,Repository,Branch,Prefix,Token}`。
- Produces: `New(config Config, client *http.Client, logger *zap.Logger) (*Uploader, error)`、`Validate`、`Inspect`、`Upload`。

- [ ] **Step 1: 写配置、路径和 Raw URL RED 测试**

覆盖空配置、非法分支、`..`、反斜杠、控制字符、超长 prefix 和 URL 注入；断言路径为 `<prefix>/<digest[:2]>/<digest><extension>`，Raw host 固定为 `raw.githubusercontent.com`。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/platform/githubassets -run 'TestConfig|TestAssetPath|TestRawURL'`

Expected: FAIL，原因是 package 不存在。

- [ ] **Step 3: 实现配置与路径**

完整摘要只接受 64 位小写十六进制；扩展只接受 Task 1 规范值；branch/prefix 拒绝空段、控制字符、反斜杠和 `..`，不静默修正非法输入。

- [ ] **Step 4: 写 GitHub API RED 测试**

使用注入 Transport 的 `httptest.Server` 覆盖公开/私有仓库、分支不存在、权限不足、GET 404 后 PUT、新上传、相同内容复用、内容冲突、并发 422 后重查、rate limit、5xx、取消和响应 body 上限。断言 Token、Base64 和原始错误 body 不进入错误或 zap 日志。

- [ ] **Step 5: 实现 Validate、Inspect 和 Upload**

`Validate` 校验公开仓库、分支和 push 权限；`Inspect` 只读目标并比较 SHA-256；`Upload` 通过 Contents API PUT，422 竞争时重新 Inspect。成功后使用匿名受限 client 有界确认 Raw URL，不跟随到非允许 Host。

- [ ] **Step 6: 验证并 reflection**

Run: `go test ./internal/platform/githubassets -race && git diff --check`

检查 Host 白名单、限流分类、幂等、并发竞争、body 上限、日志脱敏和取消传播。

---

### Task 3: Provider 配置、Secret 与设置诊断

**Files:**
- Modify: `internal/provider/publish/wechat/factory.go`
- Modify: `internal/provider/publish/wechat/factory_test.go`
- Modify: `internal/app/bootstrap/provider_runtime.go`
- Modify: `internal/app/bootstrap/bootstrap.go`
- Create: `internal/transport/http/wechat_settings.go`
- Create: `internal/transport/http/wechat_settings_test.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/transport/http/runtime_scope.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/pages/settings/SettingsPage.tsx`
- Modify: `web/app/src/pages/settings/settings.test.tsx`

**Interfaces:**
- Extends WeChat config with `github_owner`、`github_repository`、`github_branch`、`github_prefix`。
- Uses Secret key `provider/{workspaceID}/{providerID}/github-token` through `contracts.SecretResolver`。
- Extends Settings DTO with `github_token_saved` 和图片仓库诊断。

- [ ] **Step 1: 写 Factory/Secret RED 测试**

断言未配置图床仍能构建无图文章 Provider；配置存在但 Secret 缺失时本地图像 Preflight 阻断；完整配置通过 SecretResolver 构建 uploader；配置和错误不含 Token。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/provider/publish/wechat -run 'TestFactoryBuildsGitHubUploader|TestFactoryAllowsNoImageHosting'`

- [ ] **Step 3: 实现工厂与 Bootstrap 装配**

Factory 按 Provider 实例动态创建 uploader，不能注入全局 GitHub 配置。Runner 执行时解析 Secret；`Validate` 调用 GitHub 诊断。旧配置继续支持，无图文章不因未配置图床失败。

- [ ] **Step 4: 写设置 HTTP/React RED 测试**

覆盖读取只返回 `github_token_saved`、非空 Token 写 Secret Store、空 Token 保留已有值、显式删除、私有仓库和权限错误中文诊断、响应不回显 Token、重新诊断按钮真实绑定。

- [ ] **Step 5: 实现设置界面与反馈**

在现有微信渠道区段增加紧凑 GitHub 字段和诊断，不新增图片管理页。状态只使用“正常、需要处理、未启用”，失败提供可执行建议。

- [ ] **Step 6: 验证并 reflection**

Run:

```bash
go test ./internal/provider/publish/wechat ./internal/app/bootstrap ./internal/transport/http ./internal/platform/secrets
cd web/app
npm test -- --run src/pages/settings/settings.test.tsx
npm run typecheck
npm run lint
```

检查 Secret 生命周期、旧配置、无图文章、诊断按钮、私有仓库文案和脱敏。

---

### Task 4: 文章级微信准备计划与安全确认

**Files:**
- Create: `internal/app/publication/wechat_plan.go`
- Create: `internal/app/publication/wechat_plan_test.go`
- Create: `internal/app/bootstrap/wechat_plan_api.go`
- Modify: `internal/app/publication/service.go`
- Modify: `internal/app/publication/service_test.go`
- Modify: `internal/app/bootstrap/publication_runner.go`

**Interfaces:**
- Produces: `WeChatPlanService.Plan(ctx, articleID, templateID string) (WeChatPlanView, error)`。
- Produces: `WeChatPlanService.Confirm(ctx, articleID, token string) (domainjob.Job, error)`。
- Produces: HMAC-SHA256 opaque token，TTL 10 分钟。

- [ ] **Step 1: 写只读计划 RED 测试**

覆盖无图、本地 upload/reuse、远程 HTTPS、未配置图床、缺失/非法图片和模板选择。记录型 uploader 断言 Plan 从不调用 Upload；序列化视图不得含绝对路径、摘要或内部 ID。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/app/publication -run TestWeChatPlan`

- [ ] **Step 3: 实现 Resolver、规划和 token**

Resolver 只允许最近工作区、请求文章、启用微信 Provider 和兼容模板。token 绑定 workspace/article/provider/content hash/template revision/图片安全摘要/过期时间；使用 constant-time 签名比较。

- [ ] **Step 4: 写确认失效与幂等 RED 测试**

覆盖 token 篡改、过期、跨文章、跨工作区、内容变化、模板变化、图片集合变化；重复确认返回同一 queued/running/succeeded Job；客户端不能提交路径或清单。

- [ ] **Step 5: 实现 Confirm 与 Runner 复用**

Confirm 重新读取 Source 并校验计划，只在 ready 时调用现有确定性 enqueue。Job payload 不保存路径或 Secret；成功写 prepared Event，最终失败沿用 attempt 事件。

- [ ] **Step 6: 验证并 reflection**

Run:

```bash
go test ./internal/app/publication ./internal/app/bootstrap ./internal/app/job ./internal/storage/sqlite/repository
git diff --check
```

检查确认前无写入、token 隔离、旧 hash、重复确认、恢复、失败历史和路径泄露。

---

### Task 5: HTTP API 与微信准备页面

**Files:**
- Create: `internal/transport/http/wechat_plan.go`
- Create: `internal/transport/http/wechat_plan_test.go`
- Modify: `internal/transport/http/runtime.go`
- Modify: `internal/app/bootstrap/bootstrap.go`
- Modify: `web/app/src/api/types.ts`
- Modify: `web/app/src/api/client.ts`
- Create: `web/app/src/pages/wechat-preview/WeChatPlan.tsx`
- Create: `web/app/src/pages/wechat-preview/WeChatPlan.test.tsx`
- Modify: `web/app/src/pages/wechat-preview/WeChatPreviewPage.tsx`
- Modify: `web/app/src/styles/app.css`
- Modify: `web/app/vite.config.ts`
- Modify: `web/app/e2e/workflows.spec.ts`

**Interfaces:**
- Adds: `POST /api/v1/articles/{id}/wechat-plans` body `{template_id}`。
- Adds: `POST /api/v1/articles/{id}/wechat-plans/confirm` body `{plan_token}`。
- Adds frontend `getWeChatPlan` and `confirmWeChatPlan`。

- [ ] **Step 1: 写 HTTP RED 测试**

覆盖安全 DTO、无图、upload/reuse、阻断诊断、非法模板、非法/过期 token、重复确认和请求大小限制。fixture 植入绝对路径、完整摘要、Token 和 GitHub body并断言响应不含这些值。

- [ ] **Step 2: 确认 RED**

Run: `go test ./internal/transport/http -run TestWeChatPlan`

- [ ] **Step 3: 实现路由与安全映射**

逐字段构造 DTO；计划失效统一 `400 request.plan_invalid`；配置或图片阻断用 `200 ready=false`；系统依赖错误使用安全通用响应。

- [ ] **Step 4: 写 React RED 测试**

覆盖模板和图片清单、无图、upload/reuse、阻断禁用、确认 loading/重复点击、计划失效重建、任务轮询、刷新恢复、失败反馈、长路径和移动端。

- [ ] **Step 5: 实现页面状态机**

进入微信页先获取计划，确认后才轮询准备任务，成功再读取 HTML；复制和草稿确认保持独立。所有 timer 和 AbortController 在卸载时清理。

- [ ] **Step 6: 更新 Demo 与 E2E**

Demo 返回 upload/reuse 图片与无图文章；E2E 断言确认前未调用 confirm，确认后进入预览，复制后才显示草稿确认，并覆盖 1440×1000 与 390×844 无横向溢出。

- [ ] **Step 7: 验证并 reflection**

Run:

```bash
go test ./internal/transport/http ./internal/app/bootstrap
cd web/app
npm test -- --run src/pages/wechat-preview/WeChatPlan.test.tsx
npm run typecheck
npm run lint
npx playwright test e2e/workflows.spec.ts --workers=1
```

检查人工发布文案、确认前零写入、反馈、恢复、定时器、长路径、移动端和内部字段泄露。

---

### Task 6: 中文文档、真实渠道门禁与整体回归

**Files:**
- Modify: `docs/design/architecture.md`
- Modify: `docs/design/interactions.md`
- Modify: `docs/design/provider-contracts.md`
- Modify: `docs/design/data-model.md`
- Modify: `.codex/HANDOFF.md`
- Generated: `web/dist/*`

**Interfaces:**
- Validates Task 1-5 as one complete user workflow。
- Produces one Conventional Commit implementation commit。

- [ ] **Step 1: 更新中文文档**

记录 GitHub uploader 契约、公开仓库、Secret Store、准备计划、确认前无写入、幂等路径、格式限制、安全 DTO 和人工复制边界。

- [ ] **Step 2: 整体 reflection**

检查公开/私有仓库、路径与 Host 注入、符号链接、类型伪造、动画、大小与像素、重复资源、竞争、限流、部分失败、旧 hash、模板变化、token 篡改、重复确认、重启恢复、Secret/路径泄露、日志、移动端和微信三阶段事件。

- [ ] **Step 3: 修复 reflection 问题**

行为问题先补 RED 测试再实现 GREEN；文档和注释问题执行格式检查。

- [ ] **Step 4: 自动化 GitHub 协议验收**

使用 `httptest` 完整模拟 GitHub API 与匿名 Raw Host。真实仓库只在用户提供专用公开仓库和最小权限 Token 后测试；缺少凭据时明确列为人工残余风险，不读取未知 Secret 或 `.env`。

- [ ] **Step 5: 浏览器验收**

在 1440×1000 与 390×844 验证图片清单、确认、任务、预览、复制、草稿确认和失败反馈；检查 `scrollWidth === clientWidth` 与 console error/warn。

- [ ] **Step 6: 完整验证**

Run:

```bash
go test ./...
go vet ./...
cd web/app
npm test -- --run
npm run typecheck
npm run lint
npm run build
npx playwright test e2e/workflows.spec.ts --workers=1
cd ../..
git diff --check
git status --short
```

- [ ] **Step 7: 聚合提交**

排除 test-results、trace、临时数据库、Token 和真实仓库 fixture，只暂存计划涉及文件与 `web/dist`：

```bash
git commit -m "feat(wechat): upload github images"
```
