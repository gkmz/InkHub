import { Filter, Search, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { listArticles } from "../../api/client";
import type { ArticleSummary } from "../../api/types";
import { ArticleRow } from "../../components/ArticleRow";
import { useToast } from "../../components/toast";

/** LibraryPage 提供输入法安全搜索、状态筛选和稳定分页入口。 */
export function LibraryPage({ onNavigate }: { onNavigate: (path: string) => void }) {
  const toast = useToast();
  const [input, setInput] = useState("");
  const [query, setQuery] = useState("");
  const [state, setState] = useState("");
  const [items, setItems] = useState<ArticleSummary[] | null>(null);
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const composing = useRef(false);
  const stateSelect = useRef<HTMLSelectElement>(null);
  useEffect(() => { const timer = window.setTimeout(() => { if (!composing.current) setQuery(input); }, 300); return () => window.clearTimeout(timer); }, [input]);
  useEffect(() => {
    const controller = new AbortController();
    setItems(null);
    setNextCursor("");
    listArticles(articleQuery(query, state), controller.signal).then((page) => {
      setItems(page.items);
      setNextCursor(page.next_cursor ?? "");
    }).catch((reason: Error) => {
      if (reason.name !== "AbortError") {
        setItems([]);
        toast.show({ kind: "error", message: reason.message || "无法读取内容库" });
      }
    });
    return () => controller.abort();
  }, [query, state, toast]);
  const loadMore = async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    const params = articleQuery(query, state);
    params.set("cursor", nextCursor);
    try {
      const page = await listArticles(params);
      // 数据更新可能让分页边界发生变化，按稳定文章 ID 去重后再追加。
      setItems((current) => mergeArticles(current ?? [], page.items));
      setNextCursor(page.next_cursor ?? "");
    } catch (reason) {
      toast.show({ kind: "error", message: reason instanceof Error ? reason.message : "无法读取下一页" });
    } finally {
      setLoadingMore(false);
    }
  };
  return <div className="library-page">
    <div className="library-tools"><label className="search-field"><Search size={18} /><span className="sr-only">搜索文章</span><input type="search" aria-label="搜索文章" placeholder="搜索标题" value={input} onChange={(event) => setInput(event.target.value)} onCompositionStart={() => { composing.current = true; }} onCompositionEnd={(event) => { composing.current = false; setInput(event.currentTarget.value); setQuery(event.currentTarget.value); }} /></label><button className="filter-button" type="button" aria-controls="library-filters" onClick={() => stateSelect.current?.focus()}><Filter size={17} />筛选</button></div>
    <div className="filter-strip" id="library-filters"><label>审核状态<select ref={stateSelect} value={state} onChange={(event) => setState(event.target.value)}><option value="">全部</option><option value="pending_review">等待审核</option><option value="changed">内容已更新</option><option value="blocked">处理失败</option><option value="approved">已通过</option></select></label>{state && <button type="button" onClick={() => setState("")}><X size={14} />清除筛选</button>}</div>
    <h2 className="sr-only">文章列表</h2>
    <div className="list-header"><span>文章</span><span>修改时间</span><span>审核</span><span>Hugo</span><span>微信</span><span>操作</span></div>
    <div className="article-list">{items === null ? <div className="page-state">正在读取内容库…</div> : items.length === 0 ? <div className="empty-state compact"><h2>没有符合这些条件的文章</h2><p>调整搜索词或清除筛选后再试。</p></div> : items.map((article) => <ArticleRow key={article.id} article={article} onOpen={(id) => onNavigate(`/articles/${id}`)} />)}</div>
    {items !== null && nextCursor && <div className="library-more"><button type="button" className="secondary" disabled={loadingMore} onClick={loadMore}>{loadingMore ? "正在加载…" : "加载更多"}</button></div>}
  </div>;
}

function articleQuery(query: string, state: string) {
  const params = new URLSearchParams({ limit: "50" });
  if (query) params.set("q", query);
  if (state) params.set("state", state);
  return params;
}

function mergeArticles(current: ArticleSummary[], incoming: ArticleSummary[]) {
  const seen = new Set(current.map((article) => article.id));
  return [...current, ...incoming.filter((article) => !seen.has(article.id))];
}
