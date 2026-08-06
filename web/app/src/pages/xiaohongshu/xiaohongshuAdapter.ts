import { sanitizePreviewHTML } from "../../api/safeHTML";
import { renderMermaidSVG } from "../../platform/mermaid";
import { renderXiaohongshuTableCards } from "./xiaohongshuLayout";

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

/** renderXiaohongshuMermaidImages 将 Mermaid 代码块转换为可预览、可导出的图片。 */
export async function renderXiaohongshuMermaidImages(html: string): Promise<string> {
  const safe = sanitizePreviewHTML(html);
  const document = new DOMParser().parseFromString(`<div>${safe}</div>`, "text/html");
  const root = document.body.firstElementChild;
  if (!root) return safe;
  const diagrams = Array.from(root.querySelectorAll<HTMLElement>("pre > code.language-mermaid, pre > code.lang-mermaid"));
  for (const code of diagrams) {
    const source = code.textContent?.trim() ?? "";
    if (!source || !code.parentElement) continue;
    try {
      const wrapper = document.createElement("div");
      wrapper.className = "xiaohongshu-mermaid-image";
      wrapper.setAttribute("role", "img");
      wrapper.setAttribute("aria-label", "Mermaid 图表");
      // 小红书导出链路需要稳定的纯 SVG；手绘模式在部分浏览器中会触发图片解码异常。
      wrapper.innerHTML = await renderMermaidSVG(source, "modern");
      code.parentElement.replaceWith(wrapper);
    } catch (error) {
      // 单张图表失败时保留源码，避免吞掉用户原始内容。
      console.warn("小红书 Mermaid 图片生成失败", error);
    }
  }
  // 输入已先完成 HTML 清理，新增内容只来自 Mermaid strict 模式生成的 SVG。
  return root.innerHTML;
}

/** inlineXiaohongshuImages 将页面图片转成 data URL，确保导出 PNG 不依赖临时资源地址。 */
export async function inlineXiaohongshuImages(html: string): Promise<string> {
  const document = new DOMParser().parseFromString(`<div>${html}</div>`, "text/html");
  const root = document.body.firstElementChild;
  if (!root) return html;
  for (const image of root.querySelectorAll<HTMLImageElement>("img[src]")) {
    if (image.src.startsWith("data:")) continue;
    const response = await fetch(image.getAttribute("src") ?? "", { credentials: "same-origin" });
    if (!response.ok) throw new Error(`图片加载失败（${response.status}）`);
    image.src = await blobToDataURL(await response.blob());
  }
  return root.innerHTML;
}

function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(new Error("图片编码失败"));
    reader.readAsDataURL(blob);
  });
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
    const cards = renderXiaohongshuTableCards(table);
    const replacement = document.createElement("div");
    replacement.className = "xiaohongshu-table-cards";
    replacement.innerHTML = cards.join("");
    table.replaceWith(replacement);
    convertedTables += 1;
  }
  return { html: root.innerHTML, convertedTables };
}
