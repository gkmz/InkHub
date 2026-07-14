import { useEffect, useState } from "react";
import { BookOpenText } from "lucide-react";
import { getDashboard } from "../../api/client";
import type { ArticleSummary } from "../../api/types";
import { ArticleRow } from "../../components/ArticleRow";

const priority = { blocked: 0, changed: 1, incomplete: 2, pending_review: 3, approved: 4 };

/** DashboardPage 只呈现需要处理和最近处理的文章。 */
export function DashboardPage({ onNavigate }: { onNavigate: (path: string) => void }) {
  const [items, setItems] = useState<ArticleSummary[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => { const controller = new AbortController(); getDashboard(controller.signal).then((page) => setItems([...page.items].sort((a, b) => priority[a.state] - priority[b.state] || b.modified_at.localeCompare(a.modified_at)))).catch((reason: Error) => { if (reason.name !== "AbortError") setError(reason.message); }); return () => controller.abort(); }, []);
  if (error) return <div className="page-state error-state"><h2>工作台暂时无法加载</h2><p>{error}</p></div>;
  if (!items) return <div className="page-state" aria-live="polite">正在整理需要处理的文章…</div>;
  if (items.length === 0) return <div className="empty-state"><BookOpenText size={30} /><h2>目前没有需要处理的文章</h2><p>已索引的文章会继续保留在内容库中。</p><button className="secondary" onClick={() => onNavigate("/library")}>浏览内容库</button></div>;
  return <div className="workspace-page"><section><div className="section-heading"><div><p className="eyebrow">按优先级</p><h2>需要处理</h2></div><span>{items.filter((item) => item.state !== "approved").length} 篇</span></div><div className="article-list">{items.filter((item) => item.state !== "approved").map((article) => <ArticleRow key={article.id} article={article} dashboard onOpen={(id) => onNavigate(`/articles/${id}`)} />)}</div></section></div>;
}
