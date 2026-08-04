import { describe, expect, it } from "vitest";
import { adaptTablesForXiaohongshu, paginateXiaohongshuBlocks, parseXiaohongshuBlocks } from "./xiaohongshuLayout";

describe("parseXiaohongshuBlocks", () => {
  it("把正文解析为标题、图片、代码和表格块", () => {
    const blocks = parseXiaohongshuBlocks("<h2>标题</h2><p>正文</p><img src=\"cover.png\"><pre><code>go test</code></pre><table><tr><td>A</td></tr></table>");
    expect(blocks.map((block) => block.kind)).toEqual(["heading", "paragraph", "image", "code", "table"]);
  });
});

describe("paginateXiaohongshuBlocks", () => {
  it("代码块放不下时整体移动到下一页", () => {
    const pages = paginateXiaohongshuBlocks([
      { id: "p1", kind: "paragraph", html: "<p>正文</p>", splittable: true },
      { id: "c1", kind: "code", html: "<pre><code>go test</code></pre>", splittable: false },
    ], { contentWidth: 320, contentHeight: 200 }, (item) => item.kind === "paragraph" ? 80 : 160);
    expect(pages.map((page) => page.blocks.map((item) => item.kind))).toEqual([["paragraph"], ["code"]]);
  });
});

describe("adaptTablesForXiaohongshu", () => {
  it("超宽表格转换成文本块", () => {
    const blocks = adaptTablesForXiaohongshu(parseXiaohongshuBlocks("<table><tr><td>这是一个非常非常非常非常非常非常非常非常非常非常长的字段</td><td>第二个超长字段</td></tr></table>"), { contentWidth: 320, contentHeight: 500 });
    expect(blocks[0].kind).toBe("text");
    expect(blocks[0].html).toContain("非常非常");
  });
});
