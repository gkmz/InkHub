import { Filter, Search, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { listArticles } from "../../api/client";
import type { ArticleSummary } from "../../api/types";
import { ArticleRow } from "../../components/ArticleRow";

/** LibraryPage 提供输入法安全搜索、状态筛选和稳定分页入口。 */
export function LibraryPage({ onNavigate }: { onNavigate: (path: string) => void }) {
  const [input, setInput] = useState("");
  const [query, setQuery] = useState("");
  const [state, setState] = useState("");
  const [items, setItems] = useState<ArticleSummary[] | null>(null);
  const composing = useRef(false);
  useEffect(() => { const timer = window.setTimeout(() => { if (!composing.current) setQuery(input); }, 300); return () => window.clearTimeout(timer); }, [input]);
  useEffect(() => { const controller = new AbortController(); const params = new URLSearchParams({ limit: "50" }); if (query) params.set("q", query); if (state) params.set("state", state); listArticles(params, controller.signal).then((page) => setItems(page.items)).catch((reason: Error) => { if (reason.name !== "AbortError") setItems([]); }); return () => controller.abort(); }, [query, state]);
  return <div className="library-page">
    <div className="library-tools"><label className="search-field"><Search size={18} /><span className="sr-only">搜索文章</span><input type="search" aria-label="搜索文章" placeholder="搜索标题" value={input} onChange={(event) => setInput(event.target.value)} onCompositionStart={() => { composing.current = true; }} onCompositionEnd={(event) => { composing.current = false; setInput(event.currentTarget.value); setQuery(event.currentTarget.value); }} /></label><button className="filter-button" type="button"><Filter size={17} />筛选</button></div>
    <div className="filter-strip"><label>审核状态<select value={state} onChange={(event) => setState(event.target.value)}><option value="">全部</option><option value="pending_review">等待审核</option><option value="changed">内容已更新</option><option value="blocked">处理失败</option><option value="approved">已通过</option></select></label>{state && <button type="button" onClick={() => setState("")}><X size={14} />清除筛选</button>}</div>
    <h2 className="sr-only">文章列表</h2>
    <div className="list-header"><span>文章</span><span>修改时间</span><span>审核</span><span>Hugo</span><span>微信</span><span>操作</span></div>
    <div className="article-list">{items === null ? <div className="page-state">正在读取内容库…</div> : items.length === 0 ? <div className="empty-state compact"><h2>没有符合这些条件的文章</h2><p>调整搜索词或清除筛选后再试。</p></div> : items.map((article) => <ArticleRow key={article.id} article={article} onOpen={(id) => onNavigate(`/articles/${id}`)} />)}</div>
  </div>;
}
