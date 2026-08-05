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
