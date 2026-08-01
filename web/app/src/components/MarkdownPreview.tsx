import { useEffect, useMemo, useRef } from "react";
import { sanitizePreviewHTML } from "../api/safeHTML";

let mermaidSequence = 0;

/** MarkdownPreview 渲染后端 Markdown HTML，并把 Mermaid 代码块转换为 SVG。 */
export function MarkdownPreview({ html, className = "" }: { html: string; className?: string }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const safeHTML = useMemo(() => sanitizePreviewHTML(html), [html]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const diagrams = Array.from(container.querySelectorAll<HTMLElement>("pre > code.language-mermaid, pre > code.lang-mermaid"));
    if (diagrams.length === 0) return;
    let cancelled = false;
    const render = async () => {
      let mermaid: typeof import("mermaid").default;
      try {
        mermaid = (await import("mermaid")).default;
      } catch {
        return;
      }
      if (cancelled) return;
      mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme: "neutral" });
      for (const code of diagrams) {
        if (cancelled) return;
        const source = code.textContent?.trim() ?? "";
        if (!source) continue;
        try {
          const rendered = await mermaid.render(`inkhub-mermaid-${++mermaidSequence}`, source);
          if (cancelled || !code.parentElement) return;
          const wrapper = document.createElement("div");
          wrapper.className = "mermaid-diagram";
          wrapper.setAttribute("role", "img");
          wrapper.innerHTML = rendered.svg;
          code.parentElement.replaceWith(wrapper);
          rendered.bindFunctions?.(wrapper);
        } catch {
          // Mermaid 语法错误时保留源码代码块，用户仍可定位和修正问题。
        }
      }
    };
    void render();
    return () => { cancelled = true; };
  }, [safeHTML]);

  return <div ref={containerRef} className={`markdown-preview ${className}`.trim()} dangerouslySetInnerHTML={{ __html: safeHTML }} />;
}
