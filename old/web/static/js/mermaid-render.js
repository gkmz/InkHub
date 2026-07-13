// 预览页 Mermaid 渲染兜底：
// 若后端未将 mermaid 代码块转为图片，则前端尝试把代码块渲染成 SVG。
(function () {
  function mermaidThemeFromURL() {
    const v = new URLSearchParams(window.location.search).get("mermaidTheme");
    return v === "modern" ? "modern" : "handdrawn";
  }

  function mermaidConfig(theme) {
    const modern = {
      startOnLoad: false,
      securityLevel: "loose",
      theme: "base",
      themeVariables: {
        fontFamily: "Arial, PingFang SC, Microsoft YaHei, sans-serif",
        fontSize: "16px",
        primaryColor: "#F8FAFC",
        primaryTextColor: "#0F2742",
        primaryBorderColor: "#2E6FD8",
        lineColor: "#4A6079",
      },
      flowchart: {
        htmlLabels: false,
        curve: "catmullRom",
        nodeSpacing: 46,
        rankSpacing: 58,
        padding: 18,
        wrappingWidth: 180,
        diagramPadding: 8,
      },
    };

    if (theme === "modern") return modern;
    return {
      ...modern,
      look: "handDrawn",
      themeVariables: {
        ...modern.themeVariables,
        fontFamily:
          "Comic Sans MS, Bradley Hand, PingFang SC, Microsoft YaHei, sans-serif",
        fontSize: "18px",
        primaryColor: "#FFF8E8",
        primaryTextColor: "#3A2A10",
        primaryBorderColor: "#C77700",
        lineColor: "#8A4B00",
      },
      flowchart: {
        ...modern.flowchart,
        curve: "basis",
        nodeSpacing: 64,
        rankSpacing: 84,
        padding: 44,
      },
    };
  }

  async function ensureMermaidLib() {
    if (window.mermaid) return true;

    const script = document.createElement("script");
    script.src =
      "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js";
    script.defer = true;

    const loaded = new Promise((resolve) => {
      script.onload = () => resolve(true);
      script.onerror = () => resolve(false);
    });

    document.head.appendChild(script);
    return loaded;
  }

  async function renderMermaidBlocks() {
    const root = document.getElementById("articleContent");
    if (!root || root.dataset.mermaidProcessed === "true") return;

    const codes = root.querySelectorAll(
      'pre code.language-mermaid, pre code[class*="language-mermaid"]',
    );
    if (!codes.length) {
      root.dataset.mermaidProcessed = "true";
      return;
    }

    const ok = await ensureMermaidLib();
    if (!ok || !window.mermaid) return;

    window.mermaid.initialize(mermaidConfig(mermaidThemeFromURL()));

    let seq = 0;
    for (const code of codes) {
      const pre = code.closest("pre");
      if (!pre) continue;

      const source = code.textContent || "";
      const wrap = document.createElement("div");
      wrap.className = "mermaid-wrap";
      const host = document.createElement("div");
      host.className = "mermaid";
      host.textContent = source;
      wrap.appendChild(host);
      pre.replaceWith(wrap);

      try {
        const id = `mermaid-preview-${Date.now()}-${seq++}`;
        const result = await window.mermaid.render(id, source);
        host.innerHTML = result.svg;
        const svg = host.querySelector("svg");
        if (svg) {
          svg.removeAttribute("width");
          svg.removeAttribute("height");
          svg.style.width = "100%";
          svg.style.height = "auto";
          svg.style.display = "block";
        }
      } catch (e) {
        console.warn("Mermaid preview render failed:", e);
      }
    }

    root.dataset.mermaidProcessed = "true";
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", renderMermaidBlocks);
  } else {
    renderMermaidBlocks();
  }
})();
