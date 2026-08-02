import { Check, History, Sparkles, X } from "lucide-react";
import { useEffect, useState } from "react";
import type { AISuggestion, ArticleMetadata } from "../api/types";

const fieldGroups: Array<{ field: keyof ArticleMetadata; label: string }> = [
  { field: "description", label: "Description" },
  { field: "category", label: "Category" },
  { field: "series", label: "Series" },
  { field: "slug", label: "Slug" },
  { field: "keywords", label: "Keywords" },
  { field: "tags", label: "Tags" },
];

interface AISuggestionsProps {
  suggestions: AISuggestion[];
  stale: boolean;
  generating?: boolean;
  historyCount?: number;
  onAccept: (suggestion: AISuggestion) => void;
  onGenerate: () => void;
  onOpenHistory?: () => void;
}

/** AISuggestions 将所有结构化建议按元数据字段分组，并只修改页面草稿。 */
export function AISuggestions({ suggestions, stale, generating = false, historyCount = 0, onAccept, onGenerate, onOpenHistory }: AISuggestionsProps) {
  const [ignored, setIgnored] = useState<string[]>([]);
  const [accepted, setAccepted] = useState<string[]>([]);
  const [showIgnored, setShowIgnored] = useState(false);
  const suggestionKey = suggestions.map((suggestion) => suggestion.id).join("\0");
  useEffect(() => {
    setIgnored([]);
    setAccepted([]);
    setShowIgnored(false);
  }, [suggestionKey]);

  const accept = (suggestion: AISuggestion) => {
    if (stale || accepted.includes(suggestion.id) || suggestion.accepted) return;
    setAccepted((current) => [...current, suggestion.id]);
    onAccept(suggestion);
  };
  const ignore = (suggestionID: string) => setIgnored((current) => current.includes(suggestionID) ? current : [...current, suggestionID]);
  const visibleSuggestionCount = suggestions.filter((suggestion) => !ignored.includes(suggestion.id)).length;

  return <section className="tool-section ai-section">
    <div className="tool-heading">
      <h2><Sparkles size={16} />AI 建议中心</h2>
      <div className="ai-heading-actions">
        {onOpenHistory && <button className="secondary ai-history" type="button" onClick={onOpenHistory}><History size={14} />历史{historyCount > 0 ? ` ${historyCount}` : ""}</button>}
        <button className="secondary ai-generate" type="button" disabled={generating} onClick={onGenerate}>{generating ? "正在生成…" : "生成 AI 建议"}</button>
      </div>
    </div>
    <p className="ai-draft-notice">AI 建议只会加入当前草稿，保存后才写入文章。</p>
    {stale && <p className="stale-notice">文章已更新，请重新分析</p>}
    {visibleSuggestionCount === 0 && <p className="ai-empty">尚无可用建议</p>}
    {fieldGroups.map((group) => {
      const groupSuggestions = suggestions.filter((suggestion) => suggestion.field === group.field);
      if (groupSuggestions.length === 0) return null;
      const visible = groupSuggestions.filter((suggestion) => showIgnored || !ignored.includes(suggestion.id));
      const pending = visible.filter((suggestion) => !accepted.includes(suggestion.id) && !suggestion.accepted);
      return <section className="suggestion-group" key={group.field}>
        <div className="suggestion-group-heading"><h3>{group.label}</h3><span>{groupSuggestions.length} 条</span><div className="suggestion-group-actions"><button type="button" disabled={stale || pending.length === 0} onClick={() => pending.forEach(accept)}>采用全部</button><button type="button" disabled={visible.length === 0} onClick={() => visible.forEach((suggestion) => ignore(suggestion.id))}>忽略全部</button></div></div>
        {visible.map((suggestion) => <SuggestionRow key={suggestion.id} suggestion={suggestion} stale={stale} accepted={accepted.includes(suggestion.id) || Boolean(suggestion.accepted)} ignored={ignored.includes(suggestion.id)} onAccept={() => accept(suggestion)} onIgnore={() => ignore(suggestion.id)} />)}
      </section>;
    })}
    {ignored.length > 0 && <button className="ai-show-ignored" type="button" onClick={() => setShowIgnored((current) => !current)}>{showIgnored ? "隐藏已忽略" : `显示已忽略（${ignored.length}）`}</button>}
  </section>;
}

function SuggestionRow({ suggestion, stale, accepted, ignored, onAccept, onIgnore }: { suggestion: AISuggestion; stale: boolean; accepted: boolean; ignored: boolean; onAccept: () => void; onIgnore: () => void }) {
  const value = Array.isArray(suggestion.value) ? suggestion.value.join("、") : suggestion.value ?? suggestion.name;
  return <article className={`suggestion${ignored ? " suggestion-ignored" : ""}`}>
    <div><b>{value}</b>{suggestion.reason && <small>{suggestion.reason}</small>}</div>
    {suggestion.field === "tags" && <p><span>{suggestion.new_term ? "新 Tag" : `${suggestion.usage_count} 篇文章`}</span></p>}
    {suggestion.field === "keywords" && Array.isArray(suggestion.value) && <p><span>{suggestion.value.length} 个关键词</span></p>}
    <div><button aria-label={`忽略 ${value}`} type="button" onClick={onIgnore}><X size={14} />忽略</button><button aria-label={`采用 ${value}`} type="button" disabled={stale || accepted || ignored} onClick={onAccept}>{accepted ? <Check size={14} /> : <Check size={14} />}{accepted ? "已加入草稿" : "采用"}</button></div>
  </article>;
}
