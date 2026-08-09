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
  const requests: Array<{ url: string; method: string; body?: string }> = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    requests.push({ url, method, body: typeof init?.body === "string" ? init.body : undefined });
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: null });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "notes", article_count: 3, directories: [] }, { name: "posts", article_count: 8, directories: [{ path: "ai", article_count: 6 }, { path: "tools", article_count: 2 }] }], existing_section: "", existing_directory: "", selection_locked: false });
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
  const directory = screen.getByRole("combobox", { name: "分类目录" });
  await userEvent.selectOptions(directory, "ai");
  await userEvent.click(screen.getByRole("button", { name: "生成发布预览" }));

  expect(await screen.findByText("content/posts/demo")).toBeInTheDocument();
  expect(screen.getByText("index.md")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "确认同步到 Hugo" }));
  await waitFor(() => expect(published).toHaveBeenCalledOnce());
  expect(screen.queryByRole("button", { name: "确认同步到 Hugo" })).not.toBeInTheDocument();
  expect(screen.getByText("当前版本已同步到 Hugo")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /生成发布预览/ })).not.toBeInTheDocument();
  expect(requests.some((request) => request.url.endsWith("/publications"))).toBe(false);
  expect(requests.find((request) => request.url.endsWith("/articles/a1/hugo-previews"))?.body).toContain('"directory":"ai"');
  expect(requests.filter((request) => request.url.endsWith("/hugo-previews/preview_1/confirm"))).toHaveLength(1);
});

test("刷新后直接恢复 Ready Hugo 预览", async () => {
  const requests: string[] = [];
  const renderChanged = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    requests.push(url);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "ready", progress: 100, stage: "预览已准备", error: "", preview: { preview_id: "preview_ready", section: "posts", target_path: "content/posts/restored", change: "updated", files: [{ relative_path: "index.md", media_type: "text/markdown", size: 88 }], diagnostics: [], render_url: "/api/v1/hugo-previews/preview_ready/render/posts/restored/", state: "ready" } } });
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onRenderPreviewChange={renderChanged} onPublished={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("content/posts/restored")).toBeInTheDocument();
  await waitFor(() => expect(renderChanged).toHaveBeenLastCalledWith({ url: "/api/v1/hugo-previews/preview_ready/render/posts/restored/", expired: false, published: false }));
  expect(screen.getByRole("button", { name: "确认同步到 Hugo" })).toBeInTheDocument();
  expect(requests.some((url) => url.endsWith("/hugo-sections"))).toBe(false);
});

test("当前版本已经同步且 Bundle 仍存在时进入终态，不再显示同步按钮", async () => {
  const renderChanged = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "published", progress: 100, stage: "已同步", error: "", preview: { preview_id: "preview_published", section: "posts", target_path: "content/posts/20260804-superpowers-workflow", change: "updated", files: [], diagnostics: [], render_url: "/api/v1/hugo-previews/preview_published/render/posts/demo/", state: "ready" } } });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 8 }], existing_section: "posts", existing_directory: "", selection_locked: true });
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onRenderPreviewChange={renderChanged} onPublished={vi.fn()} /></ToastProvider>);

  expect(await screen.findByText("当前版本已同步到 Hugo")).toBeInTheDocument();
  expect(screen.getByText("content/posts/20260804-superpowers-workflow")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "确认同步到 Hugo" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /生成发布预览/ })).not.toBeInTheDocument();
  await waitFor(() => expect(renderChanged).toHaveBeenLastCalledWith({ url: "/api/v1/hugo-previews/preview_published/render/posts/demo/", expired: false, published: true }));
});

test("已同步记录但 Hugo Bundle 被删除时重新扫描并生成新预览", async () => {
  const published = vi.fn();
  let previewID = "";
  let refreshKey = "";
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "published", progress: 100, stage: "已同步", error: "", preview: { preview_id: "preview_old", section: "posts", target_path: "content/posts/20260731-superpower", change: "updated", files: [], diagnostics: [], state: "ready" } } });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 1 }], existing_section: "", existing_directory: "", selection_locked: false });
    if (url.endsWith("/articles/a1/hugo-previews")) {
      const body = JSON.parse(String(init?.body ?? "{}")) as { refresh_key?: string };
      refreshKey = body.refresh_key ?? "";
      previewID = "preview_new";
      return Response.json({ id: previewID, job_id: previewID, state: "queued" }, { status: 202 });
    }
    if (url.endsWith(`/hugo-previews/${previewID}`)) return Response.json({ id: previewID, content_hash: "hash", section: "posts", target_path: "content/posts/20260804-superpower", change: "added", files: [], diagnostics: [], state: "ready", job_id: previewID });
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onPublished={published} /></ToastProvider>);
  expect(await screen.findByText("检测到 Hugo 原发布目录已不存在，将按当前文章重新生成。")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "生成发布预览" }));
  expect(await screen.findByText("content/posts/20260804-superpower")).toBeInTheDocument();
  expect(refreshKey).not.toBe("");
  expect(published).not.toHaveBeenCalled();
});

test("过期预览重新生成前会重新读取 Hugo 目录", async () => {
  const requests: string[] = [];
  let refreshKey = "";
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    requests.push(url);
    if (url.endsWith("/articles/a1/publication-workflow")) return Response.json({ article_id: "a1", hugo: { state: "expired", progress: 100, stage: "预览已过期", error: "", preview: { preview_id: "preview_expired", section: "posts", target_path: "content/posts/demo", change: "updated", files: [], diagnostics: [], state: "expired" } } });
    if (url.endsWith("/articles/a1/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 2 }], existing_section: "", existing_directory: "", selection_locked: false });
    if (url.endsWith("/articles/a1/hugo-previews")) {
      const body = JSON.parse(String(init?.body ?? "{}")) as { refresh_key?: string };
      refreshKey = body.refresh_key ?? "";
      return Response.json({ id: "preview_new", job_id: "preview_new", state: "queued" }, { status: 202 });
    }
    if (url.endsWith("/hugo-previews/preview_new")) return Response.json({ id: "preview_new", content_hash: "hash", section: "posts", target_path: "content/posts/demo", change: "updated", files: [], diagnostics: [], state: "ready", job_id: "preview_new" });
    throw new Error(`未处理请求: ${url}`);
  });

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onPublished={vi.fn()} /></ToastProvider>);
  await userEvent.click(await screen.findByRole("button", { name: "重新生成预览" }));

  expect(await screen.findByText("content/posts/demo")).toBeInTheDocument();
  expect(requests.some((url) => url.endsWith("/articles/a1/hugo-sections"))).toBe(true);
  expect(requests.some((url) => url.endsWith("/articles/a1/hugo-previews"))).toBe(true);
  expect(refreshKey).not.toBe("");
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

test("开发模式取消旧请求时不显示失败提示", async () => {
  vi.spyOn(globalThis, "fetch").mockRejectedValue(new DOMException("signal is aborted without reason", "AbortError"));

  render(<ToastProvider><HugoPublishFlow articleID="a1" contentHash="hash" onPublished={vi.fn()} /></ToastProvider>);

  await waitFor(() => expect(screen.queryByText("正在恢复 Hugo 发布状态…")).not.toBeInTheDocument());
  await new Promise((resolve) => window.setTimeout(resolve, 20));
  expect(screen.getByRole("region", { name: "操作提示" })).toBeEmptyDOMElement();
});
