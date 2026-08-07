import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { XiaohongshuStoryboardEditor } from "./XiaohongshuStoryboardEditor";
import { formatXiaohongshuStoryboard } from "./xiaohongshuStoryboard";

describe("XiaohongshuStoryboardEditor", () => {
  it("允许编辑分镜提示词并复制当前页", () => {
    const onPagesChange = vi.fn();
    const onCopy = vi.fn();
    const pages = [{ id: "page-1", title: "封面", prompt: "生成封面" }];
    render(<XiaohongshuStoryboardEditor pages={pages} onPagesChange={onPagesChange} onCopy={onCopy} />);

    fireEvent.change(screen.getByRole("textbox", { name: "生图提示词" }), { target: { value: "新的提示词" } });
    expect(onPagesChange).toHaveBeenCalledWith([{ ...pages[0], prompt: "新的提示词" }]);

    fireEvent.click(screen.getByRole("button", { name: "复制本页" }));
    expect(onCopy).toHaveBeenCalledWith("生成封面", "已复制第 1 页提示词");
  });

  it("将多页分镜格式化为可复制脚本", () => {
    expect(formatXiaohongshuStoryboard([
      { id: "page-1", title: "封面", prompt: "提示词一" },
      { id: "page-2", title: "原理", prompt: "提示词二" },
    ])).toContain("第 2 页：原理\n\n提示词二");
  });
});
