import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { ToastProvider } from "./ToastProvider";
import { HugoPublishFlow } from "./HugoPublishFlow";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

test("选择 Hugo Section 后预览同一 Artifact 并确认交付", async () => {
  const published = vi.fn();
  const requests: Array<{ url: string; method: string }> = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    requests.push({ url, method });
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: null });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "notes", article_count: 3 }, { name: "posts", article_count: 8 }], existing_section: "", selection_locked: false });
    if (url.endsWith("/articles/a1/hugo-previews")) return Response.json({ id: "preview_1", job_id: "preview_1", state: "queued" }, { status: 202 });
    if (url.endsWith("/hugo-previews/preview_1/confirm")) return Response.json({ job_id: "delivery_1", state: "queued" }, { status: 202 });
    if (url.endsWith("/hugo-previews/preview_1")) return Response.json({ id: "preview_1", content_hash: "hash", section: "posts", target_path: "content/posts/demo", change: "added", files: [{ relative_path: "index.md", media_type: "text/markdown", size: 1200 }], diagnostics: [], state: "ready", job_id: "preview_1" });
    if (url.endsWith("/jobs/delivery_1")) return Response.json({ id: "delivery_1", state: "succeeded", progress: 100 });
    throw new Error(`未处理请求: ${method} ${url}`);
  });

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onPublished={published} /></ToastProvider>);
  const section = await screen.findByRole("combobox", { name: "发布目录" });
  expect(screen.getByRole("button", { name: "生成发布预览" })).toBeDisabled();
  await userEvent.selectOptions(section, "posts");
  await userEvent.click(screen.getByRole("button", { name: "生成发布预览" }));

  expect(await screen.findByText("content/posts/demo")).toBeInTheDocument();
  expect(screen.getByText("index.md")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "确认同步到 Hugo" }));
  await waitFor(() => expect(published).toHaveBeenCalledOnce());
  expect(requests.some((request) => request.url.endsWith("/publications"))).toBe(false);
  expect(requests.filter((request) => request.url.endsWith("/hugo-previews/preview_1/confirm"))).toHaveLength(1);
});

test("刷新后直接恢复 Ready Hugo 预览", async () => {
  const requests: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    requests.push(url);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "ready", progress: 100, stage: "预览已准备", error: "", preview: { preview_id: "preview_ready", section: "posts", target_path: "content/posts/restored", change: "updated", files: [{ relative_path: "index.md", media_type: "text/markdown", size: 88 }], diagnostics: [], state: "ready" } } });
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onPublished={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("content/posts/restored")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认同步到 Hugo" })).toBeInTheDocument();
  expect(requests.some((url) => url.endsWith("/hugo-sections"))).toBe(false);
});

test("失败的 Hugo 预览显示阶段、原因和处理动作", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "failed", progress: 20, stage: "正在执行发布检查", error: "图片引用无法解析: missing.png", failure: { stage: "preflight", code: "source.image_unresolved", message: "图片引用无法解析: missing.png", action: "修复文章中的图片引用后重新生成预览", retryable: true } } });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 8 }], existing_section: "", selection_locked: false });
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onPublished={vi.fn()} /></ToastProvider>);

  expect(await screen.findByText("失败阶段：发布前检查")).toBeInTheDocument();
  expect(screen.getByText("图片引用无法解析: missing.png")).toBeInTheDocument();
  expect(screen.getByText("修复文章中的图片引用后重新生成预览")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重新生成预览" })).toBeInTheDocument();
});

test("手工预览轮询在组件卸载后停止请求", async () => {
  let previewReads = 0;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: null });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 8 }], existing_section: "", selection_locked: false });
    if (url.endsWith("/articles/a1/hugo-previews")) return Response.json({ id: "preview_1", job_id: "preview_1", state: "queued" }, { status: 202 });
    if (url.endsWith("/hugo-previews/preview_1")) {
      previewReads += 1;
      return Response.json({ id: "preview_1", content_hash: "hash", section: "posts", target_path: "content/posts/demo", change: "added", files: [], diagnostics: [], state: "preparing", job_id: "preview_1" });
    }
    throw new Error(`未处理请求: ${url}`);
  });

  const view = render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onPublished={vi.fn()} /></ToastProvider>);
  const section = await screen.findByRole("combobox", { name: "发布目录" });
  fireEvent.change(section, { target: { value: "posts" } });
  vi.useFakeTimers();
  fireEvent.click(screen.getByRole("button", { name: "生成发布预览" }));
  await vi.waitFor(() => expect(previewReads).toBe(1));
  view.unmount();
  await vi.advanceTimersByTimeAsync(800);
  expect(previewReads).toBe(1);
  vi.useRealTimers();
});
