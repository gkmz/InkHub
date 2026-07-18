import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ToastProvider } from "../../components/ToastProvider";
import { WeChatPlan } from "./WeChatPlan";

test("微信计划先展示图片清单，用户确认后才创建准备任务", async () => {
  const requests: Array<{ url: string; method: string }> = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    requests.push({ url, method });
    if (url.endsWith("/wechat-plans/confirm")) return Response.json({ state: "queued" }, { status: 202 });
    return Response.json({ plan_token: "opaque", template_id: "default", ready: true, expires_at: "2026-07-18T12:00:00Z", diagnostics: [], images: [{ reference: "images/很长的封面图片.png", media_type: "image/png", size: 1200, state: "upload" }, { reference: "images/reused.webp", media_type: "image/webp", size: 800, state: "reuse" }] });
  });
  const confirmed = vi.fn();
  render(<ToastProvider><WeChatPlan articleID="a1" templateID="default" onConfirmed={confirmed} /></ToastProvider>);

  expect(await screen.findByText("images/很长的封面图片.png")).toBeInTheDocument();
  expect(screen.getByText("将上传")).toBeInTheDocument();
  expect(screen.getByText("已存在，直接复用")).toBeInTheDocument();
  expect(requests).toHaveLength(1);
  await userEvent.click(screen.getByRole("button", { name: "确认并准备" }));
  expect(confirmed).toHaveBeenCalledOnce();
  expect(requests).toHaveLength(2);
});
