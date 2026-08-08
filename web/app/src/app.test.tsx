import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { App } from "./app";

const articles = [
  { id: "a2", title: "内容已更新", filename: "02-内容已更新.md", directory: "notes", category: "工程", modified_at: "2026-07-14T09:00:00Z", state: "changed", hugo_state: "需要同步", wechat_state: "尚未准备" },
  { id: "a1", title: "发布失败", filename: "01-发布失败.md", directory: "drafts", category: "产品", modified_at: "2026-07-14T10:00:00Z", state: "blocked", hugo_state: "处理失败", wechat_state: "尚未准备" },
];

function mockAPI(hasWorkspace = true) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: hasWorkspace, workspace: hasWorkspace ? { id: "w1", name: "我的文章" } : null });
    if (url.includes("/directories/inspect-hugo")) return Response.json({ root: "/Users/me/Blog", content_dir: "articles", sections: [{ name: "posts", markdown_count: 8 }] });
    if (url.includes("/directories/inspect")) return Response.json({ directories: [{ path: "Areas", markdown_count: 12 }, { path: "Areas/私人记录", markdown_count: 3 }] });
    if (url.includes("/dashboard")) return Response.json({ failed: [articles[1]], changed: [articles[0]], needs_review: [{ ...articles[0], id: "a3", title: "等待审核", state: "pending_review" }], ready_to_publish: [], latest_ready: [], recently_handled: [{ ...articles[0], id: "a4", title: "已发表文章", state: "approved", disposition: "published" }] });
    if (url.includes("/articles")) return Response.json({ items: articles, next_cursor: "" });
    return Response.json({ error: { code: "not_found", message: "接口不存在" } }, { status: 404 });
  });
}

test("首次运行按 Vault、管理范围、Hugo 和初始化四步完成引导", async () => {
  mockAPI(false);
  render(<App />);
  expect(await screen.findByRole("heading", { name: "选择 Vault" })).toBeInTheDocument();
  expect(screen.getByText("1 / 4")).toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("工作区名称"), "写作空间");
  await userEvent.type(screen.getByLabelText("Obsidian Vault 路径"), "/Users/me/Vault");
  await userEvent.click(screen.getByRole("button", { name: "验证并读取目录" }));
  await userEvent.click(screen.getByRole("button", { name: "继续" }));
  expect(screen.getByRole("heading", { name: "管理范围" })).toBeInTheDocument();
  await userEvent.click(await screen.findByRole("checkbox", { name: "Areas（12 篇）" }));
  await userEvent.click(screen.getByRole("button", { name: "继续" }));
  expect(screen.getByRole("heading", { name: "连接 Hugo" })).toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("Hugo 根目录"), "/Users/me/Blog");
  await userEvent.click(screen.getByRole("button", { name: "检测 Hugo 站点" }));
  expect(await screen.findByText("内容目录：articles · 1 个 Section")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "继续" }));
  expect(screen.getByRole("heading", { name: "初始化" })).toBeInTheDocument();
  expect(screen.getByText(/缺少 Stable ID 的文章会补充/)).toBeInTheDocument();
});

test("初始化时可以通过原生选择器验证 Obsidian Vault", async () => {
  sessionStorage.clear();
  mockAPI(false);
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: false, workspace: null });
    if (url.includes("/directories/pick")) {
      expect(init?.body).toBe(JSON.stringify({ purpose: "vault" }));
      return Response.json({ path: "/Users/me/Vault" });
    }
    if (url.includes("/directories/inspect")) return Response.json({ directories: [{ path: "Areas", markdown_count: 12 }] });
    return Response.json({ error: { code: "not_found", message: "接口不存在" } }, { status: 404 });
  });
  render(<App />);
  await userEvent.type(await screen.findByLabelText("工作区名称"), "写作空间");
  await userEvent.click(screen.getByRole("button", { name: "选择目录" }));
  expect(await screen.findByLabelText("Obsidian Vault 路径")).toHaveValue("/Users/me/Vault");
  await userEvent.click(screen.getByRole("button", { name: "继续" }));
  expect(screen.getByRole("heading", { name: "管理范围" })).toBeInTheDocument();
});

test("应用重启后仍从会话恢复未完成的初始化门禁", async () => {
  sessionStorage.clear();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: true, workspace: { id: "w1", name: "待初始化" }, initialization: { required: true, job_id: "job-init", state: "failed" } });
    if (url.includes("/jobs/job-init")) return Response.json({ id: "job-init", state: "failed", progress: 5, indexed: 0, assigned_ids: 0, failed: 1, error_message: "有 1 篇文章无法完成初始化", issues: [{ article_path: "Areas/冲突.md", code: "obsidian.duplicate_stable_id", message: "稳定文章 ID 重复" }] });
    return Response.json({ error: { code: "not_found", message: "接口不存在" } }, { status: 404 });
  });
  render(<App />);
  expect(await screen.findByRole("heading", { name: "初始化未能完成" })).toBeInTheDocument();
  expect(screen.getByText("Areas/冲突.md")).toBeInTheDocument();
  expect(screen.queryByRole("link", { name: "工作台" })).not.toBeInTheDocument();
});

test("工作台渲染后端给出的四个互斥区段", async () => {
  mockAPI();
  render(<App />);
  expect(await screen.findByRole("heading", { name: "处理失败" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "内容已更新", level: 2 })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "需要审核", level: 2 })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "最近处理", level: 2 })).toBeInTheDocument();
  expect(screen.getByText("01-发布失败.md")).toBeInTheDocument();
  expect(screen.getByText(/drafts · 产品/)).toBeInTheDocument();
  expect(screen.getAllByTestId("dashboard-row")).toHaveLength(4);
  expect(screen.getByText("已发表")).toBeInTheDocument();
});

test("内容库搜索在输入法组合结束后才请求", async () => {
  mockAPI();
  render(<App />);
  await screen.findByText("发布失败");
  await userEvent.click(screen.getAllByRole("link", { name: "内容库" })[0]);
  const search = await screen.findByRole("searchbox", { name: "搜索文章" });
  const fetchMock = vi.mocked(fetch);
  const before = fetchMock.mock.calls.length;
  fireEvent.compositionStart(search);
  fireEvent.change(search, { target: { value: "产品" } });
  await new Promise((resolve) => setTimeout(resolve, 350));
  expect(fetchMock.mock.calls).toHaveLength(before);
  fireEvent.compositionEnd(search);
  await waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(before));
});

test("移动端导航和键盘焦点保留明确的可访问名称", async () => {
  mockAPI();
  render(<App />);
  expect((await screen.findAllByRole("navigation", { name: "主导航" })).length).toBe(2);
  expect(screen.getAllByRole("link", { name: "工作台" })[0]).toHaveAttribute("href", "/");
  await userEvent.click(screen.getAllByRole("link", { name: "内容库" })[0]);
  expect(screen.getByRole("button", { name: "筛选" })).toBeInTheDocument();
});

test("空工作台提供进入内容库的明确动作", async () => {
  mockAPI();
  vi.mocked(fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: true, workspace: { id: "w1", name: "我的文章" } });
    if (url.includes("/dashboard")) return Response.json({ failed: [], changed: [], needs_review: [], ready_to_publish: [], latest_ready: [], recently_handled: [] });
    return Response.json({ items: [], available_channels: [] });
  });
  render(<App />);
  expect(await screen.findByRole("heading", { name: "还没有已就绪的文章" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "浏览内容库" }));
  expect(await screen.findByRole("heading", { name: "内容库" })).toBeInTheDocument();
});

test("工作台只有最近处理时仍展示内容", async () => {
  mockAPI();
  vi.mocked(fetch).mockImplementation(async (input) => String(input).includes("/session")
    ? Response.json({ has_workspace: true, workspace: { id: "w1", name: "我的文章" } })
    : Response.json({ failed: [], changed: [], needs_review: [], ready_to_publish: [], latest_ready: [], recently_handled: [{ ...articles[0], state: "approved", disposition: "published" }] }));
  render(<App />);
  expect(await screen.findByRole("heading", { name: "最近处理" })).toBeInTheDocument();
  expect(screen.queryByRole("heading", { name: "目前没有需要处理的文章" })).not.toBeInTheDocument();
});

test("状态筛选写入请求且可一键清除", async () => {
  mockAPI();
  render(<App />);
  await screen.findByText("发布失败");
  await userEvent.click(screen.getAllByRole("link", { name: "内容库" })[0]);
  await userEvent.selectOptions(await screen.findByLabelText("审核状态"), "blocked");
  await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes("state=blocked"))).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "清除筛选" }));
  expect(screen.getByLabelText("审核状态")).toHaveValue("");
});

test("初始化表单有内容时阻止浏览器意外离开", async () => {
  mockAPI(false);
  render(<App />);
  await userEvent.type(await screen.findByLabelText("工作区名称"), "未完成设置");
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
});

test("内容库可以按 Cursor 追加文章直到末页", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: true, workspace: { id: "w1", name: "我的文章" } });
    if (url.includes("/dashboard")) return Response.json({ failed: [], changed: [], needs_review: [], ready_to_publish: [], latest_ready: [], recently_handled: [] });
    if (url.includes("cursor=page-2")) return Response.json({ items: [{ ...articles[0], id: "a3", title: "第三页文章" }] });
    if (url.includes("/articles")) return Response.json({ items: articles, next_cursor: "page-2" });
    return Response.json({ error: { message: "接口不存在" } }, { status: 404 });
  });
  render(<App />);
  await screen.findByRole("heading", { name: "还没有已就绪的文章" });
  await userEvent.click(screen.getAllByRole("link", { name: "内容库" })[0]);

  expect(await screen.findByText("发布失败")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "加载更多" }));

  expect(await screen.findByText("第三页文章")).toBeInTheDocument();
  expect(screen.getByText("发布失败")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "加载更多" })).not.toBeInTheDocument();
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes("cursor=page-2"))).toBe(true);
});

test("内容库下一页失败时保留现有文章并提示", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: true, workspace: { id: "w1", name: "我的文章" } });
    if (url.includes("/dashboard")) return Response.json({ failed: [], changed: [], needs_review: [], ready_to_publish: [], latest_ready: [], recently_handled: [] });
    if (url.includes("cursor=page-2")) throw new Error("无法读取下一页");
    if (url.includes("/articles")) return Response.json({ items: articles, next_cursor: "page-2" });
    return Response.json({ error: { message: "接口不存在" } }, { status: 404 });
  });
  render(<App />);
  await screen.findByRole("heading", { name: "还没有已就绪的文章" });
  await userEvent.click(screen.getAllByRole("link", { name: "内容库" })[0]);
  expect(await screen.findByText("发布失败")).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "加载更多" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("无法读取下一页");
  expect(screen.getByText("发布失败")).toBeInTheDocument();
});

test("Hugo 渠道 URL 保留统一应用框架", async () => {
  window.history.replaceState({}, "", "/articles/a1/hugo");
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: true, workspace: { id: "w1", name: "我的文章" } });
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: null });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 2 }], existing_section: "", selection_locked: false });
    if (url.includes("/articles/a1/publication-history")) return Response.json({ items: [] });
    if (url.endsWith("/articles/a1")) return Response.json({
      id: "a1", content_version: "hash", content_stage: "ready", hugo_provider_id: "h1", wechat_provider_id: "w1",
      relative_path: "Areas/demo.md", modified_at: "2026-08-03", metadata: { title: "渠道路由测试", description: "", category: "", series: "", tags: [], keywords: [], slug: "demo", cover: "" },
      preview_html: "<p>正文</p>", source_changed: false, review_state: "已通过", hugo_state: "需要同步", wechat_state: "尚未准备", xiaohongshu_state: "尚未准备",
      checks: [], ai_configured: false, suggestions: [], suggestions_stale: false, wechat_copied: false, resource_diagnostics: [],
    });
    return Response.json({ error: { message: "接口不存在" } }, { status: 404 });
  });

  render(<App />);

  expect(await screen.findByRole("heading", { name: "同步到 Hugo" })).toBeInTheDocument();
  expect(screen.getAllByRole("heading", { name: "渠道路由测试" })).not.toHaveLength(0);
  expect(screen.getAllByRole("navigation", { name: "主导航" })).toHaveLength(2);
  expect(screen.getByRole("navigation", { name: "文章工作流" })).toBeInTheDocument();
});
