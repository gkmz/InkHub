import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const demoArticles = [
  { id: "a1", title: "构建可靠的本地内容工作流", directory: "writing/engineering", category: "工程实践", modified_at: "2026-07-14T09:42:00Z", state: "blocked", hugo_state: "构建失败", wechat_state: "尚未准备" },
  { id: "a2", title: "从笔记到发布：我的写作系统", directory: "writing/product", category: "内容创作", modified_at: "2026-07-14T08:16:00Z", state: "changed", hugo_state: "需要同步", wechat_state: "草稿可能过期" },
  { id: "a3", title: "SQLite 在桌面应用中的取舍", directory: "notes/database", category: "技术笔记", modified_at: "2026-07-13T14:30:00Z", state: "incomplete", hugo_state: "尚未同步", wechat_state: "尚未准备" },
  { id: "a4", title: "公众号排版模板设计记录", directory: "writing/design", category: "设计", modified_at: "2026-07-12T11:05:00Z", state: "pending_review", hugo_state: "尚未同步", wechat_state: "尚未准备" },
  { id: "a5", title: "内容哈希与幂等发布", directory: "notes/architecture", category: "工程实践", modified_at: "2026-07-10T16:20:00Z", state: "approved", hugo_state: "已同步", wechat_state: "已确认草稿" },
];

// 开发服务器只提供可重复的页面验收数据；生产构建不会包含这段中间件。
const demoAPI = {
  name: "inkhub-demo-api",
  configureServer(server: { middlewares: { use: (path: string, handler: (request: { url?: string; headers: { referer?: string } }, response: { statusCode: number; setHeader: (name: string, value: string) => void; end: (body: string) => void }) => void) => void } }) {
    server.middlewares.use("/api/v1", (request, response) => {
      const url = new URL(request.url ?? "/", "http://localhost");
      const setupMode = request.headers.referer?.includes("demo=setup") ?? false;
      let body: unknown;
      if (url.pathname === "/session") body = { has_workspace: !setupMode, workspace: setupMode ? null : { id: "demo", name: "我的写作空间" } };
      else if (url.pathname === "/dashboard") body = { items: demoArticles };
      else if (url.pathname === "/articles") {
        const query = (url.searchParams.get("q") ?? "").toLowerCase();
        const state = url.searchParams.get("state") ?? "";
        body = { items: demoArticles.filter((item) => (!query || item.title.toLowerCase().includes(query)) && (!state || item.state === state)) };
      } else if (url.pathname === "/workspaces") body = { workspace: { id: "demo", name: "我的写作空间" }, job_id: "demo-scan" };
      else if (url.pathname === "/directories/pick") body = { path: "/Users/you/Documents/Vault" };
      else if (url.pathname.startsWith("/jobs/")) body = { id: "demo-scan", state: "succeeded", progress: 100, indexed: 42, failed: 1 };
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
  },
});
