import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { ToastProvider } from "../../components/ToastProvider";
import { TaxonomyPage } from "./TaxonomyPage";

const overview = {
  source: "我的博客",
  provider_id: "h1",
  provider_type: "hugo",
  state: "ready",
  revision: "revision-1",
  loaded_at: "2026-07-15 08:00",
  attempted_at: "2026-07-15 08:00",
  readonly: false,
  terms: [
    { kind: "category", key: "engineering", name: "Engineering", usage_count: 3, metadata: {} },
    { kind: "category", key: "product", name: "产品", usage_count: 2, metadata: {} },
    { kind: "tag", key: "go", name: "Go", usage_count: 5, metadata: {} },
    { kind: "series", key: "go-in-practice", name: "Go 工程实践", usage_count: 4, metadata: {} },
  ],
  issues: [],
};

afterEach(() => vi.restoreAllMocks());

test("类目页面展示真实快照并支持搜索和手工刷新", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async () => Response.json(overview));
  render(<ToastProvider><TaxonomyPage /></ToastProvider>);
  expect(await screen.findByText("Engineering")).toBeInTheDocument();
  expect(screen.getByText("3 篇文章")).toBeInTheDocument();
  await userEvent.type(screen.getByRole("searchbox", { name: "搜索类目" }), "产品");
  expect(screen.queryByText("Engineering")).not.toBeInTheDocument();
  expect(screen.getByText("产品")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "刷新类目" }));
  await waitFor(() => expect(fetchMock.mock.calls.some(([url, init]) => String(url).endsWith("/taxonomy/refresh") && init?.method === "POST")).toBe(true));
  expect(await screen.findByRole("status")).toHaveTextContent("类目已刷新");
});

test("新增类目先预览 Hugo 原生变更再确认应用", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/taxonomy/terms/preview")) return Response.json({ provider_id: "h1", expected_revision: "revision-1", files: [{ relative_path: "content/categories/ai/_index.md", before: "", after: "---\ntitle: AI\ndescription: AI 文章\n---\n" }] });
    if (url.endsWith("/taxonomy/terms/apply")) return Response.json({ ...overview, revision: "revision-2", terms: [...overview.terms, { kind: "category", key: "ai", name: "AI", usage_count: 0, metadata: { description: "AI 文章" } }] });
    if (init?.method === "GET" || !init?.method) return Response.json(overview);
    return Response.json({ error: { code: "not_found", message: "接口不存在" } }, { status: 404 });
  });
  render(<ToastProvider><TaxonomyPage /></ToastProvider>);
  await screen.findByText("Engineering");
  await userEvent.click(screen.getByRole("button", { name: "新建类目" }));
  await userEvent.type(screen.getByLabelText("类目名称"), "AI");
  await userEvent.type(screen.getByLabelText("类目说明"), "AI 文章");
  await userEvent.click(screen.getByRole("button", { name: "预览变更" }));
  expect(await screen.findByText("content/categories/ai/_index.md")).toBeInTheDocument();
  expect(screen.getByText(/description: AI 文章/)).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "确认创建类目" }));
  expect(await screen.findByText("AI")).toBeInTheDocument();
  expect(await screen.findByRole("status")).toHaveTextContent("类目已创建");
});

test("未配置 Hugo 时显示明确设置引导", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({ source: "尚未配置", state: "not_enabled", loaded_at: "-", readonly: true, terms: [], issues: [] }));
  render(<ToastProvider><TaxonomyPage /></ToastProvider>);
  expect(await screen.findByRole("heading", { name: "尚未连接博客类目" })).toBeInTheDocument();
  expect(screen.getByText("请先在设置中连接 Hugo 博客。" )).toBeInTheDocument();
});

test("新增类目对话框支持键盘取消", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(overview));
  render(<ToastProvider><TaxonomyPage /></ToastProvider>);
  await screen.findByText("Engineering");
  await userEvent.click(screen.getByRole("button", { name: "新建类目" }));
  expect(screen.getByRole("dialog", { name: "新建类目" })).toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "新建类目" })).not.toBeInTheDocument();
});

test("可切换查看 Hugo 系列且不提供类目创建入口", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(overview));
  render(<ToastProvider><TaxonomyPage /></ToastProvider>);
  await screen.findByText("Engineering");
  await userEvent.click(screen.getByRole("tab", { name: "系列" }));
  expect(screen.getByText("Go 工程实践")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "新建类目" })).not.toBeInTheDocument();
});
