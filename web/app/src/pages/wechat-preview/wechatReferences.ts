interface WeChatReference {
  index: number;
  text: string;
  href: string;
}

/** formatWeChatReferences 将微信不支持的正文链接转换为上标和文末引用。 */
export function formatWeChatReferences(value: string) {
  if (!value || typeof DOMParser === "undefined") return value;
  const document = new DOMParser().parseFromString(value, "text/html");
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

function hasReferenceSection(document: Document) {
  return Array.from(document.body.querySelectorAll("section > h3")).some((heading) => heading.textContent?.trim() === "引用链接");
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
  const label = document.createElement("span");
  label.textContent = `[${reference.index}]`;
  label.setAttribute("style", "color:#42b883;font-weight:700;margin-right:6px");
  const url = document.createElement("span");
  url.textContent = reference.href;
  url.setAttribute("style", "color:#34495e;word-break:break-all");
  item.append(label, `${reference.text}: `, url);
  return item;
}
