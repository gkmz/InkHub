import { RotateCcw, Save } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { ArticleMetadata } from "../api/types";
import { SingleTaxonomyField, type TaxonomyFieldOption, type TaxonomyFieldState } from "./SingleTaxonomyField";
import { TagMultiSelect, type TagOption } from "./TagMultiSelect";

const emptySuggestions: Array<{ id: string; field: keyof ArticleMetadata; value: string | string[] }> = [];

interface MetadataFormProps {
  value: ArticleMetadata;
  sourceChanged: boolean;
  identityReady?: boolean;
  onSave: (value: ArticleMetadata) => void | Promise<void>;
  onReload?: () => void;
  taxonomyState?: TaxonomyFieldState;
  categoryOptions?: TaxonomyFieldOption[];
  seriesOptions?: TaxonomyFieldOption[];
  tagOptions?: TagOption[];
  canCreateTaxonomy?: boolean;
  onCreateTaxonomy?: (kind: "category" | "series", select: (name: string) => void) => void;
  externalSuggestion?: { id: string; field: keyof ArticleMetadata; value: string | string[] } | null;
  externalSuggestions?: Array<{ id: string; field: keyof ArticleMetadata; value: string | string[] }>;
}

/** MetadataForm 编辑标准元数据，并在源文件变化时停止写回。 */
export function MetadataForm({ value, sourceChanged, identityReady = true, onSave, onReload, taxonomyState = "unavailable", categoryOptions = [], seriesOptions = [], tagOptions = [], canCreateTaxonomy = false, onCreateTaxonomy, externalSuggestion, externalSuggestions = emptySuggestions }: MetadataFormProps) {
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const previousValue = useRef(value);
  useEffect(() => {
    const incoming = [...(externalSuggestion ? [externalSuggestion] : []), ...externalSuggestions];
    // 基线和 AI 建议在同一轮合并，避免异步 effect 先重置表单再丢掉刚采用的值。
    setDraft((current) => {
      const base = previousValue.current === value ? current : value;
      previousValue.current = value;
      return incoming.reduce(applySuggestion, base);
    });
  }, [value, externalSuggestion, externalSuggestions]);
  const update = <K extends keyof ArticleMetadata>(field: K, next: ArticleMetadata[K]) => setDraft((current) => ({ ...current, [field]: next }));
  const keywords = (text: string) => update("keywords", text.split(/[,，]/).map((item) => item.trim()).filter(Boolean));
  const changed = JSON.stringify(draft) !== JSON.stringify(value);
  const changes = (Object.keys(value) as (keyof ArticleMetadata)[]).filter((field) => JSON.stringify(value[field]) !== JSON.stringify(draft[field]));
  const save = async () => {
    if (saving) return;
    setSaving(true);
    try {
      await onSave(draft);
    } finally {
      setSaving(false);
    }
  };
  return <section className="tool-section metadata-form"><div className="tool-heading"><h2>元数据</h2>{changed && <span>有未保存修改</span>}</div>
    {sourceChanged && <div className="inline-warning"><b>文章已在写作工具中更新</b><span>重新加载后才能继续保存，避免覆盖外部修改。</span><button type="button" onClick={onReload}><RotateCcw size={15} />重新加载</button></div>}
    {!identityReady && <div className="inline-identity" role="status"><b>这篇文章还没有稳定 ID</b><span>保存时会自动生成并写入源文件，之后需要重新审核。</span></div>}
    <label>标题<input value={draft.title} onChange={(event) => update("title", event.target.value)} /></label>
    <label>Description<textarea value={draft.description} onChange={(event) => update("description", event.target.value)} rows={3} /></label>
    <div className="field-pair"><SingleTaxonomyField label="Category" noun="类目" value={draft.category} options={categoryOptions} state={taxonomyState} emptyLabel="未分类" canCreate={canCreateTaxonomy} onChange={(next) => update("category", next)} onCreate={onCreateTaxonomy ? (select) => onCreateTaxonomy("category", select) : undefined} /><SingleTaxonomyField label="Series" noun="系列" value={draft.series} options={seriesOptions} state={taxonomyState} emptyLabel="无系列" canCreate={canCreateTaxonomy} onChange={(next) => update("series", next)} onCreate={onCreateTaxonomy ? (select) => onCreateTaxonomy("series", select) : undefined} /></div>
    <TagMultiSelect value={draft.tags} options={tagOptions} state={taxonomyState} onChange={(next) => update("tags", next)} />
    <label>Keywords<input value={draft.keywords.join(", ")} onChange={(event) => keywords(event.target.value)} /></label>
    <div className="field-pair"><label>Slug<input value={draft.slug} onChange={(event) => update("slug", event.target.value)} /></label><label>Cover<input value={draft.cover} onChange={(event) => update("cover", event.target.value)} /></label></div>
    {changes.length > 0 && <div className="change-summary"><b>本次将写入</b>{changes.map((field) => <p key={field}>{fieldLabel(field)}：{formatValue(value[field]) || "未填写"} → {formatValue(draft[field]) || "未填写"}</p>)}</div>}
    <button className="primary compact-button" type="button" disabled={saving || (!changed && identityReady) || sourceChanged} onClick={() => void save()}><Save size={15} />{saving ? "正在保存" : identityReady ? "保存到文章" : "保存并生成文章 ID"}</button>
  </section>;
}

/** 将一个 AI 建议合并到表单草稿，保证批量采用时每一项都被处理。 */
function applySuggestion(current: ArticleMetadata, suggestion: { field: keyof ArticleMetadata; value: string | string[] }) {
  if (suggestion.field === "tags") {
    if (typeof suggestion.value !== "string") return current;
    const exists = current.tags.some((tag) => normalize(tag) === normalize(suggestion.value as string));
    return exists ? current : { ...current, tags: [...current.tags, suggestion.value as string] };
  }
  if (suggestion.field === "keywords" && Array.isArray(suggestion.value)) {
    return { ...current, keywords: suggestion.value };
  }
  return typeof suggestion.value === "string" ? { ...current, [suggestion.field]: suggestion.value } as ArticleMetadata : current;
}

function fieldLabel(field: keyof ArticleMetadata) {
  return field === "description" ? "Description" : field.charAt(0).toUpperCase() + field.slice(1);
}

function formatValue(value: string | string[]) {
  return Array.isArray(value) ? value.join("、") : value;
}

function normalize(value: string) { return value.trim().toLocaleLowerCase(); }
