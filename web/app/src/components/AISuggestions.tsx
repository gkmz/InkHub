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
  onAction?: (action: "accepted" | "ignored", suggestionIDs: string[]) => Promise<void>;
  onGenerate: () => void;
  onOpenHistory?: () => void;
}

/** AISuggestions 将所有结构化建议按元数据字段分组，并只修改页面草稿。 */
export function AISuggestions({ suggestions, stale, generating = false, historyCount = 0, onAccept, onAction, onGenerate, onOpenHistory }: AISuggestionsProps) {
  const [ignored, setIgnored] = useState<string[]>([]);
  const [accepted, setAccepted] = useState<string[]>([]);
  const [showIgnored, setShowIgnored] = useState(false);
  const [confirmingGenerate, setConfirmingGenerate] = useState(false);
  const [processing, setProcessing] = useState<string[]>([]);
  const [actionError, setActionError] = useState("");
  const suggestionKey = suggestions.map((suggestion) => suggestion.id).join("\0");
  useEffect(() => {
    setIgnored(suggestions.filter((suggestion) => suggestion.ignored || suggestion.status === "ignored").map((suggestion) => suggestion.id));
    setAccepted(suggestions.filter((suggestion) => suggestion.accepted || suggestion.status === "accepted").map((suggestion) => suggestion.id));
    setShowIgnored(false);
    setConfirmingGenerate(false);
    setProcessing([]);
    setActionError("");
  }, [suggestionKey, suggestions]);

  const applyAction = async (action: "accepted" | "ignored", selected: AISuggestion[]) => {
    const candidates = selected.filter((suggestion) => !processing.includes(suggestion.id) && !accepted.includes(suggestion.id) && !ignored.includes(suggestion.id) && !suggestion.accepted && !suggestion.ignored);
    if (stale || candidates.length === 0) return;
    const ids = candidates.map((suggestion) => suggestion.id);
    setActionError("");
    setProcessing((current) => [...current, ...ids]);
    if (action === "accepted") setAccepted((current) => [...current, ...ids]);
    else setIgnored((current) => [...current, ...ids]);
    try {
      // 保持组件独立使用时的同步草稿行为；文章页传入 onAction 后再等待服务端确认。
      if (!onAction) {
        if (action === "accepted") candidates.forEach((suggestion) => onAccept(suggestion));
        return;
      }
      await onAction?.(action, ids);
      if (action === "accepted") candidates.forEach((suggestion) => onAccept(suggestion));
    } catch (reason) {
      if (action === "accepted") setAccepted((current) => current.filter((id) => !ids.includes(id)));
      else setIgnored((current) => current.filter((id) => !ids.includes(id)));
      setActionError(reason instanceof Error ? reason.message : "保存 AI 建议状态失败");
    } finally {
      setProcessing((current) => current.filter((id) => !ids.includes(id)));
    }
  };
  const accept = (suggestion: AISuggestion) => applyAction("accepted", [suggestion]);
  const ignore = (suggestion: AISuggestion) => applyAction("ignored", [suggestion]);
  const requestGenerate = () => {
    if (suggestions.length === 0) {
      onGenerate();
      return;
    }
    setConfirmingGenerate(true);
  };
  const visibleSuggestionCount = suggestions.filter((suggestion) => !ignored.includes(suggestion.id) && !accepted.includes(suggestion.id) && !suggestion.accepted && !suggestion.ignored).length;

  return <section className="tool-section ai-section">
    <div className="tool-heading">
      <h2><Sparkles size={16} />AI 建议中心</h2>
      <div className="ai-heading-actions">
        {ignored.length > 0 && <button className="ai-show-ignored" type="button" onClick={() => setShowIgnored((current) => !current)}>{showIgnored ? "隐藏已忽略" : `显示已忽略（${ignored.length}）`}</button>}
        {onOpenHistory && <button className="secondary ai-history" type="button" onClick={onOpenHistory}><History size={14} />历史{historyCount > 0 ? ` ${historyCount}` : ""}</button>}
        {confirmingGenerate && !generating ? <div className="ai-generate-confirm"><span>重新生成会创建新的建议版本，继续吗？</span><button type="button" onClick={() => setConfirmingGenerate(false)}>取消</button><button type="button" onClick={() => { setConfirmingGenerate(false); onGenerate(); }}>确认生成</button></div> : <button className="secondary ai-generate" type="button" disabled={generating} onClick={requestGenerate}>{generating ? "正在生成…" : "生成 AI 建议"}</button>}
      </div>
    </div>
    <p className="ai-draft-notice">AI 建议只会加入当前草稿，保存后才写入文章。</p>
    {stale && <div className="stale-notice ai-stale-notice" role="status"><span>文章已更新，当前建议已失效，请重新生成后再采用</span><button className="secondary compact-button" type="button" onClick={requestGenerate}>重新生成</button></div>}
    {actionError && <p className="stale-notice" role="alert">{actionError}</p>}
    {visibleSuggestionCount === 0 && <p className="ai-empty">尚无可用建议</p>}
    {fieldGroups.map((group) => {
      const groupSuggestions = suggestions.filter((suggestion) => suggestion.field === group.field);
      if (groupSuggestions.length === 0) return null;
      const visible = groupSuggestions.filter((suggestion) => showIgnored ? ignored.includes(suggestion.id) : !ignored.includes(suggestion.id) && !accepted.includes(suggestion.id) && !suggestion.accepted && !suggestion.ignored);
      const pending = visible.filter((suggestion) => !accepted.includes(suggestion.id) && !ignored.includes(suggestion.id) && !suggestion.accepted && !suggestion.ignored);
      return <section className="suggestion-group" key={group.field}>
        <div className="suggestion-group-heading"><h3>{group.label}</h3><span>{pending.length || visible.length} 条</span><div className="suggestion-group-actions">{!showIgnored && <><button type="button" disabled={stale || pending.length === 0 || processing.length > 0} onClick={() => applyAction("accepted", pending)}>采用全部</button><button type="button" disabled={pending.length === 0 || processing.length > 0} onClick={() => applyAction("ignored", pending)}>忽略全部</button></>}</div></div>
        {visible.map((suggestion) => <SuggestionRow key={suggestion.id} suggestion={suggestion} stale={stale} accepted={accepted.includes(suggestion.id) || Boolean(suggestion.accepted)} ignored={ignored.includes(suggestion.id) || Boolean(suggestion.ignored)} processing={processing.includes(suggestion.id)} onAccept={() => accept(suggestion)} onIgnore={() => ignore(suggestion)} />)}
      </section>;
    })}
  </section>;
}

function SuggestionRow({ suggestion, stale, accepted, ignored, processing, onAccept, onIgnore }: { suggestion: AISuggestion; stale: boolean; accepted: boolean; ignored: boolean; processing: boolean; onAccept: () => void; onIgnore: () => void }) {
  const value = Array.isArray(suggestion.value) ? suggestion.value.join("、") : suggestion.value ?? suggestion.name;
  return <article className={`suggestion${ignored ? " suggestion-ignored" : ""}`}>
    <div><b>{value}</b>{suggestion.reason && <small>{suggestion.reason}</small>}</div>
    {suggestion.field === "tags" && <p><span>{suggestion.new_term ? "新 Tag" : `${suggestion.usage_count} 篇文章`}</span></p>}
    {suggestion.field === "keywords" && Array.isArray(suggestion.value) && <p><span>{suggestion.value.length} 个关键词</span></p>}
    <div><button aria-label={`忽略 ${value}`} type="button" disabled={processing || accepted || ignored} onClick={onIgnore}><X size={14} />忽略</button><button aria-label={`采用 ${value}`} title={stale ? "文章已更新，请先重新生成 AI 建议" : undefined} type="button" disabled={stale || accepted || ignored || processing} onClick={onAccept}>{accepted ? <Check size={14} /> : <Check size={14} />}{accepted ? "已加入草稿" : ignored ? "已忽略" : processing ? "处理中" : "采用"}</button></div>
  </article>;
}
