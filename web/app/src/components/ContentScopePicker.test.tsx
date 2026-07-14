import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ContentScopePicker } from "./ContentScopePicker";

const directories = [
  { path: "Areas", markdown_count: 12 },
  { path: "Areas/私人记录", markdown_count: 3 },
  { path: "Projects", markdown_count: 5 },
];

test("内容范围支持多个管理目录并添加内部忽略目录", async () => {
  const onChange = vi.fn();
  const { rerender } = render(<ContentScopePicker directories={directories} contentRoots={[]} ignoredFolders={[]} onChange={onChange} />);

  await userEvent.click(screen.getByRole("checkbox", { name: "Areas（12 篇）" }));
  expect(onChange).toHaveBeenLastCalledWith(["Areas"], []);

  rerender(<ContentScopePicker directories={directories} contentRoots={["Areas"]} ignoredFolders={[]} onChange={onChange} />);
  await userEvent.selectOptions(screen.getByLabelText("要忽略的子目录"), "Areas/私人记录");
  await userEvent.click(screen.getByRole("button", { name: "添加忽略目录" }));
  expect(onChange).toHaveBeenLastCalledWith(["Areas"], ["Areas/私人记录"]);
});

test("未选择内容目录时明确说明不会扫描", () => {
  render(<ContentScopePicker directories={directories} contentRoots={[]} ignoredFolders={[]} onChange={vi.fn()} />);
  expect(screen.getByText("尚未选择内容目录，InkHub 不会扫描任何笔记。")).toBeInTheDocument();
});
