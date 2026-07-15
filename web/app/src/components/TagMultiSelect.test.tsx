import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { TagMultiSelect } from "./TagMultiSelect";

const options = [
  { key: "go", name: "Go", usageCount: 18 },
  { key: "inkhub", name: "InkHub", usageCount: 4 },
  { key: "sqlite", name: "SQLite", usageCount: 7 },
];

test("Tag 多选展示文章数量并保留快照外旧值", async () => {
  render(<TagMultiSelect value={["Go", "旧标签"]} options={options} state="ready" onChange={vi.fn()} />);
  expect(screen.getByText("旧标签")).toBeInTheDocument();
  expect(screen.getByText("博客中未发现")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("combobox", { name: "搜索或创建 Tag" }));
  expect(screen.queryByRole("option", { name: /Go/ })).not.toBeInTheDocument();
  expect(screen.getByRole("option", { name: "InkHub，4 篇文章" })).toBeInTheDocument();
});

test("Tag 多选支持搜索、选择和创建且按标准名称去重", async () => {
  const change = vi.fn();
  const view = render(<TagMultiSelect value={[]} options={options} state="ready" onChange={change} />);
  const input = screen.getByRole("combobox", { name: "搜索或创建 Tag" });
  await userEvent.type(input, "ink");
  await userEvent.click(screen.getByRole("option", { name: "InkHub，4 篇文章" }));
  expect(change).toHaveBeenLastCalledWith(["InkHub"]);

  view.rerender(<TagMultiSelect value={["InkHub"]} options={options} state="ready" onChange={change} />);
  await userEvent.type(input, "inkhub");
  expect(screen.queryByText("创建“inkhub”")).not.toBeInTheDocument();
  await userEvent.clear(input);
  await userEvent.type(input, "New Topic{Enter}");
  expect(change).toHaveBeenLastCalledWith(["InkHub", "New Topic"]);
});

test("Tag 多选支持键盘选择、关闭和退格删除", async () => {
  const change = vi.fn();
  const view = render(<TagMultiSelect value={[]} options={options} state="ready" onChange={change} />);
  const input = screen.getByRole("combobox", { name: "搜索或创建 Tag" });
  await userEvent.click(input);
  await userEvent.keyboard("{ArrowDown}{Enter}");
  expect(change).toHaveBeenLastCalledWith(["InkHub"]);

  view.rerender(<TagMultiSelect value={["InkHub"]} options={options} state="ready" onChange={change} />);
  await userEvent.click(input);
  expect(screen.getByRole("listbox")).toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  await userEvent.keyboard("{Backspace}");
  expect(change).toHaveBeenLastCalledWith([]);
});

test("Tag 数量和 taxonomy 状态只提供反馈", () => {
  const { rerender } = render(<TagMultiSelect value={["Go"]} options={options} state="not_enabled" onChange={vi.fn()} />);
  expect(screen.getByText("建议至少选择 3 个 Tag")).toBeInTheDocument();
  expect(screen.getByText("尚未连接博客标签，仍可手工添加")).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "搜索或创建 Tag" })).toBeEnabled();

  rerender(<TagMultiSelect value={["1", "2", "3", "4", "5", "6", "7"]} options={[]} state="unavailable" onChange={vi.fn()} />);
  expect(screen.getByText("建议最多选择 6 个 Tag")).toBeInTheDocument();
  expect(screen.getByText("博客标签暂不可用，仍可手工添加")).toBeInTheDocument();
});
