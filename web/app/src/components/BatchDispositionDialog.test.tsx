import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { BatchDispositionDialog } from "./BatchDispositionDialog";

test("标记已发表允许多选已启用渠道并阻止空提交", async () => {
  const confirm = vi.fn();
  render(<BatchDispositionDialog mode="published" count={3} channels={{ hugo: true, wechat: true }} onClose={vi.fn()} onConfirm={confirm} />);
  expect(screen.getByRole("button", { name: "确认标记" })).toBeDisabled();
  await userEvent.click(screen.getByRole("checkbox", { name: "Hugo" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "微信" }));
  await userEvent.click(screen.getByRole("button", { name: "确认标记" }));
  expect(confirm).toHaveBeenCalledWith(["hugo", "wechat"]);
});

test("未配置渠道禁用选择并提供设置入口", async () => {
  const openSettings = vi.fn();
  render(<BatchDispositionDialog mode="published" count={1} channels={{ hugo: false, wechat: false }} onClose={vi.fn()} onConfirm={vi.fn()} onOpenSettings={openSettings} />);
  expect(screen.getByRole("checkbox", { name: "Hugo" })).toBeDisabled();
  expect(screen.getByRole("checkbox", { name: "微信" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "前往设置" }));
  expect(openSettings).toHaveBeenCalledOnce();
});

test("忽略确认说明长期效果且提交时无法误关", () => {
  const close = vi.fn();
  const view = render(<BatchDispositionDialog mode="ignored" count={2} channels={{ hugo: true, wechat: true }} busy onClose={close} onConfirm={vi.fn()} />);
  expect(screen.getByText(/内容更新后仍会保持忽略/)).toBeInTheDocument();
  fireEvent.keyDown(window, { key: "Escape" });
  fireEvent.mouseDown(view.container.querySelector(".dialog-backdrop") as HTMLElement);
  expect(close).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "确认忽略" })).toBeDisabled();
});
