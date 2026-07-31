import { Check, EyeOff, RotateCcw, Settings, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { PublicationChannel } from "../api/types";

interface BatchDispositionDialogProps {
  mode: "published" | "ignored" | "restore";
  count: number;
  channels: Record<PublicationChannel, boolean>;
  busy?: boolean;
  onClose: () => void;
  onConfirm: (channels: PublicationChannel[]) => void;
  onOpenSettings?: () => void;
}

const channelLabels: Record<PublicationChannel, string> = { hugo: "Hugo", wechat: "微信" };

/** BatchDispositionDialog 在批量改变文章管理状态前收集明确确认。 */
export function BatchDispositionDialog({ mode, count, channels, busy = false, onClose, onConfirm, onOpenSettings }: BatchDispositionDialogProps) {
  const [selected, setSelected] = useState<Set<PublicationChannel>>(() => new Set());
  const closeButton = useRef<HTMLButtonElement>(null);
  const hugoInput = useRef<HTMLInputElement>(null);
  const wechatInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (mode === "published") {
      if (channels.hugo) hugoInput.current?.focus();
      else if (channels.wechat) wechatInput.current?.focus();
      else closeButton.current?.focus();
    } else closeButton.current?.focus();
  }, [channels.hugo, channels.wechat, mode]);
  useEffect(() => {
    // 写请求进行中禁止关闭，避免用户误以为批量事务已经取消。
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape" && !busy) onClose(); };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [busy, onClose]);

  const toggleChannel = (channel: PublicationChannel, checked: boolean) => setSelected((current) => {
    const next = new Set(current);
    if (checked) next.add(channel); else next.delete(channel);
    return next;
  });
  const selectedChannels = (["hugo", "wechat"] as PublicationChannel[]).filter((channel) => selected.has(channel));
  const title = mode === "published" ? "标记为已发表" : mode === "ignored" ? "忽略文章" : "恢复文章管理";
  const confirmLabel = mode === "published" ? "确认标记" : mode === "ignored" ? "确认忽略" : "确认恢复";
  const ConfirmIcon = mode === "published" ? Check : mode === "ignored" ? EyeOff : RotateCcw;

  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose(); }}>
    <section className="disposition-dialog" role="dialog" aria-modal="true" aria-labelledby="disposition-title">
      <header><h2 id="disposition-title">{title}</h2><button ref={closeButton} type="button" aria-label="关闭" title="关闭" disabled={busy} onClick={onClose}><X size={18} /></button></header>
      <div className="disposition-dialog-body">
        <p>{mode === "published" ? `选择 ${count} 篇文章已经发表的渠道。` : mode === "ignored" ? `将忽略 ${count} 篇文章，内容更新后仍会保持忽略。` : `将 ${count} 篇文章恢复到日常管理。`}</p>
        {mode === "published" && <fieldset><legend>发表渠道</legend>{(["hugo", "wechat"] as PublicationChannel[]).map((channel) => <label key={channel} className={!channels[channel] ? "disabled" : ""}><input ref={channel === "hugo" ? hugoInput : wechatInput} aria-label={channelLabels[channel]} type="checkbox" checked={selected.has(channel)} disabled={!channels[channel] || busy} onChange={(event) => toggleChannel(channel, event.currentTarget.checked)} /><span>{channelLabels[channel]}</span><small>{channels[channel] ? "已启用" : "未配置"}</small></label>)}</fieldset>}
        {mode === "published" && (!channels.hugo || !channels.wechat) && <button className="settings-link" type="button" disabled={busy} onClick={onOpenSettings}><Settings size={15} />前往设置</button>}
      </div>
      <footer><button className="secondary" type="button" disabled={busy} onClick={onClose}>取消</button><button className="primary" type="button" aria-label={confirmLabel} disabled={busy || mode === "published" && selectedChannels.length === 0} onClick={() => onConfirm(selectedChannels)}><ConfirmIcon size={16} />{busy ? "正在处理…" : confirmLabel}</button></footer>
    </section>
  </div>;
}
