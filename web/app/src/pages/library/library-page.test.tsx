import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ToastProvider } from "../../components/ToastProvider";
import { LibraryPage } from "./LibraryPage";

const articleA = { id: "a1", title: "第一篇", directory: "notes", category: "", modified_at: "2026-07-30T10:00:00Z", state: "pending_review", hugo_state: "尚未同步", wechat_state: "尚未准备", content_version: "v1" };
const articleB = { ...articleA, id: "a2", title: "第二篇", modified_at: "2026-07-30T09:00:00Z", content_version: "v2" };
const ignoredArticle = { ...articleA, id: "ignored", title: "已忽略文章", content_version: "ignored-v1", disposition: "ignored" };

test("内容库全选当前结果并在处置筛选变化后清理不可见选择", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).includes("disposition=ignored")
    ? Response.json({ items: [ignoredArticle], available_channels: ["hugo", "wechat"] })
    : Response.json({ items: [articleA, articleB], available_channels: ["hugo", "wechat"] }));
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);

  await userEvent.click(await screen.findByRole("checkbox", { name: "选择文章 第一篇" }));
  expect(screen.getByRole("checkbox", { name: "选择当前已加载文章" })).toHaveAttribute("aria-checked", "mixed");
  await userEvent.click(screen.getByRole("checkbox", { name: "选择当前已加载文章" }));
  expect(screen.getByText("已选择 2 篇")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "标记已发表" })).toBeInTheDocument();
  await userEvent.selectOptions(screen.getByLabelText("处置状态"), "ignored");

  await waitFor(() => expect(screen.queryByText("已选择 2 篇")).not.toBeInTheDocument());
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes("disposition=ignored"))).toBe(true);
  await userEvent.click(await screen.findByRole("checkbox", { name: "选择文章 已忽略文章" }));
  expect(screen.getByRole("button", { name: "恢复管理" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "标记已发表" })).not.toBeInTheDocument();
});

test("内容库处置筛选只发送固定枚举参数", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({ items: [articleA], available_channels: [] }));
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByText("第一篇");
  const select = screen.getByLabelText("处置状态");

  for (const disposition of ["published", "ignored", "unresolved"]) {
    await userEvent.selectOptions(select, disposition);
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes(`disposition=${disposition}`))).toBe(true));
  }
  expect(String(vi.mocked(fetch).mock.calls[0][0])).not.toContain("disposition=");
});

test("单行选择不打开文章且加载更多不自动选择新文章", async () => {
  const onNavigate = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).includes("cursor=page-2")
    ? Response.json({ items: [articleB], available_channels: ["hugo"] })
    : Response.json({ items: [articleA], next_cursor: "page-2", available_channels: ["hugo"] }));
  render(<ToastProvider><LibraryPage onNavigate={onNavigate} /></ToastProvider>);

  const first = await screen.findByRole("checkbox", { name: "选择文章 第一篇" });
  await userEvent.click(first);
  expect(onNavigate).not.toHaveBeenCalled();
  expect(screen.getByText("已选择 1 篇")).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "加载更多" }));
  const second = await screen.findByRole("checkbox", { name: "选择文章 第二篇" });
  expect(first).toBeChecked();
  expect(second).not.toBeChecked();
  expect(screen.getByText("已选择 1 篇")).toBeInTheDocument();
});

test("批量标记已发表提交文章版本和多个渠道，成功后清空选择并重新读取", async () => {
  let listRequests = 0;
  let submitted: unknown;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    if (init?.method === "POST") {
      submitted = JSON.parse(String(init.body));
      return Response.json({ processed: 2, changed: 2, unchanged: 0 });
    }
    listRequests += 1;
    return Response.json({ items: [articleA, articleB], available_channels: ["hugo", "wechat"] });
  });
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);

  await userEvent.click(await screen.findByRole("checkbox", { name: "选择当前已加载文章" }));
  await userEvent.click(screen.getByRole("button", { name: "标记已发表" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "Hugo" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "微信" }));
  await userEvent.click(screen.getByRole("button", { name: "确认标记" }));

  await waitFor(() => expect(submitted).toEqual({
    operation: "published",
    articles: [{ id: "a1", content_version: "v1" }, { id: "a2", content_version: "v2" }],
    channels: ["hugo", "wechat"],
  }));
  expect(await screen.findByRole("status")).toHaveTextContent("已处理 2 篇文章");
  await waitFor(() => expect(listRequests).toBe(2));
  expect(screen.queryByText("已选择 2 篇")).not.toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: "选择文章 第一篇" })).not.toBeChecked();
});

test("批量处置发生版本冲突时保留选择并给出可恢复提示", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => init?.method === "POST"
    ? Response.json({ error: { code: "disposition.content_changed", message: "内容版本冲突" } }, { status: 409 })
    : Response.json({ items: [articleA], available_channels: ["hugo"] }));
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);

  await userEvent.click(await screen.findByRole("checkbox", { name: "选择文章 第一篇" }));
  await userEvent.click(screen.getByRole("button", { name: "忽略" }));
  await userEvent.click(screen.getByRole("button", { name: "确认忽略" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("部分文章已更新，请刷新后重新选择");
  expect(screen.getByText("已选择 1 篇")).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: "选择文章 第一篇" })).toBeChecked();
});

test("已忽略筛选批量恢复时提交 restore", async () => {
  let submitted: unknown;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    if (init?.method === "POST") {
      submitted = JSON.parse(String(init.body));
      return Response.json({ processed: 1, changed: 1, unchanged: 0 });
    }
    return Response.json({ items: [ignoredArticle], available_channels: ["hugo", "wechat"] });
  });
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByText("已忽略文章");
  await userEvent.selectOptions(screen.getByLabelText("处置状态"), "ignored");
  await userEvent.click(await screen.findByRole("checkbox", { name: "选择文章 已忽略文章" }));
  await userEvent.click(screen.getByRole("button", { name: "恢复管理" }));
  await userEvent.click(screen.getByRole("button", { name: "确认恢复" }));

  await waitFor(() => expect(submitted).toEqual({
    operation: "restore",
    articles: [{ id: "ignored", content_version: "ignored-v1" }],
  }));
});

test("关闭批量对话框后焦点回到触发按钮", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({ items: [articleA], available_channels: ["hugo"] }));
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);
  await userEvent.click(await screen.findByRole("checkbox", { name: "选择文章 第一篇" }));
  const trigger = screen.getByRole("button", { name: "标记已发表" });
  await userEvent.click(trigger);
  await userEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(trigger).toHaveFocus();
});

test("内容库手动刷新工作区后重新读取当前筛选", async () => {
  let listRequests = 0;
  let resolveRefresh: ((response: Response) => void) | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    if (String(input).endsWith("/workspace/refresh")) {
      return new Promise<Response>((resolve) => { resolveRefresh = resolve; });
    }
    listRequests += 1;
    return Response.json({ items: [articleA], available_channels: [] });
  });
  render(<ToastProvider><LibraryPage onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByText("第一篇");

  const button = screen.getByRole("button", { name: "刷新工作区" });
  await userEvent.click(button);
  expect(screen.getByRole("button", { name: "刷新工作区" })).toBeDisabled();
  expect(screen.getByText("正在扫描…")).toBeInTheDocument();
  resolveRefresh?.(Response.json({ indexed: 1, failed: 0 }));

  expect(await screen.findByRole("status")).toHaveTextContent("内容库已更新，共索引 1 篇文章");
  await waitFor(() => expect(listRequests).toBe(2));
});
