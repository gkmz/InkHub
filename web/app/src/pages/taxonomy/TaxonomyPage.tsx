import { FileCheck2, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { approveTaxonomyTerm, getTaxonomyOverview } from "../../api/client";
import type { TaxonomyOverview } from "../../api/types";
import { TagApproval } from "../../components/TagApproval";

/** TaxonomyPage 展示权威文件状态和需要人工判断的治理问题。 */
export function TaxonomyPage() {
  const [overview, setOverview] = useState<TaxonomyOverview | null>(null);
  useEffect(() => { const controller = new AbortController(); void getTaxonomyOverview(controller.signal).then(setOverview); return () => controller.abort(); }, []);
  if (!overview) return <div className="page-state">正在读取标签体系…</div>;
  return <div className="taxonomy-page"><section className="taxonomy-source"><FileCheck2 /><div><p className="eyebrow">权威来源</p><h2>Taxonomy 文件正常</h2><p>{overview.source} · 最后读取 {overview.loaded_at}</p></div><button className="secondary"><RefreshCw size={15} />重新读取</button></section><section><div className="section-heading"><div><p className="eyebrow">需要判断</p><h2>新词与近义词</h2></div><span>{overview.issues.length} 项</span></div><div className="governance-list">{overview.issues.map((issue) => <TagApproval key={issue.id} term={issue.term} similar={issue.similar} affected={issue.affected} onApprove={async () => { await approveTaxonomyTerm(issue.id); setOverview((current) => current ? { ...current, issues: current.issues.filter((item) => item.id !== issue.id) } : current); }} />)}</div></section></div>;
}
