import { X } from "lucide-react";
import { useMemo, useState } from "react";
import type { KeyboardEvent } from "react";
import type { TaxonomyFieldState } from "./SingleTaxonomyField";

/** TagOption 是文章 Tag 编辑器使用的 taxonomy 快照候选。 */
export interface TagOption {
  key: string;
  name: string;
  usageCount: number;
}

/** TagMultiSelectProps 配置一个不依赖 API 和文章持久化的 Tag 多选器。 */
export interface TagMultiSelectProps {
  value: string[];
  options: TagOption[];
  state: TaxonomyFieldState;
  onChange: (value: string[]) => void;
}

/** TagMultiSelect 支持从博客快照选择 Tag，也允许在快照不可用时手工创建。 */
export function TagMultiSelect({ value, options, state, onChange }: TagMultiSelectProps) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const terms = useMemo(() => uniqueOptions(options), [options]);
  const selectedKeys = new Set(value.map(normalize));
  const normalizedQuery = normalize(query);
  const candidates = terms.filter((option) => !selectedKeys.has(normalize(option.name)) && matches(option, normalizedQuery));
  const exact = terms.find((option) => normalize(option.name) === normalizedQuery || normalize(option.key) === normalizedQuery);
  const canCreate = Boolean(query.trim()) && !exact && !selectedKeys.has(normalizedQuery);
  const entries: Array<{ kind: "option"; option: TagOption } | { kind: "create" }> = [
    ...candidates.map((option) => ({ kind: "option" as const, option })),
    ...(canCreate ? [{ kind: "create" as const }] : []),
  ];

  function choose(entry: (typeof entries)[number]) {
    const next = entry.kind === "option" ? entry.option.name : query.trim();
    if (next && !selectedKeys.has(normalize(next))) onChange([...value, next]);
    setQuery("");
    setOpen(false);
    setActiveIndex(0);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" && entries.length > 0) {
      event.preventDefault();
      setOpen(true);
      setActiveIndex((current) => Math.min(current + 1, entries.length - 1));
    } else if (event.key === "ArrowUp" && entries.length > 0) {
      event.preventDefault();
      setActiveIndex((current) => Math.max(current - 1, 0));
    } else if (event.key === "Enter" && open && entries.length > 0) {
      event.preventDefault();
      choose(entries[Math.min(activeIndex, entries.length - 1)]);
    } else if (event.key === "Escape") {
      setOpen(false);
    } else if (event.key === "Backspace" && query === "" && value.length > 0) {
      onChange(value.slice(0, -1));
    }
  }

  return <div className="tag-editor">
    <span className="tag-editor-label">Tags</span>
    <div className="tag-editor-control">
      {value.map((tag) => {
        const known = terms.some((option) => normalize(option.name) === normalize(tag));
        return <span className={`tag-chip${known ? "" : " missing"}`} key={normalize(tag)}><span>{tag}</span>{!known && <small>博客中未发现</small>}<button type="button" aria-label={`删除 Tag ${tag}`} onClick={() => onChange(value.filter((item) => normalize(item) !== normalize(tag)))}><X size={12} /></button></span>;
      })}
      <input role="combobox" aria-label="搜索或创建 Tag" aria-expanded={open} aria-controls="tag-options" value={query} onClick={() => setOpen(true)} onFocus={() => { setOpen(true); setActiveIndex(0); }} onChange={(event) => { setQuery(event.target.value); setOpen(true); setActiveIndex(0); }} onKeyDown={handleKeyDown} />
    </div>
    {open && entries.length > 0 && <div id="tag-options" className="tag-options" role="listbox">{entries.map((entry, index) => entry.kind === "option" ? <button type="button" role="option" aria-label={`${entry.option.name}，${entry.option.usageCount} 篇文章`} aria-selected={index === activeIndex} key={entry.option.key} onMouseDown={(event) => event.preventDefault()} onClick={() => choose(entry)}><span>{entry.option.name}</span><small>{entry.option.usageCount} 篇文章</small></button> : <button type="button" role="option" aria-selected={index === activeIndex} key="create" onMouseDown={(event) => event.preventDefault()} onClick={() => choose(entry)}>创建“{query.trim()}”</button>)}</div>}
    <small className="tag-editor-state">{stateText(state)}</small>
    {value.length < 3 && <small className="tag-count-hint">建议至少选择 3 个 Tag</small>}
    {value.length > 6 && <small className="tag-count-hint">建议最多选择 6 个 Tag</small>}
  </div>;
}

function uniqueOptions(options: TagOption[]) {
  const names = new Set<string>();
  return options.filter((option) => {
    const key = normalize(option.name);
    if (!key || names.has(key)) return false;
    names.add(key);
    return true;
  });
}

function matches(option: TagOption, query: string) {
  return !query || normalize(option.name).includes(query) || normalize(option.key).includes(query);
}

function normalize(value: string) { return value.trim().toLocaleLowerCase(); }

function stateText(state: TaxonomyFieldState) {
  if (state === "loading") return "正在读取博客标签，仍可手工添加";
  if (state === "not_enabled") return "尚未连接博客标签，仍可手工添加";
  if (state === "unavailable") return "博客标签暂不可用，仍可手工添加";
  return "来自当前博客标签";
}
