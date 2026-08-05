import { useEffect, useMemo, useRef } from "react";
import { sanitizePreviewHTML } from "../api/safeHTML";
import type { MermaidTheme } from "../api/types";

let mermaidSequence = 0;

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
      let mermaid: typeof import("mermaid").default;
      try {
        mermaid = (await import("mermaid")).default;
      } catch {
        diagrams.forEach((code) => replaceMermaidError(code, "Mermaid 图表组件加载失败"));
        return;
      }
      if (cancelled) return;
      // 预览和微信准备任务使用同一套历史主题，避免复制前后视觉不一致。
      mermaid.initialize(mermaidTheme === "modern" ? modernMermaidConfig : handDrawnMermaidConfig);
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
          if (!cancelled) replaceMermaidError(code, "Mermaid 图表渲染失败，请检查图表语法");
        }
      }
    };
    void render();
    return () => { cancelled = true; };
  }, [safeHTML, mermaidTheme]);

  return <div ref={containerRef} className={`markdown-preview ${className}`.trim()} dangerouslySetInnerHTML={{ __html: safeHTML }} />;
}

const modernMermaidConfig = {
  startOnLoad: false, securityLevel: "strict" as const, theme: "base" as const,
  themeVariables: { fontFamily: "Arial, PingFang SC, Microsoft YaHei, sans-serif", fontSize: "16px", primaryColor: "#F8FAFC", primaryTextColor: "#0F2742", primaryBorderColor: "#2E6FD8", lineColor: "#4A6079", secondaryColor: "#ECF2FF", tertiaryColor: "#F4F7FB", clusterBkg: "#F1F5FA", clusterBorder: "#A7B6C7", edgeLabelBackground: "#FFFFFF", nodeBorder: "#2E6FD8", mainBkg: "#FFFFFF", textColor: "#102A43" },
  flowchart: { curve: "catmullRom", htmlLabels: false, nodeSpacing: 46, rankSpacing: 58, padding: 18, wrappingWidth: 180, diagramPadding: 8 },
};

const handDrawnMermaidConfig = {
  startOnLoad: false, securityLevel: "strict" as const, theme: "base" as const, look: "handDrawn" as const,
  themeVariables: { fontFamily: "Comic Sans MS, Bradley Hand, PingFang SC, Microsoft YaHei, sans-serif", fontSize: "18px", primaryColor: "#FFF8E8", primaryTextColor: "#3A2A10", primaryBorderColor: "#C77700", lineColor: "#8A4B00", secondaryColor: "#FFF3D8", tertiaryColor: "#FFF9EE", clusterBkg: "#FFF6E3", clusterBorder: "#D3912D", edgeLabelBackground: "#FFFDF7", nodeBorder: "#C77700", mainBkg: "#FFFFFF", textColor: "#3A2A10" },
  flowchart: { curve: "basis", htmlLabels: false, nodeSpacing: 64, rankSpacing: 84, padding: 44 },
};

function replaceMermaidError(code: HTMLElement, message: string) {
  if (!code.parentElement) return;
  const error = document.createElement("div");
  error.className = "mermaid-render-error";
  error.setAttribute("role", "alert");
  error.textContent = message;
  code.parentElement.replaceWith(error);
}
