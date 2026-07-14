import { Check, GitCompareArrows, X } from "lucide-react";
import { useState } from "react";

/** TagApproval 在写入 taxonomy 前展示 YAML 差异和影响范围。 */
export function TagApproval({ term, similar, affected, onApprove }: { term: string; similar: string[]; affected: string[]; onApprove: () => void | Promise<void> }) {
  const [preview, setPreview] = useState(false);
  return <article className="governance-item"><div className="term-summary"><span className="term-mark">T</span><div><h3>{term}</h3><p>{similar.length ? `可能与「${similar.join("、")}」含义接近` : "尚未收入 taxonomy"}</p></div><span>{affected.length} 篇文章</span></div>{!preview ? <div className="governance-actions"><button type="button"><X size={15} />改用现有 Tag</button><button type="button" onClick={() => setPreview(true)}><GitCompareArrows size={15} />批准新 Tag</button></div> : <div className="diff-preview"><div><b>Taxonomy YAML</b><pre><code>+ {term}</code></pre></div><p>将更新 {affected.length} 篇文章</p><ul>{affected.map((article) => <li key={article}>{article}</li>)}</ul><button className="primary" type="button" onClick={() => void onApprove()}><Check size={15} />批准并更新 {affected.length} 篇文章</button></div>}</article>;
}
