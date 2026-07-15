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
  const { rerender } = render(<ContentScopePicker directories={directories} contentRoots={[]} ignoredFolders={[]} ignoredFileNames={["index.md", "_index.md"]} onChange={onChange} />);

  await userEvent.click(screen.getByRole("checkbox", { name: "Areas（12 篇）" }));
  expect(onChange).toHaveBeenLastCalledWith(["Areas"], [], ["index.md", "_index.md"]);

  rerender(<ContentScopePicker directories={directories} contentRoots={["Areas"]} ignoredFolders={[]} ignoredFileNames={["index.md", "_index.md"]} onChange={onChange} />);
  await userEvent.click(screen.getByRole("checkbox", { name: "忽略 Areas/私人记录（3 篇）" }));
  expect(onChange).toHaveBeenLastCalledWith(["Areas"], ["Areas/私人记录"], ["index.md", "_index.md"]);
  rerender(<ContentScopePicker directories={directories} contentRoots={["Areas"]} ignoredFolders={["Areas/私人记录"]} ignoredFileNames={["index.md", "_index.md"]} onChange={onChange} />);
  await userEvent.type(screen.getByLabelText("搜索忽略目录"), "私人");
  expect(screen.getByRole("checkbox", { name: "忽略 Areas/私人记录（3 篇）" })).toBeChecked();
  await userEvent.type(screen.getByLabelText("添加忽略文件名"), "README.md");
  await userEvent.click(screen.getByRole("button", { name: "添加文件名" }));
  expect(onChange).toHaveBeenLastCalledWith(["Areas"], ["Areas/私人记录"], ["_index.md", "index.md", "readme.md"]);
});

test("未选择内容目录时明确说明不会扫描", () => {
  render(<ContentScopePicker directories={directories} contentRoots={[]} ignoredFolders={[]} ignoredFileNames={[]} onChange={vi.fn()} />);
  expect(screen.getByText("尚未选择内容目录，InkHub 不会扫描任何笔记。")).toBeInTheDocument();
});
