import { AlertTriangle, ChevronRight, History, RefreshCw, X } from "lucide-react";
import type { SuggestionHistoryItem, SuggestionVersionView } from "../api/types";

interface SuggestionHistoryProps {
  items: SuggestionHistoryItem[];
  selected: SuggestionVersionView | null;
  loading: boolean;
  detailLoading: boolean;
  error: string;
  onSelect: (id: string) => void;
  onRetry: () => void;
  onClose: () => void;
}

/** SuggestionHistory 展示 AI 建议版本历史，并以只读方式查看选中版本。 */
export function SuggestionHistory({ items, selected, loading, detailLoading, error, onSelect, onRetry, onClose }: SuggestionHistoryProps) {
  return <section className="tool-section suggestion-history" aria-label="AI 建议历史">
    <div className="tool-heading"><h2><History size={16} />建议历史</h2><button type="button" aria-label="关闭建议历史" onClick={onClose}><X size={15} /></button></div>
    {loading && <p className="history-state">正在读取建议历史…</p>}
    {error && <div className="history-error"><span>{error}</span><button type="button" onClick={onRetry}><RefreshCw size={13} />重试</button></div>}
    {!loading && !error && items.length === 0 && <p className="history-state">还没有生成过 AI 建议。</p>}
    {!loading && !error && items.length > 0 && <ol className="suggestion-history-list">{items.map((item) => <li key={item.id}><button type="button" className={selected?.id === item.id ? "selected" : ""} onClick={() => onSelect(item.id)}><span><b>{formatGeneratedAt(item.generated_at)}</b><small>{item.model || "未知模型"} · {item.suggestion_count} 条建议</small></span><span className="history-item-meta">{item.current ? "当前" : "内容已变化"}<ChevronRight size={14} /></span></button></li>)}</ol>}
    {detailLoading && <p className="history-state">正在读取版本详情…</p>}
    {selected && !detailLoading && <SuggestionVersionDetail version={selected} />}
  </section>;
}

function SuggestionVersionDetail({ version }: { version: SuggestionVersionView }) {
  return <div className="suggestion-version-detail">
    <div className="suggestion-version-heading"><strong>{formatGeneratedAt(version.generated_at)}</strong>{version.suggestions_stale && <span><AlertTriangle size={13} />内容已变化</span>}</div>
    {version.suggestions.length === 0 ? <p className="history-state">该版本没有有效建议。</p> : <ul>{version.suggestions.map((suggestion) => <li key={suggestion.id}><b>{suggestion.field}</b><span>{Array.isArray(suggestion.value) ? suggestion.value.join("、") : suggestion.value ?? suggestion.name}</span><em className={`suggestion-status status-${suggestion.status ?? (suggestion.accepted ? "accepted" : suggestion.ignored ? "ignored" : "pending")}`}>{suggestionStatusLabel(suggestion.status ?? (suggestion.accepted ? "accepted" : suggestion.ignored ? "ignored" : "pending"))}</em></li>)}</ul>}
  </div>;
}

function suggestionStatusLabel(status: string) {
  if (status === "accepted") return "已采用";
  if (status === "ignored") return "已忽略";
  return "待处理";
}

function formatGeneratedAt(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value || "未知时间" : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}
