import { renderMermaidSVG } from "../../platform/mermaid";

/** renderHugoPreviewMermaid 将 Hugo 预览中的 Mermaid 容器转换为 SVG，同时保持 iframe 脚本禁用。 */
export async function renderHugoPreviewMermaid(frame: HTMLIFrameElement): Promise<void> {
  const previewDocument = frame.contentDocument;
  if (!previewDocument) return;
  const diagrams = Array.from(previewDocument.querySelectorAll<HTMLElement>("pre.mermaid"));
  for (const diagram of diagrams) {
    const source = diagram.textContent?.trim() ?? "";
    if (!source) continue;
    try {
      const svg = await renderMermaidSVG(source, "modern");
      const wrapper = previewDocument.createElement("div");
      wrapper.className = "mermaid-diagram";
      wrapper.setAttribute("role", "img");
      wrapper.setAttribute("aria-label", "Mermaid 图表");
      wrapper.style.margin = "24px 0";
      wrapper.style.overflowX = "auto";
      wrapper.innerHTML = svg;
      const renderedSVG = wrapper.querySelector<SVGElement>("svg");
      if (renderedSVG) {
        const naturalWidth = mermaidViewBoxWidth(renderedSVG);
        renderedSVG.style.display = "block";
        renderedSVG.style.maxWidth = "100%";
        renderedSVG.style.width = naturalWidth > 0 ? `${Math.ceil(naturalWidth)}px` : "100%";
        renderedSVG.style.height = "auto";
        renderedSVG.style.margin = "0 auto";
      }
      diagram.replaceWith(wrapper);
    } catch {
      diagram.classList.add("mermaid-render-error");
      diagram.setAttribute("title", "Mermaid 图表渲染失败，请检查图表语法");
    }
  }
}

// mermaidViewBoxWidth 读取 Mermaid 的自然画布宽度，避免窄而高的图被 width=100% 放大。
function mermaidViewBoxWidth(svg: SVGElement): number {
  const values = (svg.getAttribute("viewBox") ?? "").trim().split(/[\s,]+/).map(Number);
  return values.length === 4 && Number.isFinite(values[2]) && values[2] > 0 ? values[2] : 0;
}
