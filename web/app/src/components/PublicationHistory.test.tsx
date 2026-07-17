import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { PublicationHistory } from "./PublicationHistory";

afterEach(() => vi.restoreAllMocks());

test("发布历史默认折叠并稳定加载更多", async () => {
  const requests: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    requests.push(url);
    if (url.includes("cursor=next")) return Response.json({ items: [{ id: "h2", channel: "wechat", state: "confirmed", title: "已确认保存微信草稿", detail: "用户已确认草稿保存", occurred_at: "2026-07-16T12:00:00Z" }] });
    return Response.json({ items: [{ id: "h1", channel: "hugo", state: "published", title: "已同步到 Hugo", detail: "博客内容已更新", occurred_at: "2026-07-17T12:00:00Z" }], next_cursor: "next" });
  });

  render(<PublicationHistory articleID="a1" refreshKey={0} />);
  expect(screen.queryByText("已同步到 Hugo")).not.toBeInTheDocument();
  await userEvent.click(screen.getByText("发布历史"));
  expect(await screen.findByText("已同步到 Hugo")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "加载更多历史" }));
  expect(await screen.findByText("已确认保存微信草稿")).toBeInTheDocument();
  expect(requests.some((url) => url.includes("cursor=next"))).toBe(true);
});
