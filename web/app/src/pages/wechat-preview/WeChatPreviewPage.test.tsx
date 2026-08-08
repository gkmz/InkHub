import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ToastProvider } from "../../components/ToastProvider";
import { WeChatPreviewPage } from "./WeChatPreviewPage";

const article = {
  id: "article-1",
  stable_id: "article_TEST",
  content_version: "hash-1",
  content_stage: "ready",
  hugo_provider_id: "hugo-1",
  wechat_provider_id: "wechat-1",
  relative_path: "Areas/article.md",
  modified_at: "2026-08-05",
  metadata: { title: "微信文章", description: "文章摘要", category: "工程", series: "", tags: [], keywords: [], slug: "wechat-article", cover: "" },
  preview_html: "<p>预览正文</p>",
  source_changed: false,
  review_state: "已通过",
  hugo_state: "尚未同步",
  wechat_state: "尚未准备",
  checks: [],
  ai_configured: false,
  suggestions: [],
  suggestions_stale: false,
  wechat_copied: false,
  resource_diagnostics: [],
};

test("准备完成区域直接展示微信交付操作", async () => {
  let prepared = false;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/settings")) return Response.json({ default_template: "default" });
    if (url.endsWith("/wechat-plans/confirm")) { prepared = true; return Response.json({ state: "queued" }); }
    if (url.endsWith("/wechat-plans")) return Response.json({ plan_token: "plan", template_id: "default", ready: true, expires_at: "2026-08-05T12:00:00Z", diagnostics: [], images: [] });
    if (url.includes("/wechat/content/")) return Response.json({ html: "<p><strong>格式化正文</strong></p>" });
    if (url.includes("/articles/article-1")) return Response.json({ ...article, wechat_state: prepared ? "已准备" : "尚未准备" });
    return Response.json({});
  });

  render(<ToastProvider><WeChatPreviewPage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  await userEvent.click(await screen.findByRole("button", { name: "确认并准备" }));

  const readyHeading = await screen.findByRole("heading", { name: "内容已准备" });
  const readyRegion = readyHeading.closest("aside");
  expect(readyRegion).not.toBeNull();
  expect(within(readyRegion as HTMLElement).getByRole("button", { name: "复制格式化内容" })).toBeVisible();
  expect(within(readyRegion as HTMLElement).getByRole("button", { name: "打开微信公众平台" })).toBeVisible();
});

test("已准备内容读取失败时不回退到原文，并支持重新读取", async () => {
  let reads = 0;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/settings")) return Response.json({ default_template: "default" });
    if (url.includes("/wechat/content/")) {
      reads += 1;
      if (reads === 1) return Response.json({ error: { message: "处理结果暂时不可用" } }, { status: 503 });
      return Response.json({ html: "<p><strong>处理后的正文</strong></p>", mermaid_theme: "modern" });
    }
    if (url.includes("/articles/article-1")) return Response.json({ ...article, preview_html: "<p>不应显示的原文</p>", wechat_state: "已准备" });
    return Response.json({});
  });

  render(<ToastProvider><WeChatPreviewPage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("处理结果暂时不可用")).toBeVisible();
  expect(screen.queryByText("不应显示的原文")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "重新读取处理结果" }));
  expect(await screen.findByText("处理后的正文")).toBeVisible();
  expect(screen.getByRole("button", { name: "现代" })).toHaveAttribute("aria-pressed", "true");
  expect(screen.queryByText("不应显示的原文")).not.toBeInTheDocument();
});
