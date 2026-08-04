import { sanitizePreviewHTML } from "../../api/safeHTML";

/** XiaohongshuHTMLAdaptation 描述一次导出前的浏览器测量和表格降级结果。 */
export interface XiaohongshuHTMLAdaptation {
  html: string;
  convertedTables: number;
}

/** stripXiaohongshuTitle 移除正文中的首个一级标题，避免与手机模板标题重复。 */
export function stripXiaohongshuTitle(html: string): string {
  const safe = sanitizePreviewHTML(html);
  const document = new DOMParser().parseFromString(`<div>${safe}</div>`, "text/html");
  const root = document.body.firstElementChild;
  if (!root) return safe;
  root.querySelector("h1")?.remove();
  return root.innerHTML;
}

/** adaptXiaohongshuHTML 在目标手机宽度中测量表格，超出时转换为结构化文本。 */
export function adaptXiaohongshuHTML(html: string, viewportWidth: number): XiaohongshuHTMLAdaptation {
  const safe = stripXiaohongshuTitle(html);
  const document = new DOMParser().parseFromString(`<div>${safe}</div>`, "text/html");
  const root = document.body.firstElementChild;
  if (!root) return { html: safe, convertedTables: 0 };
  let convertedTables = 0;
  const tables = Array.from(root.querySelectorAll("table"));
  for (const table of tables) {
    const host = document.createElement("div");
    host.style.cssText = `position:absolute;left:-100000px;width:${Math.max(1, viewportWidth)}px;visibility:hidden;`;
    const clone = table.cloneNode(true) as HTMLTableElement;
    host.appendChild(clone); document.body.appendChild(host);
    const estimatedWidth = Array.from(clone.querySelectorAll("th,td")).reduce((total, cell) => total + (cell.textContent?.trim().length ?? 0) * 8 + 24, 0);
    const overflow = clone.scrollWidth > viewportWidth || clone.getBoundingClientRect().width > viewportWidth || estimatedWidth > viewportWidth;
    host.remove();
    if (!overflow) continue;
    const rows = Array.from(table.querySelectorAll("tr"));
    const text = document.createElement("div");
    text.className = "xiaohongshu-table-text";
    text.textContent = rows.map((row) => Array.from(row.querySelectorAll("th,td")).map((cell) => cell.textContent?.trim() ?? "").filter(Boolean).join(" ｜ ")).filter(Boolean).join("\n");
    table.replaceWith(text);
    convertedTables += 1;
  }
  return { html: root.innerHTML, convertedTables };
}
