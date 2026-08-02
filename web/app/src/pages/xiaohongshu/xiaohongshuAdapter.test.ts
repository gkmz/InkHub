import { describe, expect, it } from "vitest";
import { adaptXiaohongshuHTML } from "./xiaohongshuAdapter";

describe("adaptXiaohongshuHTML", () => {
  it("保留目标视口内的表格", () => {
    const result = adaptXiaohongshuHTML("<table><tr><td>名称</td><td>值</td></tr></table>", 360);
    expect(result.convertedTables).toBe(0);
    expect(result.html).toContain("<table");
  });

  it("将测量后溢出的表格转换为结构化文本", () => {
    const result = adaptXiaohongshuHTML("<table><tr><td>这是一个很长很长很长很长很长很长的列</td><td>第二列</td></tr></table>", 120);
    expect(result.convertedTables).toBe(1);
    expect(result.html).toContain("xiaohongshu-table-text");
    expect(result.html).toContain("｜");
  });
});
