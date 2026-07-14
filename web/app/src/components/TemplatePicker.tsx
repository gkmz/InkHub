import { Check, LayoutTemplate } from "lucide-react";
import type { TemplateSummary } from "../api/types";

/** TemplatePicker 统一展示内置和第三方模板，并在无效默认值时明确回退。 */
export function TemplatePicker({ value, templates, onChange }: { value: string; templates: TemplateSummary[]; onChange: (id: string) => void }) {
  const selected = templates.some((template) => template.id === value && template.compatible) ? value : "default";
  const fallback = selected !== value;
  return <div className="template-picker">{fallback && <p className="persistent-notice" role="status">原默认模板不可用，已回退到 InkHub Default</p>}<div className="template-grid">{templates.map((template) => <label key={template.id} className={template.id === selected ? "selected" : ""}><input type="radio" name="template" checked={template.id === selected} disabled={!template.compatible} onChange={() => onChange(template.id)} aria-label={`${template.name} ${template.version}`} /><span className={`real-template-preview ${template.id}`}><LayoutTemplate /><i /><i /><i /></span><b>{template.name}</b><small>v{template.version} · {template.compatible ? "兼容当前版本" : "暂不兼容"}</small>{template.id === selected && <Check className="selected-check" />}</label>)}</div></div>;
}
