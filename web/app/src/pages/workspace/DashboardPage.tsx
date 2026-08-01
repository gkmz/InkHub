import { useEffect, useState } from "react";
import { BookOpenText, RefreshCw } from "lucide-react";
import { getDashboard, refreshWorkspace } from "../../api/client";
import type { ArticleSummary, DashboardView } from "../../api/types";
import { ArticleRow } from "../../components/ArticleRow";
import { useToast } from "../../components/toast";

const sections: Array<{ key: keyof DashboardView; title: string }> = [
	{ key: "latest_ready", title: "最新已就绪" },
	{ key: "failed", title: "处理失败" },
	{ key: "changed", title: "内容已更新" },
	{ key: "needs_review", title: "需要审核" },
	{ key: "ready_to_publish", title: "等待发布" },
	{ key: "recently_handled", title: "最近处理" },
];

function DashboardSection({ title, items, onNavigate }: { title: string; items: ArticleSummary[]; onNavigate: (path: string) => void }) {
  if (items.length === 0) return null;
  const visible = items.filter((article) => article.content_stage !== "draft");
  if (visible.length === 0) return null;
  return <section><div className="section-heading"><h2>{title}</h2><span>{visible.length} 篇</span></div><div className="article-list">{visible.map((article) => <ArticleRow key={article.id} article={article} dashboard onOpen={(id) => onNavigate(`/articles/${id}`)} />)}</div></section>;
}

/** DashboardPage 只呈现需要处理和最近处理的文章。 */
export function DashboardPage({ onNavigate }: { onNavigate: (path: string) => void }) {
  const toast = useToast();
  const [view, setView] = useState<DashboardView | null>(null);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  useEffect(() => { const controller = new AbortController(); getDashboard(controller.signal).then((nextView) => { setView(nextView); setError(""); }).catch((reason: Error) => { if (reason.name !== "AbortError") setError(reason.message); }); return () => controller.abort(); }, [reloadKey]);
  const refresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      const result = await refreshWorkspace();
      setReloadKey((value) => value + 1);
      toast.show({ kind: "success", message: `内容库已更新，共索引 ${result.indexed} 篇文章` });
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "内容库刷新失败" });
    } finally {
      setRefreshing(false);
    }
  };
  if (error) return <div className="page-state error-state"><h2>工作台暂时无法加载</h2><p>{error}</p></div>;
  if (!view) return <div className="page-state" aria-live="polite">正在整理需要处理的文章…</div>;
  // 后端已经完成互斥分类和排序，页面保持其顺序作为唯一事实来源。
  const total = sections.reduce((count, section) => count + (view[section.key] ?? []).filter((article) => article.content_stage !== "draft").length, 0);
  return <div className="workspace-page"><div className="page-toolbar"><button className="secondary" type="button" aria-label="刷新工作区" disabled={refreshing} onClick={() => void refresh()}><RefreshCw size={15} aria-hidden="true" />{refreshing ? "正在扫描…" : "刷新工作区"}</button></div>{total === 0 ? <div className="empty-state"><BookOpenText size={30} /><h2>还没有已就绪的文章</h2><p>在文章 frontmatter 中设置 publish.status: ready 后，文章会进入工作台。</p><button className="secondary" type="button" onClick={() => onNavigate("/library")}>浏览内容库</button></div> : <div className="dashboard-sections">{sections.map((section) => <DashboardSection key={section.key} title={section.title} items={view[section.key] ?? []} onNavigate={onNavigate} />)}</div>}</div>;
}
