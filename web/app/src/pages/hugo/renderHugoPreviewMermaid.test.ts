import { afterEach, expect, test, vi } from "vitest";
import { renderMermaidSVG } from "../../platform/mermaid";
import { renderHugoPreviewMermaid } from "./renderHugoPreviewMermaid";

vi.mock("../../platform/mermaid", () => ({ renderMermaidSVG: vi.fn() }));

afterEach(() => {
  document.body.innerHTML = "";
  vi.resetAllMocks();
});

test("将 Hugo Mermaid 容器转换为 SVG", async () => {
  vi.mocked(renderMermaidSVG).mockResolvedValue('<svg viewBox="0 0 100 50"><text>流程图</text></svg>');
  const frame = document.createElement("iframe");
  document.body.appendChild(frame);
  const previewDocument = frame.contentDocument;
  if (!previewDocument) throw new Error("测试 iframe 缺少 contentDocument");
  previewDocument.body.innerHTML = '<pre class="mermaid">graph TD\nA--&gt;B</pre>';

  await renderHugoPreviewMermaid(frame);

  expect(renderMermaidSVG).toHaveBeenCalledWith("graph TD\nA-->B", "modern");
  expect(previewDocument.querySelector("pre.mermaid")).not.toBeInTheDocument();
  const svg = previewDocument.querySelector<SVGElement>('.mermaid-diagram[role="img"] svg');
  expect(svg).toBeInTheDocument();
  expect(svg).toHaveStyle({ width: "100px", maxWidth: "100%", height: "auto" });
});
