import { RotateCcw, Save } from "lucide-react";
import { useEffect, useState } from "react";
import type { ArticleMetadata } from "../api/types";

interface MetadataFormProps {
  value: ArticleMetadata;
  sourceChanged: boolean;
  onSave: (value: ArticleMetadata) => void | Promise<void>;
  onReload?: () => void;
}

/** MetadataForm 编辑标准元数据，并在源文件变化时停止写回。 */
export function MetadataForm({ value, sourceChanged, onSave, onReload }: MetadataFormProps) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [value]);
  const update = <K extends keyof ArticleMetadata>(field: K, next: ArticleMetadata[K]) => setDraft((current) => ({ ...current, [field]: next }));
  const list = (field: "tags" | "keywords", text: string) => update(field, text.split(/[,，]/).map((item) => item.trim()).filter(Boolean));
  const changed = JSON.stringify(draft) !== JSON.stringify(value);
  const changes = (Object.keys(value) as (keyof ArticleMetadata)[]).filter((field) => JSON.stringify(value[field]) !== JSON.stringify(draft[field]));
  return <section className="tool-section metadata-form"><div className="tool-heading"><h2>元数据</h2>{changed && <span>有未保存修改</span>}</div>
    {sourceChanged && <div className="inline-warning"><b>文章已在写作工具中更新</b><span>重新加载后才能继续保存，避免覆盖外部修改。</span><button type="button" onClick={onReload}><RotateCcw size={15} />重新加载</button></div>}
    <label>标题<input value={draft.title} onChange={(event) => update("title", event.target.value)} /></label>
    <label>Description<textarea value={draft.description} onChange={(event) => update("description", event.target.value)} rows={3} /></label>
    <div className="field-pair"><label>Category<input value={draft.category} onChange={(event) => update("category", event.target.value)} /></label><label>Series<input value={draft.series} onChange={(event) => update("series", event.target.value)} /></label></div>
    <label>Tags<input value={draft.tags.join(", ")} onChange={(event) => list("tags", event.target.value)} /></label>
    <label>Keywords<input value={draft.keywords.join(", ")} onChange={(event) => list("keywords", event.target.value)} /></label>
    <div className="field-pair"><label>Slug<input value={draft.slug} onChange={(event) => update("slug", event.target.value)} /></label><label>Cover<input value={draft.cover} onChange={(event) => update("cover", event.target.value)} /></label></div>
    {changes.length > 0 && <div className="change-summary"><b>本次将写入</b>{changes.map((field) => <p key={field}>{fieldLabel(field)}：{formatValue(value[field]) || "未填写"} → {formatValue(draft[field]) || "未填写"}</p>)}</div>}
    <button className="primary compact-button" type="button" disabled={!changed || sourceChanged} onClick={() => void onSave(draft)}><Save size={15} />保存到文章</button>
  </section>;
}

function fieldLabel(field: keyof ArticleMetadata) {
  return field === "description" ? "Description" : field.charAt(0).toUpperCase() + field.slice(1);
}

function formatValue(value: string | string[]) {
  return Array.isArray(value) ? value.join("、") : value;
}
