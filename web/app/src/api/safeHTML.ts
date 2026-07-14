import DOMPurify from "dompurify";

/** sanitizePreviewHTML 在浏览器渲染前移除可执行内容和危险 URL。 */
export function sanitizePreviewHTML(value: string) {
  return DOMPurify.sanitize(value, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ["style", "form", "input", "button"],
    FORBID_ATTR: ["style"],
  });
}
