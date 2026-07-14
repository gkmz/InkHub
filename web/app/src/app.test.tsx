import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { App } from "./app";

const articles = [
  { id: "a2", title: "内容已更新", directory: "notes", category: "工程", modified_at: "2026-07-14T09:00:00Z", state: "changed", hugo_state: "需要同步", wechat_state: "尚未准备" },
  { id: "a1", title: "发布失败", directory: "drafts", category: "产品", modified_at: "2026-07-14T10:00:00Z", state: "blocked", hugo_state: "处理失败", wechat_state: "尚未准备" },
];

function mockAPI(hasWorkspace = true) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.includes("/session")) return Response.json({ has_workspace: hasWorkspace, workspace: hasWorkspace ? { id: "w1", name: "我的文章" } : null });
    if (url.includes("/dashboard")) return Response.json({ items: articles });
    if (url.includes("/articles")) return Response.json({ items: articles, next_cursor: "" });
    return Response.json({ error: { code: "not_found", message: "接口不存在" } }, { status: 404 });
  });
}

test("首次运行显示四步初始化并允许跳过可选渠道", async () => {
  mockAPI(false);
  render(<App />);
  expect(await screen.findByRole("heading", { name: "选择内容库" })).toBeInTheDocument();
  expect(screen.getByText("1 / 4")).toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("工作区名称"), "写作空间");
  await userEvent.type(screen.getByLabelText("Obsidian Vault 路径"), "/Users/me/Vault");
  await userEvent.click(screen.getByRole("button", { name: "继续" }));
  expect(screen.getByRole("heading", { name: "确认博客" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "暂不配置博客" }));
  expect(screen.getByRole("heading", { name: "配置微信" })).toBeInTheDocument();
});

test("工作台按失败、更新和待审核的优先级排列", async () => {
  mockAPI();
  render(<App />);
  const rows = await screen.findAllByTestId("dashboard-row");
  expect(rows.map((row) => row.textContent)).toEqual(expect.arrayContaining([expect.stringContaining("发布失败"), expect.stringContaining("内容已更新")]));
  expect(screen.getByRole("heading", { name: "发布失败" }).compareDocumentPosition(screen.getByRole("heading", { name: "内容已更新" })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
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
  vi.mocked(fetch).mockImplementation(async (input) => String(input).includes("/session")
    ? Response.json({ has_workspace: true, workspace: { id: "w1", name: "我的文章" } })
    : Response.json({ items: [] }));
  render(<App />);
  expect(await screen.findByRole("heading", { name: "目前没有需要处理的文章" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "浏览内容库" }));
  expect(await screen.findByRole("heading", { name: "内容库" })).toBeInTheDocument();
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
