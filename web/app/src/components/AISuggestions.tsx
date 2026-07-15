import { Check, Sparkles, X } from "lucide-react";
import { useEffect, useState } from "react";
import type { AISuggestion } from "../api/types";

/** AISuggestions 将建议嵌入字段流，只允许用户逐项采用或忽略。 */
export function AISuggestions({ suggestions, stale, generating = false, onAccept, onGenerate }: { suggestions: AISuggestion[]; stale: boolean; generating?: boolean; onAccept: (suggestion: AISuggestion) => void; onGenerate: () => void }) {
  const [ignored, setIgnored] = useState<string[]>([]);
  const tagSuggestions = suggestions.filter((suggestion) => suggestion.field === "tags");
  const suggestionKey = tagSuggestions.map((suggestion) => suggestion.id).join("\0");
  useEffect(() => setIgnored([]), [suggestionKey]);
  const visible = tagSuggestions.filter((suggestion) => !ignored.includes(suggestion.id));
  return <section className="tool-section ai-section"><div className="tool-heading"><h2><Sparkles size={16} />AI Tag 建议</h2><button className="secondary ai-generate" type="button" disabled={generating} onClick={onGenerate}>{generating ? "正在生成…" : "生成 AI Tag"}</button></div>{stale && <p className="stale-notice">文章已更新，请重新分析</p>}{visible.length === 0 && <p className="ai-empty">尚无建议</p>}{visible.map((suggestion) => <article className="suggestion" key={suggestion.id}><div><b>{suggestion.name}</b><small>{suggestion.reason}</small></div><p><span>{suggestion.new_term ? "新 Tag" : `${suggestion.usage_count} 篇文章`}</span></p><div><button aria-label={`忽略 Tag ${suggestion.name}`} type="button" onClick={() => setIgnored([...ignored, suggestion.id])}><X size={14} />忽略</button><button aria-label={`采用 Tag ${suggestion.name}`} type="button" disabled={stale} onClick={() => onAccept(suggestion)}><Check size={14} />采用</button></div></article>)}</section>;
}
