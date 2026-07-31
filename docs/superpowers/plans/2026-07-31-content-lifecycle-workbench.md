# Content Lifecycle and Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Make publish.status: ready the explicit gate into review and publishing, classify all articles in the library by content stage, and turn the workbench into a mutually exclusive action queue.

**Architecture:** Parse the author-controlled content stage from Markdown into the rebuildable article index. Keep content stage, editorial review, and per-channel publication state independent; derive list and dashboard DTOs on the backend from content hashes and channel policies. Add server-side readiness guards to review, Hugo preview, publication, and WeChat preparation, then update the React library, article detail, and workbench to render backend decisions.

**Tech Stack:** Go 1.24, SQLite migrations and repositories, React 19, TypeScript 5.7, Vitest, Testing Library, Playwright, existing yaml.v3 and lucide-react dependencies.

## Global Constraints

- Missing or invalid publish.status is always treated as draft; only exact publish.status: ready enters the review workflow.
- Display labels use “草稿” and “已就绪”; “可发布” means the current ready version has passed review and blocking checks.
- File modification time is only a scan hint and sort key; content hashes decide review and channel invalidation.
- Hugo publication is version-bound; confirmed WeChat delivery remains delivered after later source changes and is never automatically requeued.
- Every new public Go method needs a Chinese documentation comment; complex logic needs a succinct Chinese comment.
- Every migration table and field needs a database-visible comment through schema_comments.
- Preserve pre-existing web/app/e2e, Vite, and web/dist changes; do not reset or stage unrelated files.
- Do not add dependencies.

---

### Task 1: Content-stage domain and Obsidian parser

**Files:** Create internal/domain/article/lifecycle.go and lifecycle_test.go. Modify internal/domain/article/article.go, internal/provider/source/obsidian/frontmatter.go, provider_test.go, internal/provider/contracts/source.go, and internal/app/workspace/scan.go plus tests.

**Interfaces:**

- article.ContentStage exposes ContentStageDraft and ContentStageReady.
- article.ResolveContentStage(value string, present bool, scalar bool) (ContentStage, string) resolves missing, draft, ready, empty, unknown, and non-scalar values.
- article.Article gains ContentStage and ContentStageIssue.
- SourceDocument continues using Article for the non-blocking issue.

- [ ] Step 1: Write failing table tests for missing => draft, exact ready => ready, draft => draft, and invalid values => draft with a Chinese issue.
- [ ] Step 2: Run go test ./internal/domain/article ./internal/provider/source/obsidian ./internal/app/workspace and verify failure because the type and resolver do not exist.
- [ ] Step 3: Implement the documented type and resolver. In parseDocument inspect publish.status without turning invalid values into blocking parse errors. Store the resolved stage and issue on Article. Keep publish.status out of HashInput because author intent does not change channel output; the raw source fingerprint still detects the edit.
- [ ] Step 4: Run the same focused tests and verify all pass.
- [ ] Step 5: Commit with:
~~~bash
git add internal/domain/article/lifecycle.go internal/domain/article/lifecycle_test.go internal/domain/article/article.go internal/provider/source/obsidian/frontmatter.go internal/provider/source/obsidian/provider_test.go internal/provider/contracts/source.go internal/app/workspace/scan.go internal/app/workspace/scan_test.go
git commit -m "feat(article): parse explicit content readiness"
~~~

### Task 2: SQLite persistence and migration

**Files:** Create internal/storage/sqlite/migrations/0007_article_content_stage.sql. Modify internal/storage/sqlite/migrate.go, repository/article.go, repository/article_test.go, and sqlite_test.go.

**Interfaces:**

- articles.content_stage TEXT NOT NULL DEFAULT 'draft' CHECK (content_stage IN ('draft','ready')) stores the resolved stage.
- articles.content_stage_issue TEXT NOT NULL DEFAULT '' stores an invalid-value repair hint.
- ArticleRepository.Upsert persists both fields atomically with the existing snapshot.

- [ ] Step 1: Add failing tests for migration version 7, upsert persistence, legacy rows defaulting to draft, and schema comments for both columns.
- [ ] Step 2: Run go test ./internal/storage/sqlite/... and verify failure because migration 7 and columns do not exist.
- [ ] Step 3: Add migration 0007, update table/column comment maps, and update every article INSERT, conflict UPDATE, and SELECT scan in repository/article.go. Keep review invalidation based only on content_hash.
- [ ] Step 4: Run go test ./internal/storage/sqlite/... and verify migration, comment, repository, and compatibility tests pass.
- [ ] Step 5: Commit with:
~~~bash
git add internal/storage/sqlite/migrations/0007_article_content_stage.sql internal/storage/sqlite/migrate.go internal/storage/sqlite/repository/article.go internal/storage/sqlite/repository/article_test.go internal/storage/sqlite/sqlite_test.go
git commit -m "feat(storage): persist article content stage"
~~~

### Task 3: Backend list and workbench state derivation

**Files:** Create internal/app/bootstrap/article_status.go and article_status_test.go. Modify internal/transport/http/router.go, internal/app/bootstrap/article_list_api.go and tests, dashboard_api.go and tests, web/app/src/api/types.ts, and web/app/src/api/client.ts.

**Interfaces:**

- ArticleSummary gains ContentStage, ContentStageIssue, and NextAction.
- ArticleListQuery gains ContentStage; accepted values are empty, draft, and ready.
- DashboardView gains ReadyToPublish and LatestReady.
- Pure derivation helpers accept stage, review state/hash, channel states/hashes, dispositions, and configured channels, then return labels, next action, and one dashboard bucket.

- [ ] Step 1: Write failing pure tests: drafts return state draft, no action, and no bucket; ready failures outrank changed; changed outranks review; confirmed WeChat remains delivered after a hash mismatch; ready rows without higher priority enter the latest-ready fallback.
- [ ] Step 2: Run go test ./internal/app/bootstrap ./internal/transport/http and verify failure.
- [ ] Step 3: Add stage and issue to list/dashboard SQL. Filter library by content_stage and keep ignored filtering. For WeChat preserve confirmed when hashes differ; prepared and copied become outdated on mismatch. Restrict dashboard to ready rows and assign exactly one bucket in this order: failed, changed, needs_review, ready_to_publish, recently_handled, latest_ready. Cap latest_ready at 10 after higher buckets claim rows. Return NextAction only for ready rows.
- [ ] Step 4: Accept stage=draft|ready in the HTTP query, reject unknown values, and update TypeScript types/client without any casts.
- [ ] Step 5: Run go test ./internal/app/bootstrap ./internal/transport/http and cd web/app && npm run typecheck. Update old fixtures to explicitly set content_stage=ready where they modelled reviewable articles.
- [ ] Step 6: Commit with:
~~~bash
git add internal/app/bootstrap/article_status.go internal/app/bootstrap/article_status_test.go internal/app/bootstrap/article_list_api.go internal/app/bootstrap/article_list_api_test.go internal/app/bootstrap/dashboard_api.go internal/app/bootstrap/dashboard_api_test.go internal/transport/http/router.go web/app/src/api/types.ts web/app/src/api/client.ts
git commit -m "feat(api): separate article stage from workflow state"
~~~

### Task 4: Ready-only server gates and WeChat terminal behavior

**Files:** Modify internal/transport/http/router.go and runtime.go, internal/app/bootstrap/runtime_api.go, hugo_preview_api.go, wechat_plan_api.go, internal/app/publication/service.go and tests, hugo_preview.go and tests, wechat_plan.go and tests, and related HTTP tests.

**Interfaces:**

- Add ErrArticleNotReady and map it to HTTP 422, code article.not_ready, message 文章尚未标记为已就绪.
- publication.QueueRequest and publication.PreviewArticle carry ContentStage and reject non-ready articles before enqueue.
- Article detail includes content_stage and content_stage_issue. Draft detail uses review_state 不适用 and both channel states 未进入发布流程.

- [ ] Step 1: Add failing tests for draft review, publication queue, Hugo preview, and WeChat plan returning article.not_ready. Add tests that confirmed WeChat with an old hash remains 已确认草稿 while copied with an old hash becomes outdated.
- [ ] Step 2: Run go test ./internal/app/publication ./internal/app/bootstrap ./internal/transport/http and verify failure.
- [ ] Step 3: Load content_stage in every review/publication lookup. Guard both HTTP/application adapters and pure publication services. In runtime and list projections, confirmed WeChat bypasses hash-outdated mapping; all other states remain version-bound.
- [ ] Step 4: Return stage and issue in article detail and run the focused suites until all pass.
- [ ] Step 5: Commit with:
~~~bash
git add internal/transport/http/router.go internal/transport/http/runtime.go internal/app/bootstrap/runtime_api.go internal/app/bootstrap/hugo_preview_api.go internal/app/bootstrap/wechat_plan_api.go internal/app/publication/service.go internal/app/publication/service_test.go internal/app/publication/hugo_preview.go internal/app/publication/hugo_preview_test.go internal/app/publication/wechat_plan.go internal/app/publication/wechat_plan_test.go internal/transport/http/runtime_test.go internal/transport/http/hugo_preview_test.go internal/transport/http/wechat_plan_test.go
git commit -m "feat(workflow): gate review and publishing by readiness"
~~~

### Task 5: Content library and article detail UI

**Files:** Modify web/app/src/api/types.ts, pages/library/LibraryPage.tsx, components/ArticleRow.tsx, pages/article/ArticlePage.tsx, their tests, and web/app/src/styles/app.css.

**Interfaces:**

- Library requests use stage alongside existing state and disposition filters.
- ArticleSummary content_stage, content_stage_issue, and next_action are rendered from backend decisions.
- ArticleDetail content_stage and content_stage_issue control review/publish affordances.

- [ ] Step 1: Add failing tests for the 内容阶段 select sending stage=draft, draft rows showing 草稿 without review/publish actions, ready details retaining 审核通过, and invalid stage showing its repair warning.
- [ ] Step 2: Run cd web/app && npm test -- --run src/pages/library/library-page.test.tsx src/pages/article/article-workflow.test.tsx and verify failure.
- [ ] Step 3: Add 内容阶段 options 全部, 已就绪, 草稿, pass stage through list requests, render stage and issue on rows, and suppress review/publish row actions for drafts while preserving comprehensive-library selection and disposition actions.
- [ ] Step 4: Add a draft badge and frontmatter guidance near the article toolbar. For drafts omit the primary review/publish button but keep preview, metadata, taxonomy, and history. Preserve the existing ready action sequence.
- [ ] Step 5: Run cd web/app && npm test -- --run src/pages/library/library-page.test.tsx src/pages/article/article-workflow.test.tsx && npm run typecheck && npm run lint.
- [ ] Step 6: Commit with:
~~~bash
git add web/app/src/api/types.ts web/app/src/pages/library/LibraryPage.tsx web/app/src/components/ArticleRow.tsx web/app/src/pages/article/ArticlePage.tsx web/app/src/pages/article/article-workflow.test.tsx web/app/src/pages/library/library-page.test.tsx web/app/src/styles/app.css
git commit -m "feat(ui): organize library by content stage"
~~~

### Task 6: Action-oriented workbench UI

**Files:** Modify web/app/src/api/types.ts, pages/workspace/DashboardPage.tsx, components/ArticleRow.tsx, web/app/src/app.test.tsx, and web/app/src/styles/app.css.

**Interfaces:**

- Sections map to failed, changed, needs_review, ready_to_publish, latest_ready, and recently_handled.
- ArticleSummary.next_action maps to one primary button; the page must not infer a different action from an internal state string.

- [ ] Step 1: Add failing tests that drafts from a stale response are not rendered, ready rows appear in a higher-priority action bucket or latest-ready, IDs are not duplicated across sections, and an empty ready queue explains publish.status: ready and links to the library.
- [ ] Step 2: Run cd web/app && npm test -- --run src/app.test.tsx and verify failure.
- [ ] Step 3: Render the six backend buckets, defensively skip draft rows, cap latest-ready at 10, and use next_action for labels/icons. Keep row navigation separate from the action button.
- [ ] Step 4: Replace the old empty state with ready-state guidance and a content-library link; retain compact section counts and responsive layout.
- [ ] Step 5: Run cd web/app && npm test -- --run src/app.test.tsx && npm run typecheck && npm run lint.
- [ ] Step 6: Commit with:
~~~bash
git add web/app/src/api/types.ts web/app/src/pages/workspace/DashboardPage.tsx web/app/src/components/ArticleRow.tsx web/app/src/app.test.tsx web/app/src/styles/app.css
git commit -m "feat(ui): turn workbench into ready action queue"
~~~

### Task 7: Documentation, demo data, production assets, and regression

**Files:** Modify docs/PRD.md, docs/design/interactions.md, README.md, web/app/vite.config.ts, web/app/e2e/workflows.spec.ts, web/app/e2e/screenshots.spec.ts, and generated web/dist assets as required by the existing build.

- [ ] Step 1: Add Playwright coverage for a missing-status article appearing only in library draft view, ready articles appearing in the workbench, and a changed article with confirmed WeChat not showing a repeated WeChat action.
- [ ] Step 2: Extend the existing demo reset fixture with content_stage, content_stage_issue, and next_action while preserving its current user changes.
- [ ] Step 3: Update PRD, interaction design, and README with the ready frontmatter example, 草稿/已就绪 labels, content-hash rules, library grouping, workbench buckets, and WeChat terminal delivery.
- [ ] Step 4: Run:
~~~bash
go test -race ./...
go vet ./...
go build ./cmd/inkhub
cd web/app && npm test -- --run && npm run typecheck && npm run lint && npm run build && npx playwright test
~~~
Expected: all pass; inspect generated asset names before staging.
- [ ] Step 5: Inspect git diff --check, git status --short, git diff HEAD --stat, and the documentation diff. Confirm unrelated existing changes were not reverted or silently included.
- [ ] Step 6: Commit with:
~~~bash
git add docs/PRD.md docs/design/interactions.md README.md web/app/vite.config.ts web/app/e2e/workflows.spec.ts web/app/e2e/screenshots.spec.ts web/dist/index.html web/dist/assets
git commit -m "docs: document ready content workflow"
~~~

## Final Verification Checklist

- [ ] Missing publish.status indexes as draft and never appears in the workbench.
- [ ] Only exact publish.status: ready enters review and publishing.
- [ ] Invalid status remains visible in the library with a repair warning.
- [ ] Content hashes, not modification times, invalidate review and Hugo delivery.
- [ ] Confirmed WeChat delivery remains delivered after source changes and is not automatically requeued.
- [ ] Content library contains all articles and supports 全部, 已就绪, and 草稿 views.
- [ ] Workbench buckets are backend-derived, mutually exclusive, ready-only, and capped at 10 for latest-ready.
- [ ] Draft article URLs cannot bypass review, Hugo preview, publication, or WeChat preparation guards.
- [ ] Go race tests, vet, build, frontend tests, typecheck, lint, build, and Playwright pass.
