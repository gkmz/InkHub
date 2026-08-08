import { describe, expect, it } from "vitest";
import { adaptXiaohongshuHTML, stripXiaohongshuTitle } from "./xiaohongshuAdapter";

describe("stripXiaohongshuTitle", () => {
  it("移除正文中的首个一级标题", () => {
    const html = stripXiaohongshuTitle("<h1>文章标题</h1><p>正文</p><h1>正文中的其他标题</h1>");
    expect(html).not.toContain("文章标题");
    expect(html).toContain("正文");
    expect(html).toContain("正文中的其他标题");
  });
});

describe("adaptXiaohongshuHTML", () => {
  it("保留目标视口内的表格", () => {
    const result = adaptXiaohongshuHTML("<table><tr><td>名称</td><td>值</td></tr></table>", 360);
    expect(result.convertedTables).toBe(0);
    expect(result.html).toContain("<table");
  });

  it("将测量后溢出的表格转换为结构化卡片", () => {
    const result = adaptXiaohongshuHTML("<table><tr><td>这是一个很长很长很长很长很长很长的列</td><td>第二列</td></tr></table>", 120);
    expect(result.convertedTables).toBe(1);
    expect(result.html).toContain("xiaohongshu-table-card");
    expect(result.html).toContain("xiaohongshu-table-field");
    expect(result.html).toContain("这是一个很长很长很长很长很长很长的列");
    expect(result.html).toContain("第二列");
  });
});
