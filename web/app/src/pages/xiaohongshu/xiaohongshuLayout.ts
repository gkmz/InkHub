import { sanitizePreviewHTML } from "../../api/safeHTML";
import type { XiaohongshuBlock, XiaohongshuPage } from "../../api/types";

export type { XiaohongshuBlock, XiaohongshuPage } from "../../api/types";

/** XiaohongshuBlockKind 表示小红书卡片编辑器支持的正文块类型。 */
export type XiaohongshuBlockKind = XiaohongshuBlock["kind"];

/** XiaohongshuTemplateMetrics 描述模板内容区域的可用尺寸。 */
export interface XiaohongshuTemplateMetrics {
  contentWidth: number;
  contentHeight: number;
  firstPageContentHeight?: number;
}

/** XiaohongshuTemplate 是卡片尺寸和视觉样式的稳定模板定义。 */
export interface XiaohongshuTemplate extends XiaohongshuTemplateMetrics {
  id: string;
  label: string;
  viewportWidth: number;
  pageHeight: number;
  paddingX: number;
  paddingY: number;
  backgroundColor: string;
  textColor: string;
  headingColor: string;
  mutedColor: string;
  accentColor: string;
  secondaryAccentColor: string;
  borderColor: string;
  bodyFontFamily: string;
  headingFontFamily: string;
}

/** XIAOHONGSHU_DEFAULT_TEMPLATE 是唯一对外提供的中文悦读模板。 */
export const XIAOHONGSHU_DEFAULT_TEMPLATE: XiaohongshuTemplate = {
  id: "mobile-clean",
  label: "中文悦读",
  viewportWidth: 375,
  pageHeight: 667,
  paddingX: 22,
  paddingY: 28,
  contentWidth: 331,
  contentHeight: 540,
  firstPageContentHeight: 430,
  backgroundColor: "#f3f6f1",
  textColor: "#26332f",
  headingColor: "#183f35",
  mutedColor: "#68766f",
  accentColor: "#245b4d",
  secondaryAccentColor: "#9a5642",
  borderColor: "#c9d4cc",
  bodyFontFamily: "'Songti SC','STSong','Noto Serif CJK SC','Source Han Serif SC',serif",
  headingFontFamily: "'PingFang SC','Microsoft YaHei','Noto Sans CJK SC',sans-serif",
};

/** XIAOHONGSHU_TEMPLATES 是预览、分页和导出共同使用的模板注册表。 */
export const XIAOHONGSHU_TEMPLATES: XiaohongshuTemplate[] = [
  XIAOHONGSHU_DEFAULT_TEMPLATE,
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

/** adaptTablesForXiaohongshu 将超出卡片宽度的表格按数据行转换为移动端信息块。 */
export function adaptTablesForXiaohongshu(blocks: XiaohongshuBlock[], metrics: XiaohongshuTemplateMetrics): XiaohongshuBlock[] {
  return blocks.flatMap((block) => {
    if (block.kind !== "table" || estimateTableWidth(block.html) <= metrics.contentWidth) return [block];
    const document = new DOMParser().parseFromString(block.html, "text/html");
    const table = document.querySelector("table");
    if (!table) return [block];
    const cards = renderXiaohongshuTableCards(table);
    if (cards.length === 0) return [block];
    // 每一行独立成块，分页时不会把整张长表强行缩进同一张图片。
    return cards.map((html, index) => ({
      ...block,
      id: `${block.id}-row-${index + 1}`,
      kind: "text" as const,
      html,
      splittable: false,
    }));
  });
}

/** renderXiaohongshuTableCards 将表格数据行渲染为适合手机窄屏阅读的字段卡片。 */
export function renderXiaohongshuTableCards(table: HTMLTableElement): string[] {
  const rows = Array.from(table.querySelectorAll("tr"));
  if (rows.length === 0) return [];
  const headerRow = rows.find((row) => row.querySelector("th"));
  const headers = headerRow
    ? Array.from(headerRow.querySelectorAll("th,td")).map((cell, index) => cell.textContent?.trim() || `第 ${index + 1} 列`)
    : [];
  const dataRows = headerRow ? rows.filter((row) => row !== headerRow) : rows;
  return dataRows.map((row, rowIndex) => {
    const values = Array.from(row.querySelectorAll("th,td")).map((cell) => cell.textContent?.trim() ?? "");
    const fields = values.map((value, columnIndex) => {
      const label = headers[columnIndex] || `第 ${columnIndex + 1} 列`;
      return `<div class="xiaohongshu-table-field"><strong>${escapeHTML(label)}</strong><span>${escapeHTML(value || "-")}</span></div>`;
    }).join("");
    return `<section class="xiaohongshu-table-card" aria-label="表格第 ${rowIndex + 1} 行">${fields}</section>`;
  }).filter((html) => html.includes("xiaohongshu-table-field"));
}

/** paginateXiaohongshuBlocks 按模板高度分页，并保持不可拆分块的完整性。 */
export function paginateXiaohongshuBlocks(blocks: XiaohongshuBlock[], metrics: XiaohongshuTemplateMetrics, measure: BlockMeasure): XiaohongshuPage[] {
  const pages: XiaohongshuPage[] = [];
  let current: XiaohongshuPage = createPage(0);
  for (const block of blocks) {
    const height = Math.max(1, measure(block, metrics.contentWidth));
    const pageHeight = pages.length === 0 ? metrics.firstPageContentHeight ?? metrics.contentHeight : metrics.contentHeight;
    if (current.blocks.length > 0 && current.measured_height + height > pageHeight) {
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
  // 中文宋体正文接近全角字宽，按 16px 字格估算，避免沿用英文密度后把过多段落塞进一页。
  const charsPerLine = Math.max(12, Math.floor(contentWidth / 16));
  const lines = Math.max(1, Math.ceil(textLength / charsPerLine));
  const images = document.querySelectorAll("img");
  if (images.length > 0) {
    // 图片所在段落可能同时带正文，估算时两部分都计入，避免首页加载图片后被整体缩小。
    return images.length * 170 + lines * 27 + 12;
  }
  if (document.querySelector("pre > code.language-mermaid, pre > code.lang-mermaid")) {
    // Mermaid 源码很短但生成的图表较高，需要按图形而不是源码行数预留空间。
    return 240;
  }
  const tableCard = document.querySelector(".xiaohongshu-table-card");
  if (tableCard) {
    // 字段卡片包含标签、值和分组间距，不能沿用普通段落的紧凑估算。
    const fields = Array.from(tableCard.querySelectorAll(".xiaohongshu-table-field"));
    return 24 + fields.reduce((height, field) => {
      const valueLength = field.querySelector("span")?.textContent?.trim().length ?? 0;
      return height + 24 + Math.max(26, Math.ceil(valueLength / charsPerLine) * 26);
    }, 0);
  }
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
