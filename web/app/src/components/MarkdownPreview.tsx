import { useEffect, useMemo, useRef } from "react";
import { sanitizePreviewHTML } from "../api/safeHTML";
import type { MermaidTheme } from "../api/types";
import { renderMermaidSVG } from "../platform/mermaid";

/** MarkdownPreview 渲染后端 Markdown HTML，并把 Mermaid 代码块转换为 SVG。 */
export function MarkdownPreview({ html, className = "", mermaidTheme = "handdrawn" }: { html: string; className?: string; mermaidTheme?: MermaidTheme }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const safeHTML = useMemo(() => sanitizePreviewHTML(html), [html]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const diagrams = Array.from(container.querySelectorAll<HTMLElement>("pre > code.language-mermaid, pre > code.lang-mermaid"));
    if (diagrams.length === 0) return;
    let cancelled = false;
    const render = async () => {
      for (const code of diagrams) {
        if (cancelled) return;
        const source = code.textContent?.trim() ?? "";
        if (!source) continue;
        try {
          const svg = await renderMermaidSVG(source, mermaidTheme);
          if (cancelled || !code.parentElement) return;
          const wrapper = document.createElement("div");
          wrapper.className = "mermaid-diagram";
          wrapper.setAttribute("role", "img");
          wrapper.innerHTML = svg;
          code.parentElement.replaceWith(wrapper);
        } catch {
          if (!cancelled) replaceMermaidError(code, "Mermaid 图表渲染失败，请检查图表语法");
        }
      }
    };
    void render();
    return () => { cancelled = true; };
  }, [safeHTML, mermaidTheme]);

  return <div ref={containerRef} className={`markdown-preview ${className}`.trim()} dangerouslySetInnerHTML={{ __html: safeHTML }} />;
}

function replaceMermaidError(code: HTMLElement, message: string) {
  if (!code.parentElement) return;
  const error = document.createElement("div");
  error.className = "mermaid-render-error";
  error.setAttribute("role", "alert");
  error.textContent = message;
  code.parentElement.replaceWith(error);
}
