import { Download, MousePointer2, X } from "lucide-react";
import { useEffect, useMemo, useRef } from "react";
import { sanitizePreviewHTML } from "../api/safeHTML";

/** ManualWeChatCopyDialog 在浏览器拒绝剪贴板权限时提供可选中的安全富文本。 */
export function ManualWeChatCopyDialog({ html, onClose }: { html: string; onClose: () => void }) {
  const contentRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const safeHTML = useMemo(() => sanitizePreviewHTML(html), [html]);

  useEffect(() => {
    closeRef.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose]);

  const selectAll = () => {
    if (!contentRef.current) return;
    const range = document.createRange();
    range.selectNodeContents(contentRef.current);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    contentRef.current.focus();
  };

  const download = () => {
    const documentHTML = `<!doctype html><html><head><meta charset="utf-8"><title>微信格式化内容</title></head><body>${safeHTML}</body></html>`;
    const url = URL.createObjectURL(new Blob([documentHTML], { type: "text/html;charset=utf-8" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "wechat-content.html";
    anchor.click();
    URL.revokeObjectURL(url);
  };

  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <section className="manual-copy-dialog" role="dialog" aria-modal="true" aria-labelledby="manual-copy-title">
      <header><h2 id="manual-copy-title">手工复制微信内容</h2><button ref={closeRef} type="button" aria-label="关闭" onClick={onClose}><X size={17} /></button></header>
      <div ref={contentRef} className="manual-copy-content" tabIndex={0} dangerouslySetInnerHTML={{ __html: safeHTML }} />
      <footer><button className="secondary" type="button" onClick={download}><Download size={15} />下载 HTML</button><button className="primary" type="button" onClick={selectAll}><MousePointer2 size={15} />选中全部内容</button></footer>
    </section>
  </div>;
}
