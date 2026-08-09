import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { ToastProvider } from "../../components/ToastProvider";
import { HugoPage } from "./HugoPage";

const article = {
  id: "a1", content_version: "hash", content_stage: "ready", hugo_provider_id: "h1", wechat_provider_id: "w1",
  relative_path: "Areas/demo.md", modified_at: "2026-08-03", metadata: { title: "独立发布流程", description: "", category: "", series: "", tags: [], keywords: [], slug: "demo", cover: "" },
  preview_html: "<p>正文</p>", source_changed: false, review_state: "已通过", hugo_state: "需要同步", wechat_state: "尚未准备", xiaohongshu_state: "尚未准备",
  checks: [], ai_configured: false, suggestions: [], suggestions_stale: false, wechat_copied: false, resource_diagnostics: [],
};

afterEach(() => vi.restoreAllMocks());

test("Hugo 独立页面显示渠道导航和同步流程", async () => {
  const navigate = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: null });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 8 }], existing_section: "", selection_locked: false });
    if (url.endsWith("/articles/a1")) return Response.json(article);
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPage articleID="a1" onNavigate={navigate} /></ToastProvider>);

  expect(await screen.findByRole("heading", { name: "同步到 Hugo" })).toBeInTheDocument();
  expect(screen.getAllByText("独立发布流程")).toHaveLength(1);
  expect(screen.getByRole("article", { name: "Hugo 发布内容" })).toHaveTextContent("正文");
  expect(screen.getByRole("button", { name: /同步到 Hugo/ })).toHaveAttribute("aria-current", "page");
  expect(await screen.findByRole("combobox", { name: "发布目录" })).toHaveValue("posts");
  expect(screen.queryByRole("heading", { name: "元数据" })).not.toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: /发布到微信/ }));
  expect(navigate).toHaveBeenCalledWith("/articles/a1/wechat");
});

test("Ready 预览在正文区域显示真实 Hugo 渲染页面", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "ready", progress: 100, stage: "预览已准备", preview: { preview_id: "preview_ready", section: "posts", target_path: "content/posts/demo", change: "updated", files: [], diagnostics: [], render_url: "/api/v1/hugo-previews/preview_ready/render/posts/demo/", state: "ready" } } });
    if (url.endsWith("/articles/a1")) return Response.json(article);
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPage articleID="a1" onNavigate={vi.fn()} /></ToastProvider>);

  const rendered = await screen.findByTitle("Hugo 当前文章渲染预览");
  expect(rendered).toHaveAttribute("src", "/api/v1/hugo-previews/preview_ready/render/posts/demo/");
  expect(rendered).toHaveAttribute("sandbox", "allow-same-origin");
  expect(rendered.getAttribute("sandbox")).not.toContain("allow-scripts");
  expect(screen.getByRole("article", { name: "Hugo 发布内容" })).not.toHaveTextContent("正文");
  expect(screen.getByText("待确认")).toBeInTheDocument();
});

test("同步成功后继续显示 Hugo 渲染页面并标记已同步", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "published", progress: 100, stage: "已同步", preview: { preview_id: "preview_published", section: "posts", target_path: "content/posts/demo", change: "updated", files: [], diagnostics: [], render_url: "/api/v1/hugo-previews/preview_published/render/posts/demo/", state: "ready" } } });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 8 }], existing_section: "posts", existing_directory: "", selection_locked: true });
    if (url.endsWith("/articles/a1/publication-history")) return Response.json({ items: [], next_cursor: "" });
    if (url.endsWith("/articles/a1")) return Response.json({ ...article, hugo_state: "已同步" });
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPage articleID="a1" onNavigate={vi.fn()} /></ToastProvider>);

  const rendered = await screen.findByTitle("Hugo 当前文章渲染预览");
  expect(rendered).toHaveAttribute("src", "/api/v1/hugo-previews/preview_published/render/posts/demo/");
  expect(rendered.closest(".hugo-render-document")).toHaveTextContent("已同步");
  expect(screen.getByRole("article", { name: "Hugo 发布内容" })).not.toHaveTextContent("正文");
});
