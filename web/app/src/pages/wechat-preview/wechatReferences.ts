interface WeChatReference {
  index: number;
  text: string;
  href: string;
}

/** formatWeChatReferences 修正微信行内代码，并将正文链接转换为上标和文末引用。 */
export function formatWeChatReferences(value: string) {
  if (!value || typeof DOMParser === "undefined") return value;
  const document = new DOMParser().parseFromString(value, "text/html");
  normalizeInlineCode(document);
  normalizeImages(document);
  normalizeReferenceLabels(document);
  if (hasReferenceSection(document)) return document.body.innerHTML;

  const references: WeChatReference[] = [];
  for (const link of document.body.querySelectorAll<HTMLAnchorElement>("a[href]")) {
    if (!isReferenceLink(link)) continue;
    const href = link.getAttribute("href")?.trim() ?? "";
    const reference = { index: references.length + 1, text: link.textContent?.trim() || href, href };
    references.push(reference);

    // 正文只保留可读标签，链接目标统一移动到文末引用章节。
    link.removeAttribute("href");
    const index = document.createElement("sup");
    index.textContent = `[${reference.index}]`;
    index.setAttribute("style", "margin-left:3px;color:#42b883;font-size:0.72em;font-weight:700;line-height:0;vertical-align:super");
    link.after(index);
  }
  if (references.length > 0) document.body.append(createReferenceSection(document, references));
  return document.body.innerHTML;
}

function normalizeImages(document: Document) {
  for (const image of document.body.querySelectorAll<HTMLImageElement>("img")) {
    // 微信编辑器可能保留上游浮动规则，图片必须以内联样式明确独占一行并居中。
    image.style.display = "block";
    image.style.float = "none";
    image.style.clear = "both";
    image.style.maxWidth = "100%";
    image.style.height = "auto";
    image.style.marginLeft = "auto";
    image.style.marginRight = "auto";
  }
}

function normalizeInlineCode(document: Document) {
  for (const code of document.body.querySelectorAll<HTMLElement>("code")) {
    if (code.closest("pre")) continue;
    // 兼容已经生成的旧产物：移除误继承的代码块属性，并恢复随上下文继承的字号和行高。
    code.style.backgroundColor = "#f0f4f8";
    code.style.borderRadius = "5px";
    code.style.color = "#c7522a";
    code.style.display = "inline";
    code.style.fontFamily = "system-monospace";
    code.style.fontSize = "1em";
    code.style.padding = "2px 7px";
    code.style.whiteSpace = "pre-wrap";
    code.style.wordBreak = "break-word";
    code.style.removeProperty("line-height");
    code.style.removeProperty("margin");
  }
}

function normalizeReferenceLabels(document: Document) {
  for (const section of referenceSections(document)) {
    for (const item of section.querySelectorAll("li")) {
      const label = item.querySelector<HTMLElement>(":scope > span:first-child");
      const value = label?.textContent?.trim() ?? "";
      if (!label || !/^\[\d+\]$/.test(value)) continue;
      const titleNode = label.nextSibling;
      if (!titleNode || titleNode.nodeType !== 3) continue;

      // 编号与标题必须位于同一个不可换行容器，单独放置 NBSP 仍可能在节点边界换行。
      const prefix = document.createElement("span");
      prefix.style.whiteSpace = "nowrap";
      item.insertBefore(prefix, label);
      prefix.append(label, document.createTextNode(`\u00a0${titleNode.textContent?.trimStart() ?? ""}`));
      titleNode.remove();
      label.textContent = value;
      label.style.removeProperty("margin-right");
      label.style.whiteSpace = "nowrap";
    }
  }
}

function hasReferenceSection(document: Document) {
  return referenceSections(document).length > 0;
}

function referenceSections(document: Document) {
  return Array.from(document.body.querySelectorAll<HTMLElement>("section")).filter((section) => section.querySelector(":scope > h3")?.textContent?.trim() === "引用链接");
}

function isReferenceLink(link: HTMLAnchorElement) {
  const href = link.getAttribute("href")?.trim() ?? "";
  const normalized = href.toLowerCase();
  return href !== "" && !href.startsWith("#") && !normalized.startsWith("javascript:") && !link.querySelector("img") && !link.closest("pre, code");
}

function createReferenceSection(document: Document, references: WeChatReference[]) {
  const section = document.createElement("section");
  section.setAttribute("style", "margin-top:38px;padding:12px 16px;background-color:#f7fcf9;border-left:4px solid #42b883;border-radius:0 8px 8px 0");
  const title = document.createElement("h3");
  title.textContent = "引用链接";
  title.setAttribute("style", "margin:0 0 10px;padding:0;border:0;color:#1a2733;font-size:16px;font-weight:600");
  section.append(title);

  const list = document.createElement("ul");
  list.setAttribute("style", "margin:0;padding-left:0;list-style-type:none");
  for (const reference of references) list.append(createReferenceItem(document, reference));
  section.append(list);
  return section;
}

function createReferenceItem(document: Document, reference: WeChatReference) {
  const item = document.createElement("li");
  item.setAttribute("style", "display:block;margin:7px 0;color:#5c6975;font-size:14px;line-height:1.55;word-break:break-word");
  const prefix = document.createElement("span");
  prefix.setAttribute("style", "white-space:nowrap");
  const label = document.createElement("span");
  label.textContent = `[${reference.index}]`;
  label.setAttribute("style", "color:#42b883;font-weight:700;white-space:nowrap");
  prefix.append(label, `\u00a0${reference.text}: `);
  const url = document.createElement("span");
  url.textContent = reference.href;
  url.setAttribute("style", "color:#34495e;word-break:break-all");
  item.append(prefix, url);
  return item;
}
