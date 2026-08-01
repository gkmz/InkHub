import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { ToastProvider } from "../../components/ToastProvider";
import { DashboardPage } from "./DashboardPage";

afterEach(() => vi.restoreAllMocks());

const article = (id: string, title: string, state: string) => ({
  id,
  title,
  directory: "Areas",
  category: "",
  modified_at: "2026-07-31T10:00:00Z",
  state,
  hugo_state: "尚未同步",
  wechat_state: "尚未准备",
  content_version: id,
  content_stage: "ready",
});

test("工作台将最新已就绪文章置于第一组", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({
    latest_ready: [article("ready-1", "最新已就绪文章", "pending_review")],
    failed: [article("failed-1", "失败文章", "blocked")],
    changed: [],
    needs_review: [],
    ready_to_publish: [],
    recently_handled: [],
  }));

  render(<ToastProvider><DashboardPage onNavigate={vi.fn()} /></ToastProvider>);

  const headings = await screen.findAllByRole("heading", { level: 2 });
  expect(headings[0]).toHaveTextContent("最新已就绪");
  expect(headings[1]).toHaveTextContent("处理失败");
});

test("工作台手动刷新工作区后重新读取数据", async () => {
  let dashboardRequests = 0;
  let resolveRefresh: ((response: Response) => void) | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    if (String(input).endsWith("/workspace/refresh")) {
      return new Promise<Response>((resolve) => { resolveRefresh = resolve; });
    }
    dashboardRequests += 1;
    return Response.json({ latest_ready: [article("ready-1", "最新已就绪文章", "pending_review")], failed: [], changed: [], needs_review: [], ready_to_publish: [], recently_handled: [] });
  });
  render(<ToastProvider><DashboardPage onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByText("最新已就绪文章");

  const button = screen.getByRole("button", { name: "刷新工作区" });
  await userEvent.click(button);
  expect(screen.getByRole("button", { name: "刷新工作区" })).toBeDisabled();
  expect(screen.getByText("正在扫描…")).toBeInTheDocument();
  resolveRefresh?.(Response.json({ indexed: 1, failed: 0 }));

  expect(await screen.findByRole("status")).toHaveTextContent("内容库已更新，共索引 1 篇文章");
  await waitFor(() => expect(dashboardRequests).toBe(2));
});
