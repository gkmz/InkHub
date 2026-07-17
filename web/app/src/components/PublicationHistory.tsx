import { Clock3, LoaderCircle, RotateCcw } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { getPublicationHistory } from "../api/client";
import type { PublicationHistoryItem } from "../api/types";

/** PublicationHistory 按需展示 Hugo 与微信统一发布历史。 */
export function PublicationHistory({ articleID, refreshKey }: { articleID: string; refreshKey: number }) {
  const controller = useRef<AbortController | null>(null);
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<PublicationHistoryItem[]>([]);
  const [cursor, setCursor] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async (nextCursor = "", append = false) => {
    controller.current?.abort();
    controller.current = new AbortController();
    setLoading(true);
    setError("");
    try {
      const page = await getPublicationHistory(articleID, nextCursor, controller.current.signal);
      setItems((current) => append ? [...current, ...page.items] : page.items);
      setCursor(page.next_cursor ?? "");
      setLoaded(true);
    } catch (reason) {
      if (!(reason instanceof DOMException && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "发布历史读取失败");
    } finally {
      setLoading(false);
    }
  }, [articleID]);

  useEffect(() => () => controller.current?.abort(), []);
  useEffect(() => {
    setItems([]);
    setCursor("");
    setLoaded(false);
    if (open) void load();
  }, [refreshKey, open, load]);

  return <details className="publication-history" onToggle={(event) => setOpen(event.currentTarget.open)}>
    <summary><span><Clock3 size={16} />发布历史</span><small>{loaded ? `${items.length} 条记录` : "点击查看"}</small></summary>
    <div className="publication-history-body">
      {loading && items.length === 0 && <p className="history-state"><LoaderCircle className="spin" size={15} />正在读取发布历史…</p>}
      {!loading && loaded && items.length === 0 && <p className="history-state">暂无发布记录</p>}
      {items.length > 0 && <ol>{items.map((item) => <li key={item.id} className={`history-${item.state}`}><i aria-hidden="true" /><div><b>{item.title}</b><p>{item.detail}</p><small>{item.channel === "hugo" ? "Hugo" : "微信"} · {formatHistoryTime(item.occurred_at)}</small></div></li>)}</ol>}
      {error && <div className="history-error" role="alert"><span>{error}</span><button type="button" onClick={() => void load()}><RotateCcw size={13} />重新加载</button></div>}
      {cursor && !error && <button type="button" className="secondary history-more" disabled={loading} onClick={() => void load(cursor, true)}>{loading ? <LoaderCircle className="spin" size={14} /> : null}加载更多历史</button>}
    </div>
  </details>;
}

function formatHistoryTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "时间未知";
  return new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(date);
}
