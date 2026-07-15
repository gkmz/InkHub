import { AlertTriangle, FileCheck2, FolderTree, ListTree, Plus, RefreshCw, Search, Tags } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { APIError, getTaxonomyOverview, refreshTaxonomy } from "../../api/client";
import type { TaxonomyOverview } from "../../api/types";
import { useToast } from "../../components/toast";
import { CreateTaxonomyTermDialog } from "./CreateTaxonomyTermDialog";

type TaxonomyKind = "category" | "series" | "tag";

const kindLabels: Record<TaxonomyKind, string> = { category: "类目", series: "系列", tag: "标签" };

/** TaxonomyPage 展示博客权威 taxonomy 快照，并提供可审查的类目创建流程。 */
export function TaxonomyPage() {
  const toast = useToast();
  const [overview, setOverview] = useState<TaxonomyOverview | null>(null);
  const [loadError, setLoadError] = useState("");
  const [kind, setKind] = useState<TaxonomyKind>("category");
  const [query, setQuery] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    void getTaxonomyOverview(controller.signal).then(setOverview).catch((error: unknown) => {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setLoadError(error instanceof APIError ? error.message : "类目读取失败");
    });
    return () => controller.abort();
  }, []);

  const terms = useMemo(() => overview?.terms.filter((term) => term.kind === kind && `${term.name} ${term.key}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())) ?? [], [kind, overview, query]);

  async function refresh() {
    setRefreshing(true);
    try { setOverview(await refreshTaxonomy()); toast.show({ kind: "success", message: "类目已刷新" }); }
    catch (error) { toast.show({ kind: "error", message: error instanceof APIError ? error.message : "类目刷新失败" }); }
    finally { setRefreshing(false); }
  }

  if (loadError) return <div className="empty-state error-state"><AlertTriangle size={30} /><h2>类目读取失败</h2><p>{loadError}</p></div>;
  if (!overview) return <div className="page-state">正在读取博客类目…</div>;
  if (overview.state === "not_enabled") return <div className="empty-state"><FolderTree size={32} /><h2>尚未连接博客类目</h2><p>请先在设置中连接 Hugo 博客。</p></div>;

  const canCreate = kind === "category" && !overview.readonly && Boolean(overview.provider_id && overview.revision);
  const kindLabel = kindLabels[kind];
  return <div className="taxonomy-page">
    <section className={`taxonomy-source taxonomy-state-${overview.state}`}><FileCheck2 /><div><p className="eyebrow">权威来源</p><h2>{overview.state === "ready" ? "博客类目已同步" : overview.state === "failed" ? "上次刷新失败" : "等待首次读取"}</h2><p>{overview.source} · {overview.loaded_at === "-" ? "尚无快照" : `最后读取 ${overview.loaded_at}`}</p>{overview.error && <small>{overview.error}</small>}</div><button className="secondary" type="button" aria-label="刷新类目" disabled={refreshing} onClick={() => void refresh()}><RefreshCw size={15} />{refreshing ? "正在刷新…" : "刷新类目"}</button></section>
    <section>
      <div className="taxonomy-toolbar"><div className="taxonomy-tabs" role="tablist" aria-label="Taxonomy 类型"><button role="tab" aria-selected={kind === "category"} onClick={() => { setKind("category"); setQuery(""); }}><FolderTree size={15} />类目</button><button role="tab" aria-selected={kind === "series"} onClick={() => { setKind("series"); setQuery(""); }}><ListTree size={15} />系列</button><button role="tab" aria-selected={kind === "tag"} onClick={() => { setKind("tag"); setQuery(""); }}><Tags size={15} />标签</button></div>{canCreate && <button className="primary taxonomy-create" type="button" onClick={() => setCreating(true)}><Plus size={16} />新建类目</button>}</div>
      <div className="taxonomy-search"><Search size={16} /><label className="sr-only" htmlFor="taxonomy-search">搜索{kindLabel}</label><input id="taxonomy-search" type="search" placeholder={`搜索${kindLabel}名称或路径`} value={query} onChange={(event) => setQuery(event.target.value)} /></div>
      <div className="taxonomy-list-heading"><span>{kindLabel}</span><span>标识</span><span>使用情况</span></div>
      <div className="taxonomy-term-list">{terms.map((term) => <article key={`${term.kind}:${term.key}`}><div><strong>{term.name}</strong>{term.metadata.description && <small>{term.metadata.description}</small>}</div><code>{term.key}</code><span>{term.usage_count} 篇文章</span></article>)}{terms.length === 0 && <div className="empty-state compact"><p>{query ? "没有匹配的结果" : `博客中还没有${kindLabel}`}</p></div>}</div>
    </section>
    {creating && <CreateTaxonomyTermDialog overview={overview} onClose={() => setCreating(false)} onApplied={setOverview} />}
  </div>;
}
