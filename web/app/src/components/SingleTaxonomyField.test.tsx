import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { SingleTaxonomyField } from "./SingleTaxonomyField";

test("单值 Taxonomy 字段保留旧值并按名称去重候选", () => {
  render(<SingleTaxonomyField label="Series" noun="系列" value="旧系列" options={[{ key: "course", name: "课程" }, { key: "course-copy", name: "课程" }]} state="ready" emptyLabel="无系列" canCreate={false} onChange={vi.fn()} />);
  expect(screen.getByRole("combobox", { name: "Series" })).toHaveValue("旧系列");
  expect(screen.getByRole("option", { name: "旧系列（博客中未发现）" })).toBeInTheDocument();
  expect(screen.getAllByRole("option", { name: "课程" })).toHaveLength(1);
});

test("单值 Taxonomy 字段支持选择和创建回调", async () => {
  const change = vi.fn();
  const create = vi.fn();
  render(<SingleTaxonomyField label="Series" noun="系列" value="" options={[{ key: "course", name: "课程" }]} state="ready" emptyLabel="无系列" canCreate onChange={change} onCreate={create} />);
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Series" }), "课程");
  expect(change).toHaveBeenCalledWith("课程");
  await userEvent.click(screen.getByRole("button", { name: "新建系列" }));
  expect(create).toHaveBeenCalledOnce();
  expect(screen.getByText("来自当前博客系列")).toBeInTheDocument();
});
