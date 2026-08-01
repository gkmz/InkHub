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
