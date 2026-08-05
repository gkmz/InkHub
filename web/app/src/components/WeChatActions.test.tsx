import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { WeChatActions } from "./WeChatActions";

test("准备完成后同时提供复制和打开微信公众平台操作", async () => {
  const openPlatform = vi.fn();
  render(<WeChatActions html="<p>正文</p>" copied={false} onCopy={vi.fn().mockResolvedValue(undefined)} onConfirm={vi.fn()} onOpenPlatform={openPlatform} />);

  expect(screen.getByRole("button", { name: "复制格式化内容" })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "打开微信公众平台" }));
  expect(openPlatform).toHaveBeenCalledOnce();
});

test("自动复制彻底失败后可以打开手工复制界面", async () => {
  render(<WeChatActions html="<p><strong>格式化正文</strong></p>" copied={false} onCopy={vi.fn().mockRejectedValue(new Error("denied"))} onConfirm={vi.fn()} onOpenPlatform={vi.fn()} />);

  await userEvent.click(screen.getByRole("button", { name: "复制格式化内容" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("无法自动写入剪贴板");
  await userEvent.click(screen.getByRole("button", { name: "手工复制" }));
  expect(screen.getByRole("dialog", { name: "手工复制微信内容" })).toBeVisible();
  expect(screen.getByText("格式化正文")).toBeVisible();
});

test("复制成功后才显示草稿保存确认", async () => {
  render(<WeChatActions html="<p>正文</p>" copied={false} onCopy={vi.fn().mockResolvedValue(undefined)} onConfirm={vi.fn()} onOpenPlatform={vi.fn()} />);

  expect(screen.queryByRole("button", { name: "草稿已保存" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "复制格式化内容" }));
  expect(screen.getByRole("button", { name: "草稿已保存" })).toBeVisible();
});
