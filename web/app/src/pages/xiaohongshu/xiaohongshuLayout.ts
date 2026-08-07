import { sanitizePreviewHTML } from "../../api/safeHTML";
import type { XiaohongshuBlock, XiaohongshuPage } from "../../api/types";
import { TOKYO_NIGHT_CODE_THEME, type XiaohongshuCodeTheme } from "./xiaohongshuCodeTheme";

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
  codeTheme: XiaohongshuCodeTheme;
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
  contentHeight: 560,
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
  codeTheme: TOKYO_NIGHT_CODE_THEME,
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
  const pending = [...blocks];
  while (pending.length > 0) {
    const block = pending.shift();
    if (!block) continue;
    const height = Math.max(1, measure(block, metrics.contentWidth));
    const pageHeight = pages.length === 0 ? metrics.firstPageContentHeight ?? metrics.contentHeight : metrics.contentHeight;
    const remainingHeight = pageHeight - current.measured_height;
    const availableHeight = current.blocks.length > 0 ? remainingHeight : pageHeight;
    if (height > availableHeight && availableHeight >= 72) {
      const parts = splitXiaohongshuBlock(block, metrics.contentWidth, availableHeight, measure);
      if (parts.length > 1) {
        // 先把当前页能容纳的部分放回队列，后续部分自然进入下一页继续排版。
        pending.unshift(...parts);
        continue;
      }
    }
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

/** splitXiaohongshuBlock 按句末边界拆分正文块，减少段落整体换页造成的空白。 */
function splitXiaohongshuBlock(block: XiaohongshuBlock, contentWidth: number, availableHeight: number, measure: BlockMeasure): XiaohongshuBlock[] {
  if (!block.splittable || block.kind === "image" || block.kind === "code" || block.kind === "table") return [block];
  const parsed = new DOMParser().parseFromString(`<div>${block.html}</div>`, "text/html");
  const root = parsed.body.firstElementChild?.firstElementChild;
  if (!root || root.querySelector("img,pre,table")) return [block];
  if (/^(UL|OL)$/.test(root.tagName)) return splitXiaohongshuListBlock(root, block, contentWidth, availableHeight, measure);
  const text = root.textContent ?? "";
  if (text.trim().length < 2) return [block];

  const charsPerLine = Math.max(12, Math.floor(contentWidth / 16));
  const sentenceEnds = findSentenceEnds(text);
  const end = chooseSplitEnd(root, block, text, 0, sentenceEnds, charsPerLine, contentWidth, availableHeight, measure);
  if (end <= 0 || end >= text.length) return [block];
  const head = cloneElementTextRange(root, 0, end);
  const tail = cloneElementTextRange(root, end, text.length);
  if (!head || !tail) return [block];
  // 每次只切出当前页可容纳的头部，剩余内容到下一页后会按完整页高重新计算。
  return [
    { ...block, id: `${block.id}-head`, html: head.outerHTML },
    { ...block, id: `${block.id}-tail`, html: tail.outerHTML },
  ];
}

/** splitXiaohongshuListBlock 只在列表项之间分页，避免产生断词和重复的残缺项目符号。 */
function splitXiaohongshuListBlock(root: Element, block: XiaohongshuBlock, contentWidth: number, availableHeight: number, measure: BlockMeasure): XiaohongshuBlock[] {
  const items = Array.from(root.children).filter((child) => child.tagName === "LI");
  if (items.length < 2) return [block];
  let splitIndex = 0;
  for (let index = 1; index < items.length; index += 1) {
    const head = cloneListRange(root, items.slice(0, index));
    if (measure({ ...block, html: head.outerHTML }, contentWidth) <= availableHeight) splitIndex = index;
    else break;
  }
  if (splitIndex <= 0 || splitIndex >= items.length) return [block];
  const head = cloneListRange(root, items.slice(0, splitIndex));
  const tail = cloneListRange(root, items.slice(splitIndex));
  if (root.tagName === "OL" && !tail.hasAttribute("start") && !tail.hasAttribute("reversed")) {
    const originalStart = Number.parseInt(root.getAttribute("start") ?? "1", 10) || 1;
    tail.setAttribute("start", String(originalStart + splitIndex));
  }
  return [
    { ...block, id: `${block.id}-head`, html: head.outerHTML },
    { ...block, id: `${block.id}-tail`, html: tail.outerHTML },
  ];
}

function cloneListRange(root: Element, items: Element[]): Element {
  const clone = root.cloneNode(false) as Element;
  items.forEach((item) => clone.appendChild(item.cloneNode(true)));
  return clone;
}

/** chooseSplitEnd 选择当前页能容纳的最长句子，必要时才退化为字符边界。 */
function chooseSplitEnd(root: Element, block: XiaohongshuBlock, text: string, start: number, sentenceEnds: number[], charsPerLine: number, contentWidth: number, availableHeight: number, measure: BlockMeasure): number {
  let best = start;
  for (const end of sentenceEnds) {
    if (end <= start) continue;
    const fragment = cloneElementTextRange(root, start, end);
    if (!fragment) continue;
    const candidate: XiaohongshuBlock = { ...block, html: fragment.outerHTML };
    if (measure(candidate, contentWidth) <= availableHeight) best = end;
    else if (best > start) break;
  }
  if (best > start) return best;

  // 极长且没有标点的段落按估算行数切开，并向后回退到可容纳的位置。
  let fallback = Math.min(text.length, start + Math.max(1, Math.floor(Math.max(1, availableHeight - 12) / 27) * charsPerLine));
  while (fallback > start) {
    const fragment = cloneElementTextRange(root, start, fallback);
    if (fragment && measure({ ...block, html: fragment.outerHTML }, contentWidth) <= availableHeight) return fallback;
    fallback -= charsPerLine;
  }
  return Math.min(text.length, start + 1);
}

/** findSentenceEnds 返回中英文常见句末标点后的文本位置。 */
function findSentenceEnds(text: string): number[] {
  const ends: number[] = [];
  const pattern = /[。！？；;.!?](?:["'”’）)\]]*)/g;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(text)) !== null) ends.push(match.index + match[0].length);
  if (ends[ends.length - 1] !== text.length) ends.push(text.length);
  return ends;
}

/** cloneElementTextRange 保留原有行内标签，只截取指定文本区间。 */
function cloneElementTextRange(root: Element, start: number, end: number): Element | null {
  const cursor = { value: 0 };
  const cloned = cloneNodeTextRange(root, start, end, cursor);
  return cloned instanceof Element ? cloned : null;
}

function cloneNodeTextRange(node: Node, start: number, end: number, cursor: { value: number }): Node | null {
  if (node.nodeType === Node.TEXT_NODE) {
    const value = node.textContent ?? "";
    const nodeStart = cursor.value;
    cursor.value += value.length;
    const from = Math.max(0, start - nodeStart);
    const to = Math.min(value.length, end - nodeStart);
    return to > from ? node.ownerDocument?.createTextNode(value.slice(from, to)) ?? null : null;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) return null;
  const clone = node.cloneNode(false) as Element;
  for (const child of Array.from(node.childNodes)) {
    const childClone = cloneNodeTextRange(child, start, end, cursor);
    if (childClone) clone.appendChild(childClone);
  }
  return clone.childNodes.length > 0 ? clone : null;
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
    // 小红书横向流程图会自适应卡片宽度，按图形容器高度预留，避免把源码长度当作图表高度。
    return 70;
  }
  // 普通正文使用与预览相同的 CSS 在隐藏容器中测量，避免中英文混排、标题和列表继续依赖字符数猜测。
  const renderedHeight = measureXiaohongshuBlockInDOM(block, contentWidth);
  if (renderedHeight !== null) return renderedHeight;
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

/** measureXiaohongshuBlockInDOM 使用真实字体和样式测量单个正文块的有效高度。 */
function measureXiaohongshuBlockInDOM(block: XiaohongshuBlock, contentWidth: number): number | null {
  if (typeof window === "undefined" || typeof window.getComputedStyle !== "function" || !document.body) return null;
  const host = document.createElement("div");
  host.style.cssText = "position:fixed;left:-100000px;top:0;width:0;height:0;overflow:visible;visibility:hidden;pointer-events:none;";
  const content = document.createElement("div");
  content.className = "xiaohongshu-card-content";
  content.style.width = `${contentWidth}px`;
  content.innerHTML = block.html;
  host.appendChild(content);
  document.body.appendChild(host);
  try {
    const element = content.firstElementChild;
    if (!(element instanceof HTMLElement)) return null;
    const style = window.getComputedStyle(element);
    const marginBottom = Number.parseFloat(style.marginBottom) || 0;
    // 相邻块的上下边距会在真实文档流中折叠；这里只计下边距，模板预留空间负责吸收剩余差值。
    const height = element.getBoundingClientRect().height + marginBottom;
    return height > 0 ? Math.round(height * 10) / 10 : null;
  } finally {
    host.remove();
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
