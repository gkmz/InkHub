import { sanitizePreviewHTML } from "../api/safeHTML";

/** copyFormattedHTML 将安全 HTML 作为富文本写入剪贴板，并兼容受限浏览器环境。 */
export async function copyFormattedHTML(html: string): Promise<void> {
  const safeHTML = sanitizePreviewHTML(html);
  const plainText = htmlToPlainText(safeHTML);

  if (navigator.clipboard?.write && typeof ClipboardItem !== "undefined") {
    try {
      await navigator.clipboard.write([new ClipboardItem({
        "text/html": new Blob([safeHTML], { type: "text/html" }),
        "text/plain": new Blob([plainText], { type: "text/plain" }),
      })]);
      return;
    } catch {
      // 权限策略可能禁止异步 Clipboard API，继续尝试用户点击事件内的选区复制。
    }
  }

  if (copyWithSelection(safeHTML)) return;
  throw new Error("无法写入格式化内容，请使用手工复制");
}

function htmlToPlainText(html: string): string {
  const template = document.createElement("template");
  template.innerHTML = html;
  return template.content.textContent ?? "";
}

function copyWithSelection(html: string): boolean {
  const container = document.createElement("div");
  container.dataset.inkhubClipboard = "true";
  container.contentEditable = "true";
  container.setAttribute("aria-hidden", "true");
  container.style.cssText = "position:fixed;left:-10000px;top:0;width:800px;opacity:0;pointer-events:none;";
  container.innerHTML = html;

  const selection = window.getSelection();
  const previousRanges: Range[] = [];
  if (selection) {
    for (let index = 0; index < selection.rangeCount; index += 1) previousRanges.push(selection.getRangeAt(index).cloneRange());
  }
  const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;

  try {
    document.body.appendChild(container);
    const range = document.createRange();
    range.selectNodeContents(container);
    selection?.removeAllRanges();
    selection?.addRange(range);
    return typeof document.execCommand === "function" && document.execCommand("copy");
  } catch {
    return false;
  } finally {
    container.remove();
    selection?.removeAllRanges();
    previousRanges.forEach((range) => selection?.addRange(range));
    activeElement?.focus({ preventScroll: true });
  }
}
