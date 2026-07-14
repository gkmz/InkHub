import { expect, test } from "vitest";
import { sanitizePreviewHTML } from "./safeHTML";

test("文章预览移除脚本、事件属性和危险链接", () => {
  const html = sanitizePreviewHTML('<p onclick="steal()">正文</p><script>steal()</script><a href="javascript:steal()">链接</a>');
  expect(html).toContain("正文");
  expect(html).not.toMatch(/script|onclick|javascript:/i);
});
