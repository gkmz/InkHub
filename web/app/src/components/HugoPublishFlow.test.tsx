import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { ToastProvider } from "./ToastProvider";
import { HugoPublishFlow } from "./HugoPublishFlow";

afterEach(() => vi.restoreAllMocks());

test("选择 Hugo Section 后预览同一 Artifact 并确认交付", async () => {
  const published = vi.fn();
  const requests: Array<{ url: string; method: string }> = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    requests.push({ url, method });
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
