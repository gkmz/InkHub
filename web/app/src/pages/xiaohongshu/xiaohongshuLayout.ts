import { sanitizePreviewHTML } from "../../api/safeHTML";
import type { XiaohongshuBlock, XiaohongshuPage } from "../../api/types";

export type { XiaohongshuBlock, XiaohongshuPage } from "../../api/types";

/** XiaohongshuBlockKind 表示小红书卡片编辑器支持的正文块类型。 */
export type XiaohongshuBlockKind = XiaohongshuBlock["kind"];

/** XiaohongshuTemplateMetrics 描述模板内容区域的可用尺寸。 */
export interface XiaohongshuTemplateMetrics {
  contentWidth: number;
  contentHeight: number;
}

/** XiaohongshuTemplate 是卡片尺寸和视觉样式的稳定模板定义。 */
export interface XiaohongshuTemplate extends XiaohongshuTemplateMetrics {
  id: string;
  label: string;
  viewportWidth: number;
  pageHeight: number;
  paddingX: number;
  paddingY: number;
}

/** XIAOHONGSHU_TEMPLATES 是预览、分页和导出共同使用的模板注册表。 */
export const XIAOHONGSHU_TEMPLATES: XiaohongshuTemplate[] = [
  { id: "mobile-clean", label: "Mobile Clean", viewportWidth: 375, pageHeight: 667, paddingX: 22, paddingY: 28, contentWidth: 331, contentHeight: 611 },
  { id: "mobile-paper", label: "Mobile Paper", viewportWidth: 375, pageHeight: 667, paddingX: 22, paddingY: 28, contentWidth: 331, contentHeight: 611 },
];

/** BlockMeasure 在指定宽度中返回正文块的渲染高度。 */
export type BlockMeasure = (block: XiaohongshuBlock, contentWidth: number) => number;

/** parseXiaohongshuBlocks 将安全 HTML 拆解为稳定顺序的内容块。 */
export function parseXiaohongshuBlocks(html: string): XiaohongshuBlock[] {
  const safe = sanitizePreviewHTML(html);
  const document = new DOMParser().parseFromString(`<div>${safe}</div>`, "text/html");
  const root = document.body.firstElementChild;
  if (!root) return [];
  const elements = Array.from(root.children);
  if (elements.length === 0 && root.textContent?.trim()) {
    return [createBlock(0, "paragraph", `<p>${escapeHTML(root.textContent.trim())}</p>`, true)];
  }
  return elements.map((element, index) => createBlock(index, classifyElement(element), element.outerHTML, isSplittable(element)));
}

/** adaptTablesForXiaohongshu 将超出卡片宽度的表格转换为可读文本块。 */
export function adaptTablesForXiaohongshu(blocks: XiaohongshuBlock[], metrics: XiaohongshuTemplateMetrics): XiaohongshuBlock[] {
  return blocks.map((block) => {
    if (block.kind !== "table" || estimateTableWidth(block.html) <= metrics.contentWidth) return block;
    const document = new DOMParser().parseFromString(block.html, "text/html");
    const table = document.querySelector("table");
    if (!table) return block;
    const rows = Array.from(table.querySelectorAll("tr"))
      .map((row) => Array.from(row.querySelectorAll("th,td")).map((cell) => cell.textContent?.trim() ?? "").filter(Boolean).join(" ｜ "))
      .filter(Boolean);
    const text = rows.join("\n");
    return { ...block, kind: "text", html: `<p>${escapeHTML(text)}</p>`, splittable: true };
  });
}

/** paginateXiaohongshuBlocks 按模板高度分页，并保持不可拆分块的完整性。 */
export function paginateXiaohongshuBlocks(blocks: XiaohongshuBlock[], metrics: XiaohongshuTemplateMetrics, measure: BlockMeasure): XiaohongshuPage[] {
  const pages: XiaohongshuPage[] = [];
  let current: XiaohongshuPage = createPage(0);
  for (const block of blocks) {
    const height = Math.max(1, measure(block, metrics.contentWidth));
    if (current.blocks.length > 0 && current.measured_height + height > metrics.contentHeight) {
      pages.push(current);
      current = createPage(pages.length);
    }
    current.blocks.push(block);
    current.measured_height += height;
  }
  if (current.blocks.length > 0 || pages.length === 0) pages.push(current);
  return pages;
}

/** getXiaohongshuTemplate 返回指定模板，不认识的模板回退到默认模板。 */
export function getXiaohongshuTemplate(templateID: string): XiaohongshuTemplate {
  return XIAOHONGSHU_TEMPLATES.find((template) => template.id === templateID) ?? XIAOHONGSHU_TEMPLATES[0];
}

/** buildXiaohongshuPages 将旧正文转换为可持久化的分页卡片。 */
export function buildXiaohongshuPages(html: string, templateID: string): XiaohongshuPage[] {
  const template = getXiaohongshuTemplate(templateID);
  const blocks = adaptTablesForXiaohongshu(parseXiaohongshuBlocks(html), template);
  return paginateXiaohongshuBlocks(blocks, template, estimateXiaohongshuBlockHeight);
}

/** flattenXiaohongshuPages 将页面块重新组合为兼容旧接口的正文 HTML。 */
export function flattenXiaohongshuPages(pages: XiaohongshuPage[]): string {
  return sanitizePreviewHTML(pages.flatMap((page) => page.blocks.map((block) => block.html)).join(""));
}

/** estimateXiaohongshuBlockHeight 在真实 DOM 测量不可用时提供稳定的分页估算。 */
export function estimateXiaohongshuBlockHeight(block: XiaohongshuBlock, contentWidth: number): number {
  const document = new DOMParser().parseFromString(block.html, "text/html");
  const textLength = document.body.textContent?.trim().length ?? 0;
  const charsPerLine = Math.max(12, Math.floor(contentWidth / 8));
  const lines = Math.max(1, Math.ceil(textLength / charsPerLine));
  switch (block.kind) {
    case "image": return 190;
    case "code": return Math.max(72, lines * 19 + 24);
    case "table": return Math.max(54, document.querySelectorAll("tr").length * 32 + 18);
    case "heading": return 46;
    default: return lines * 27 + 12;
  }
}

function createBlock(index: number, kind: XiaohongshuBlockKind, html: string, splittable: boolean): XiaohongshuBlock {
  return { id: `xhs-block-${index + 1}`, kind, html, splittable };
}

function createPage(index: number): XiaohongshuPage {
  return { id: `xhs-page-${index + 1}`, blocks: [], measured_height: 0 };
}

function classifyElement(element: Element): XiaohongshuBlockKind {
  const tagName = element.tagName.toLowerCase();
  if (tagName === "img" || element.querySelector("img")) return "image";
  if (tagName === "pre" || element.querySelector("pre")) return "code";
  if (tagName === "table" || element.querySelector("table")) return "table";
  if (/^h[1-6]$/.test(tagName)) return "heading";
  if (["p", "blockquote", "ul", "ol", "li"].includes(tagName)) return "paragraph";
  return "text";
}

function isSplittable(element: Element): boolean {
  return ["p", "blockquote", "ul", "ol", "li", "div"].includes(element.tagName.toLowerCase());
}

function estimateTableWidth(html: string): number {
  const document = new DOMParser().parseFromString(html, "text/html");
  return Array.from(document.querySelectorAll("th,td")).reduce((total, cell) => total + (cell.textContent?.trim().length ?? 0) * 8 + 24, 0);
}

function escapeHTML(value: string): string {
  return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char] ?? char));
}
