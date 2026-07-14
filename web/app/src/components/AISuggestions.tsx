import { Check, Sparkles, X } from "lucide-react";
import type { AISuggestion, ArticleMetadata } from "../api/types";

/** AISuggestions 将建议嵌入字段流，只允许用户逐项采用或忽略。 */
export function AISuggestions({ suggestions, stale, onAccept }: { suggestions: AISuggestion[]; stale: boolean; onAccept: (field: keyof ArticleMetadata, value: string) => void }) {
  if (suggestions.length === 0) return null;
  return <section className="tool-section ai-section"><div className="tool-heading"><h2><Sparkles size={16} />AI 建议</h2><span>{suggestions.length} 项</span></div>{stale && <p className="stale-notice">文章已更新，请重新分析</p>}{suggestions.map((suggestion) => <article className="suggestion" key={suggestion.field}><div><b>{label(suggestion.field)}</b><small>{suggestion.reason}</small></div><p><del>{suggestion.original || "未填写"}</del><span>{suggestion.suggested}</span></p><div><button aria-label={`忽略 ${label(suggestion.field)} 建议`} type="button"><X size={14} />忽略</button><button aria-label={`采用 ${label(suggestion.field)} 建议`} type="button" disabled={stale} onClick={() => onAccept(suggestion.field, suggestion.suggested)}><Check size={14} />采用</button></div></article>)}</section>;
}

function label(field: keyof ArticleMetadata) {
  return field === "description" ? "Description" : field.charAt(0).toUpperCase() + field.slice(1);
}
