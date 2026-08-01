import { expect, test } from "vitest";
import { previewHasHeading, sanitizePreviewHTML } from "./safeHTML";

test("文章预览移除脚本、事件属性和危险链接", () => {
  const html = sanitizePreviewHTML('<p onclick="steal()">正文</p><script>steal()</script><a href="javascript:steal()">链接</a>');
  expect(html).toContain("正文");
  expect(html).not.toMatch(/script|onclick|javascript:/i);
});

test("正文存在一级标题时不需要额外显示元数据标题", () => {
  expect(previewHasHeading("<h1>正文标题</h1><p>正文</p>")).toBe(true);
  expect(previewHasHeading("<p>正文</p>")).toBe(false);
});
