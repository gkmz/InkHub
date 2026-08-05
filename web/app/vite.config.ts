import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import type { IncomingMessage } from "node:http";

const demoArticles = [
  { id: "a1", title: "构建可靠的本地内容工作流", directory: "writing/engineering", category: "工程实践", modified_at: "2026-07-14T09:42:00Z", state: "blocked", content_stage: "ready", next_action: "retry", hugo_state: "构建失败", wechat_state: "尚未准备", content_version: "demo-a1-v1" },
  { id: "a2", title: "从笔记到发布：我的写作系统", directory: "writing/product", category: "内容创作", modified_at: "2026-07-14T08:16:00Z", state: "changed", content_stage: "ready", next_action: "review", hugo_state: "需要同步", wechat_state: "草稿可能过期", content_version: "demo-a2-v1" },
  { id: "a3", title: "SQLite 在桌面应用中的取舍", directory: "notes/database", category: "技术笔记", modified_at: "2026-07-13T14:30:00Z", state: "draft", content_stage: "draft", content_stage_issue: "", hugo_state: "未进入发布流程", wechat_state: "未进入发布流程", content_version: "demo-a3-v1" },
  { id: "a4", title: "公众号排版模板设计记录", directory: "writing/design", category: "设计", modified_at: "2026-07-12T11:05:00Z", state: "pending_review", content_stage: "ready", next_action: "review", hugo_state: "尚未同步", wechat_state: "尚未准备", content_version: "demo-a4-v1" },
  { id: "a5", title: "内容哈希与幂等发布", directory: "notes/architecture", category: "工程实践", modified_at: "2026-07-10T16:20:00Z", state: "approved", content_stage: "ready", next_action: "view", hugo_state: "已同步", wechat_state: "已确认草稿", content_version: "demo-a5-v1" },
];

const demoArticle = {
  id: "a4", content_version: "demo-current-version", hugo_provider_id: "demo-hugo", wechat_provider_id: "demo-wechat", relative_path: "writing/design/wechat-template.md", modified_at: "2026-07-12T11:05:00Z",
  metadata: { title: "公众号排版模板设计记录", description: "从安全约束、模板变量和复制流程出发，记录一套可分享的公众号排版方案。", category: "设计", series: "InkHub 构建记录", tags: ["微信", "模板", "前端"], keywords: ["公众号排版", "内容工作流"], slug: "wechat-template-design", cover: "" },
  preview_html: "<h2>为什么需要模板标准</h2><p>排版不是把 CSS 塞进 HTML，而是建立一条可验证、可恢复的内容交付链路。</p><blockquote>模板只负责呈现，不应掌握文章状态。</blockquote><h2>三个明确阶段</h2><p>准备内容、复制格式化内容、人工确认草稿，三个动作必须彼此独立。</p><pre><code>Obsidian → 审核 → 微信预览</code></pre>",
  source_changed: false, review_state: "等待审核", hugo_state: "尚未同步", wechat_state: "尚未准备",
  checks: [{ id: "c1", level: "recommended", title: "Description 可以更具体", detail: "补充读者能获得的结果。", channel: "Hugo · 微信" }, { id: "c2", level: "passed", title: "Slug 格式正确", detail: "可以用于 Hugo 页面路径。", channel: "Hugo" }],
  ai_configured: true, suggestions: [
    { id: "demo-description", field: "description", name: "拆解安全模板、CSS 内联与人工确认，构建可靠的公众号内容交付流程。", value: "拆解安全模板、CSS 内联与人工确认，构建可靠的公众号内容交付流程。", reason: "突出文章覆盖的工程环节", new_term: false, usage_count: 0 },
    { id: "demo-category", field: "category", name: "工程实践", value: "工程实践", reason: "文章聚焦模板和内容交付的实现方法。", new_term: false, usage_count: 8 },
    { id: "demo-series", field: "series", name: "InkHub 构建记录", value: "InkHub 构建记录", reason: "与产品构建过程主题一致。", new_term: false, usage_count: 4 },
    { id: "demo-keywords", field: "keywords", name: "公众号排版、内容工作流", value: ["公众号排版", "内容工作流"], reason: "覆盖文章的核心搜索词。", new_term: false, usage_count: 0 },
    { id: "demo-tag-template", field: "tags", name: "模板", value: "模板", reason: "文章讨论模板标准。", new_term: false, usage_count: 6 },
    { id: "demo-tag-css", field: "tags", name: "CSS 内联", value: "CSS 内联", reason: "正文包含 CSS 内联处理。", new_term: true, usage_count: 0 },
  ], suggestions_stale: false, wechat_copied: false,
};

let demoWeChatPrepared = false;
const demoIgnored = new Set<string>();
const demoPublished = new Map<string, Set<string>>();
const demoSuggestionStates = new Map<string, "accepted" | "ignored">();

function resetDemoState() {
  demoWeChatPrepared = false;
  demoIgnored.clear();
  demoPublished.clear();
  demoSuggestionStates.clear();
}

function demoSuggestionViews(includeHandled = false) {
  return demoArticle.suggestions.filter((suggestion) => includeHandled || !demoSuggestionStates.has(suggestion.id)).map((suggestion) => {
    const status = demoSuggestionStates.get(suggestion.id) ?? "pending";
    return { ...suggestion, status, accepted: status === "accepted", ignored: status === "ignored" };
  });
}

function demoSummary(article: typeof demoArticles[number]) {
  if (demoIgnored.has(article.id)) return { ...article, disposition: "ignored" as const };
  if (demoPublished.has(article.id)) return { ...article, disposition: "published" as const };
  return { ...article, disposition: undefined };
}

function readJSONBody(request: IncomingMessage) {
  return new Promise<Record<string, unknown>>((resolve, reject) => {
    let raw = "";
    request.setEncoding("utf8");
    request.on("data", (chunk) => { raw += chunk; });
    request.on("end", () => {
      try { resolve(raw ? JSON.parse(raw) as Record<string, unknown> : {}); } catch (error) { reject(error); }
    });
    request.on("error", reject);
  });
}

// 开发服务器只提供可重复的页面验收数据；生产构建不会包含这段中间件。
const demoAPI: Plugin = {
  name: "inkhub-demo-api",
  configureServer(server) {
    server.middlewares.use("/api/v1", async (request, response) => {
      const url = new URL(request.url ?? "/", "http://localhost");
      const setupMode = request.headers.referer?.includes("demo=setup") ?? false;
      let body: unknown;
      if (url.pathname === "/demo/reset" && request.method === "POST") { resetDemoState(); body = { state: "reset" }; }
      else if (url.pathname === "/session") body = { has_workspace: !setupMode, workspace: setupMode ? null : { id: "demo", name: "我的写作空间" } };
      else if (url.pathname === "/dashboard") {
        const visible = demoArticles.filter((article) => !demoIgnored.has(article.id)).map(demoSummary);
        const ready = visible.filter((article) => article.content_stage === "ready");
        const failed = ready.filter((article) => article.state === "blocked");
        const changed = ready.filter((article) => article.state === "changed");
        const needsReview = ready.filter((article) => article.state === "incomplete" || article.state === "pending_review");
        const readyToPublish = ready.filter((article) => article.state === "approved" && article.id !== "a5" && article.disposition !== "published");
        const latestReady = ready.filter((article) => article.id === "a5").slice(0, 10);
        const recentlyHandled = visible.filter((article) => article.disposition === "published");
        body = {
          failed,
          changed,
          needs_review: needsReview,
          ready_to_publish: readyToPublish,
          latest_ready: latestReady,
          recently_handled: recentlyHandled,
        };
      }
      else if (url.pathname === "/articles") {
        const query = (url.searchParams.get("q") ?? "").toLowerCase();
        const state = url.searchParams.get("state") ?? "";
        const disposition = url.searchParams.get("disposition") ?? "";
        const stage = url.searchParams.get("stage") ?? "";
        const items = demoArticles.filter((article) => {
          const ignored = demoIgnored.has(article.id);
          const published = demoPublished.has(article.id);
          const dispositionMatches = disposition === "ignored" ? ignored : disposition === "published" ? published : disposition === "unresolved" ? !ignored && !published : !ignored;
          return dispositionMatches && (!query || article.title.toLowerCase().includes(query)) && (!state || article.state === state) && (!stage || article.content_stage === stage);
        }).map(demoSummary);
        body = { items, available_channels: ["hugo", "wechat"] };
      } else if (url.pathname === "/articles/batch-disposition" && request.method === "POST") {
        const command = await readJSONBody(request);
        const operation = String(command.operation ?? "");
        const articles = Array.isArray(command.articles) ? command.articles as Array<{ id?: string }> : [];
        const channels = Array.isArray(command.channels) ? command.channels.map(String) : [];
        let changed = 0;
        for (const article of articles) {
          const id = String(article.id ?? "");
          if (operation === "ignored") {
            if (!demoIgnored.has(id)) changed += 1;
            demoIgnored.add(id);
            demoPublished.delete(id);
          } else if (operation === "restore") {
            if (demoIgnored.delete(id)) changed += 1;
          } else if (operation === "published") {
            const previous = demoPublished.get(id);
            if (!previous || channels.some((channel) => !previous.has(channel)) || previous.size !== channels.length) changed += 1;
            demoIgnored.delete(id);
            demoPublished.set(id, new Set(channels));
          }
        }
        body = { processed: articles.length, changed, unchanged: articles.length - changed };
      } else if (url.pathname === "/workspaces") body = { workspace: { id: "demo", name: "我的写作空间" }, job_id: "demo-scan" };
      else if (/^\/articles\/[^/]+\/suggestions$/.test(url.pathname) && request.method === "POST") { demoSuggestionStates.clear(); body = { suggestions: demoSuggestionViews(), suggestions_id: "demo-suggestion-current", suggestions_generated_at: "2026-08-02T10:00:00Z", suggestions_stale: false }; }
      else if (/^\/articles\/[^/]+\/suggestions\/[^/]+\/actions$/.test(url.pathname) && request.method === "POST") {
        const command = await readJSONBody(request);
        const action = String(command.action ?? "") as "accepted" | "ignored";
        const itemIDs = Array.isArray(command.item_ids) ? command.item_ids.map(String) : [];
        itemIDs.forEach((id) => demoSuggestionStates.set(id, action));
        body = { id: "demo-suggestion-current", generated_at: "2026-08-02T10:00:00Z", model: "demo-model", input_content_hash: "demo-current-version", state: "partially_accepted", suggestions: demoSuggestionViews(true), suggestions_stale: false };
      }
      else if (/^\/articles\/[^/]+\/suggestions$/.test(url.pathname)) body = { items: [{ id: "demo-suggestion-current", generated_at: "2026-08-02T10:00:00Z", model: "demo-model", input_content_hash: "demo-current-version", state: "pending", suggestion_count: demoArticle.suggestions.length, current: true }] };
      else if (/^\/articles\/[^/]+\/suggestions\/[^/]+$/.test(url.pathname)) body = { id: "demo-suggestion-current", generated_at: "2026-08-02T10:00:00Z", model: "demo-model", input_content_hash: "demo-current-version", state: "pending", suggestions: demoSuggestionViews(true), suggestions_stale: false };
      else if (/^\/articles\/[^/]+$/.test(url.pathname)) {
        const articleID = url.pathname.split("/").pop() ?? demoArticle.id;
        const summary = demoArticles.find((article) => article.id === articleID);
        const channels = [...(demoPublished.get(articleID) ?? [])];
        const disposition = demoIgnored.has(articleID) ? { kind: "ignored", channels: [] } : channels.length > 0 ? { kind: "published", channels } : undefined;
        body = { ...demoArticle, suggestions: demoSuggestionViews(), id: articleID, content_version: summary?.content_version ?? demoArticle.content_version, content_stage: summary?.content_stage ?? "draft", content_stage_issue: summary?.content_stage_issue ?? "", disposition, review_state: summary?.content_stage === "draft" ? "不适用" : articleID === "a2" ? "已通过" : demoArticle.review_state, hugo_state: summary?.content_stage === "draft" ? "未进入发布流程" : articleID === "a2" ? "需要同步" : demoArticle.hugo_state, wechat_state: summary?.content_stage === "draft" ? "未进入发布流程" : demoWeChatPrepared ? "已准备" : demoArticle.wechat_state };
      }
      else if (/^\/articles\/[^/]+\/metadata$/.test(url.pathname)) body = demoArticle;
      else if (/^\/articles\/[^/]+\/review$/.test(url.pathname)) body = { state: "approved" };
      else if (/^\/articles\/[^/]+\/publication-workflow$/.test(url.pathname)) {
        const articleID = url.pathname.split("/")[2];
        body = articleID === "a2" ? { article_id: articleID, hugo: { state: "ready", progress: 100, stage: "预览已准备", preview: { preview_id: "preview-restored", section: "posts", target_path: "content/posts/restored-writing-system", change: "updated", files: [{ relative_path: "index.md", media_type: "text/markdown", size: 1674 }], diagnostics: [{ code: "hugo.build_ready", level: "passed", message: "Hugo staging 构建通过" }], state: "ready" } } } : { article_id: articleID, hugo: null };
      }
      else if (/^\/articles\/[^/]+\/publication-history$/.test(url.pathname)) body = { items: [{ id: "history-hugo", channel: "hugo", state: "published", title: "已同步到 Hugo", detail: "文章内容已写入 Hugo 发布目录", occurred_at: "2026-07-16T09:30:00Z" }, { id: "history-wechat", channel: "wechat", state: "confirmed", title: "已确认保存微信草稿", detail: "已由用户确认保存到微信草稿箱", occurred_at: "2026-07-15T11:20:00Z" }] };
      else if (/^\/articles\/[^/]+\/wechat-plans$/.test(url.pathname)) { const command = await readJSONBody(request); body = { plan_token: "demo-opaque-plan", template_id: "default", mermaid_theme: command.mermaid_theme === "modern" ? "modern" : "handdrawn", ready: true, expires_at: "2026-07-18T12:00:00Z", diagnostics: [], images: [{ reference: "images/cover.png", media_type: "image/png", size: 1842, state: "upload" }] }; }
      else if (/^\/articles\/[^/]+\/wechat-plans\/confirm$/.test(url.pathname)) { demoWeChatPrepared = true; body = { state: "queued" }; }
      else if (/^\/articles\/[^/]+\/hugo-sections$/.test(url.pathname)) body = { sections: [{ name: "notes", article_count: 12, directories: [] }, { name: "posts", article_count: 28, directories: [{ path: "ai", article_count: 18 }, { path: "tools", article_count: 10 }] }], existing_section: "", existing_directory: "", selection_locked: false };
      else if (/^\/articles\/[^/]+\/hugo-previews$/.test(url.pathname)) body = { id: "preview-demo", job_id: "preview-demo", state: "queued" };
      else if (url.pathname === "/hugo-previews/preview-demo") body = { id: "preview-demo", content_hash: "demo-current-version", section: "posts", target_path: "content/posts/ai/wechat-template-design", change: "added", files: [{ relative_path: "index.md", media_type: "text/markdown", size: 1842 }], diagnostics: [{ code: "hugo.build_ready", level: "passed", message: "Hugo staging 构建通过" }], state: "ready", job_id: "preview-demo" };
      else if (url.pathname === "/hugo-previews/preview-demo/confirm") body = { job_id: "delivery-demo", state: "queued" };
      else if (url.pathname === "/publications") body = { job_id: "demo-publication" };
      else if (url.pathname === "/wechat/confirm") body = { state: "confirmed" };
      else if (url.pathname === "/wechat/copied") body = { state: "copied" };
      else if (url.pathname.startsWith("/wechat/content/")) body = { html: demoArticle.preview_html };
      else if (url.pathname === "/taxonomy") body = { source: "data/taxonomy.yaml", provider_id: "demo-hugo", provider_type: "hugo", state: "ready", revision: "demo-revision", loaded_at: "刚刚", readonly: false, terms: [{ kind: "category", key: "engineering", name: "工程实践", usage_count: 8, metadata: {} }, { kind: "tag", key: "local-first", name: "local-first", usage_count: 2, metadata: {} }], issues: [] };
      else if (/^\/taxonomy\/issues\/[^/]+\/approve$/.test(url.pathname)) body = { state: "approved" };
      else if (url.pathname === "/settings") body = { workspace_name: "我的写作空间", vault_path: "/Users/me/Documents/Writing", content_roots: ["Areas"], ignored_folders: ["Areas/私人记录"], ignored_file_names: ["index.md", "_index.md"], directories: [{ path: "Areas", markdown_count: 42 }, { path: "Areas/私人记录", markdown_count: 8 }], ai_enabled: true, ai_secret_saved: true, hugo_enabled: true, wechat_enabled: true, wechat_secret_saved: false, default_template: "default", templates: [{ id: "default", name: "InkHub 墨绿", version: "1.0.0", compatible: true }], diagnostics: [{ name: "Obsidian Vault", state: "正常", message: "路径可读，已建立索引" }, { name: "Hugo CLI", state: "正常", message: "0.163.0 extended" }, { name: "图片托管", state: "未启用", message: "含本地图片的微信内容将无法准备" }] };
      else if (url.pathname === "/settings/content-scope/preview") body = { added: 3, removed: 1 };
      else if (url.pathname === "/settings/content-scope") body = { indexed: 42, failed: 1 };
      else if (url.pathname === "/directories/inspect") body = { directories: [{ path: "Areas", markdown_count: 42 }, { path: "Areas/私人记录", markdown_count: 8 }] };
      else if (url.pathname === "/directories/pick") body = { path: "/Users/you/Documents/Vault" };
      else if (url.pathname.startsWith("/jobs/")) body = { id: url.pathname.split("/").pop(), state: "succeeded", progress: 100, indexed: 42, failed: 1 };
      else { response.statusCode = 404; body = { error: { code: "route.not_found", message: "接口不存在" } }; }
      response.setHeader("Content-Type", "application/json; charset=utf-8");
      response.end(JSON.stringify(body));
    });
  },
};

export default defineConfig({
  plugins: [react(), demoAPI],
  build: {
    outDir: "../dist",
    emptyOutDir: true,
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    css: true,
    exclude: ["e2e/**", "node_modules/**"],
  },
});
