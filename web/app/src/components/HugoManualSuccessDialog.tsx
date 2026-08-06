import { Check, X } from "lucide-react";
import { useEffect, useRef } from "react";

interface HugoManualSuccessDialogProps {
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}

/** HugoManualSuccessDialog 在记录外部 Hugo 发布事实前取得用户明确确认。 */
export function HugoManualSuccessDialog({ busy, onClose, onConfirm }: HugoManualSuccessDialogProps) {
  const closeButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    closeButton.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) onClose();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [busy, onClose]);

  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => {
    if (event.target === event.currentTarget && !busy) onClose();
  }}>
    <section className="disposition-dialog" role="dialog" aria-modal="true" aria-labelledby="hugo-manual-success-title">
      <header><h2 id="hugo-manual-success-title">手动标记 Hugo 成功</h2><button ref={closeButton} type="button" aria-label="关闭" disabled={busy} onClick={onClose}><X size={18} /></button></header>
      <div className="disposition-dialog-body"><p>仅记录当前版本已在外部完成发布，不会重新写入 Hugo 文件。</p></div>
      <footer><button className="secondary" type="button" disabled={busy} onClick={onClose}>取消</button><button className="primary" type="button" disabled={busy} onClick={onConfirm}><Check size={16} />{busy ? "正在处理…" : "确认标记"}</button></footer>
    </section>
  </div>;
}
