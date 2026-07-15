import { Plus } from "lucide-react";

/** TaxonomyFieldState 描述文章字段当前可用的 taxonomy 快照状态。 */
export type TaxonomyFieldState = "loading" | "ready" | "unavailable" | "not_enabled";

/** TaxonomyFieldOption 是单值 taxonomy 字段可选择的标准 term。 */
export interface TaxonomyFieldOption {
  key: string;
  name: string;
}

/** SingleTaxonomyFieldProps 配置一个无 API 依赖的单值 taxonomy 选择器。 */
export interface SingleTaxonomyFieldProps {
  label: "Category" | "Series";
  noun: "类目" | "系列";
  value: string;
  options: TaxonomyFieldOption[];
  state: TaxonomyFieldState;
  emptyLabel: string;
  canCreate: boolean;
  onChange: (value: string) => void;
  onCreate?: (select: (name: string) => void) => void;
}

/** SingleTaxonomyField 保留快照外旧值，并统一 Category 与 Series 的状态反馈。 */
export function SingleTaxonomyField({ label, noun, value, options, state, emptyLabel, canCreate, onChange, onCreate }: SingleTaxonomyFieldProps) {
  const terms = uniqueTerms(options);
  const currentMissing = Boolean(value) && !terms.some((option) => option.name === value);
  const createEnabled = canCreate && Boolean(onCreate);
  return <div className={`taxonomy-field${createEnabled ? " has-create" : ""}`}>
    <label>{label}<select value={value} onChange={(event) => onChange(event.target.value)}><option value="">{emptyLabel}</option>{currentMissing && <option value={value}>{value}（博客中未发现）</option>}{terms.map((option) => <option key={option.key} value={option.name}>{option.name}</option>)}</select></label>
    {createEnabled && onCreate && <button type="button" aria-label={`新建${noun}`} onClick={() => onCreate(onChange)}><Plus size={15} /></button>}
    <small>{fieldStateText(state, noun)}</small>
  </div>;
}

function uniqueTerms(options: TaxonomyFieldOption[]) {
  const names = new Set<string>();
  return options.filter((option) => {
    if (!option.name || names.has(option.name)) return false;
    names.add(option.name);
    return true;
  });
}

function fieldStateText(state: TaxonomyFieldState, noun: "类目" | "系列") {
  if (state === "loading") return `正在读取博客${noun}…`;
  if (state === "not_enabled") return `尚未连接博客${noun}`;
  if (state === "unavailable") return `博客${noun}暂不可用`;
  return `来自当前博客${noun}`;
}
