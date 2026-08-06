import type { MermaidTheme } from "../api/types";
import type { MermaidConfig } from "mermaid";

let mermaidSequence = 0;

/** renderMermaidSVG 使用统一主题把 Mermaid 源码渲染为安全 SVG。 */
export async function renderMermaidSVG(source: string, theme: MermaidTheme = "handdrawn"): Promise<string> {
  const mermaid = (await import("mermaid")).default;
  mermaid.initialize(theme === "modern" ? modernMermaidConfig : handDrawnMermaidConfig);
  const rendered = await mermaid.render(`inkhub-mermaid-${++mermaidSequence}`, source.trim());
  return rendered.svg;
}

const modernMermaidConfig: MermaidConfig = {
  startOnLoad: false, securityLevel: "strict" as const, theme: "base" as const,
  themeVariables: { fontFamily: "Arial, PingFang SC, Microsoft YaHei, sans-serif", fontSize: "16px", primaryColor: "#F8FAFC", primaryTextColor: "#0F2742", primaryBorderColor: "#2E6FD8", lineColor: "#4A6079", secondaryColor: "#ECF2FF", tertiaryColor: "#F4F7FB", clusterBkg: "#F1F5FA", clusterBorder: "#A7B6C7", edgeLabelBackground: "#FFFFFF", nodeBorder: "#2E6FD8", mainBkg: "#FFFFFF", textColor: "#102A43" },
  flowchart: { curve: "catmullRom", htmlLabels: false, nodeSpacing: 46, rankSpacing: 58, padding: 18, wrappingWidth: 180, diagramPadding: 8 },
};

const handDrawnMermaidConfig: MermaidConfig = {
  startOnLoad: false, securityLevel: "strict" as const, theme: "base" as const, look: "handDrawn" as const,
  themeVariables: { fontFamily: "Comic Sans MS, Bradley Hand, PingFang SC, Microsoft YaHei, sans-serif", fontSize: "18px", primaryColor: "#FFF8E8", primaryTextColor: "#3A2A10", primaryBorderColor: "#C77700", lineColor: "#8A4B00", secondaryColor: "#FFF3D8", tertiaryColor: "#FFF9EE", clusterBkg: "#FFF6E3", clusterBorder: "#D3912D", edgeLabelBackground: "#FFFDF7", nodeBorder: "#C77700", mainBkg: "#FFFFFF", textColor: "#3A2A10" },
  flowchart: { curve: "basis", htmlLabels: false, nodeSpacing: 64, rankSpacing: 84, padding: 44 },
};
