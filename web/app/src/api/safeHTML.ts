import DOMPurify from "dompurify";

/** sanitizePreviewHTML 在浏览器渲染前移除可执行内容和危险 URL。 */
export function sanitizePreviewHTML(value: string) {
  return DOMPurify.sanitize(value, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ["style", "form", "input", "button"],
    // 后端只输出经过 Goldmark/模板清理的样式：代码高亮和微信模板都依赖它。
    FORBID_ATTR: [],
  });
}

/** previewHasHeading 判断 Markdown 预览是否已经包含正文一级标题。 */
export function previewHasHeading(value: string) {
  const document = new DOMParser().parseFromString(sanitizePreviewHTML(value), "text/html");
  return document.querySelector("h1") !== null;
}
