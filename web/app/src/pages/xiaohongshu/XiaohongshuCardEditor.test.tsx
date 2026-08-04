import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { XiaohongshuPage } from "../../api/types";
import { XiaohongshuCardEditor } from "./XiaohongshuCardEditor";

function page(id: string): XiaohongshuPage {
  return { id, measured_height: 320, blocks: [{ id: `${id}-block`, kind: "paragraph", html: `<p>第 ${id} 页</p>`, splittable: true }] };
}

describe("XiaohongshuCardEditor", () => {
  it("按页码渲染卡片并支持左右滚动", () => {
    render(<XiaohongshuCardEditor pages={[page("1"), page("2")]} template="mobile-clean" onPagesChange={vi.fn()} onSelectionChange={vi.fn()} />);
    expect(screen.getByLabelText("第 1 页，共 2 页")).toBeInTheDocument();
    expect(screen.getByLabelText("第 2 页，共 2 页")).toBeInTheDocument();
  });

  it("编辑卡片正文时只更新当前页面", async () => {
    const onPagesChange = vi.fn();
    render(<XiaohongshuCardEditor pages={[page("1"), page("2")]} template="mobile-clean" onPagesChange={onPagesChange} onSelectionChange={vi.fn()} />);
    await userEvent.click(screen.getByRole("textbox", { name: "第 1 页正文" }));
    await userEvent.type(screen.getByRole("textbox", { name: "第 1 页正文" }), "新增");
    const latestPages = onPagesChange.mock.lastCall?.[0] as XiaohongshuPage[];
    expect(latestPages[0].blocks[0].html).toContain("新增");
    expect(latestPages[1].id).toBe("2");
  });
});
