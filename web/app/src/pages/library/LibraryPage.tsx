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
  const [disposition, setDisposition] = useState("");
  const [items, setItems] = useState<ArticleSummary[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const composing = useRef(false);
  const stateSelect = useRef<HTMLSelectElement>(null);
  const selectAll = useRef<HTMLInputElement>(null);
  useEffect(() => { const timer = window.setTimeout(() => { if (!composing.current) setQuery(input); }, 300); return () => window.clearTimeout(timer); }, [input]);
  useEffect(() => {
    const controller = new AbortController();
    setItems(null);
    setNextCursor("");
    listArticles(articleQuery(query, state, disposition), controller.signal).then((page) => {
      setItems(page.items);
      setNextCursor(page.next_cursor ?? "");
      // 搜索或筛选变化后，只保留新结果中仍可见的选择。
      const visibleIDs = new Set(page.items.map((article) => article.id));
      setSelected((current) => new Set([...current].filter((id) => visibleIDs.has(id))));
    }).catch((reason: Error) => {
      if (reason.name !== "AbortError") {
        setItems([]);
        setSelected(new Set());
        toast.show({ kind: "error", message: reason.message || "无法读取内容库" });
      }
    });
    return () => controller.abort();
  }, [query, state, disposition, toast]);
  const loadMore = async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    const params = articleQuery(query, state, disposition);
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
  const visibleItems = items ?? [];
  const allSelected = visibleItems.length > 0 && visibleItems.every((article) => selected.has(article.id));
  const someSelected = visibleItems.some((article) => selected.has(article.id));
  useEffect(() => {
    if (selectAll.current) selectAll.current.indeterminate = someSelected && !allSelected;
  }, [allSelected, someSelected]);
  const toggleArticle = (id: string, checked: boolean) => setSelected((current) => {
    const next = new Set(current);
    if (checked) next.add(id); else next.delete(id);
    return next;
  });
  return <div className="library-page">
    <div className="library-tools"><label className="search-field"><Search size={18} /><span className="sr-only">搜索文章</span><input type="search" aria-label="搜索文章" placeholder="搜索标题" value={input} onChange={(event) => setInput(event.target.value)} onCompositionStart={() => { composing.current = true; }} onCompositionEnd={(event) => { composing.current = false; setInput(event.currentTarget.value); setQuery(event.currentTarget.value); }} /></label><button className="filter-button" type="button" aria-controls="library-filters" onClick={() => stateSelect.current?.focus()}><Filter size={17} />筛选</button></div>
    <div className="filter-strip" id="library-filters"><label>审核状态<select ref={stateSelect} value={state} onChange={(event) => setState(event.target.value)}><option value="">全部</option><option value="pending_review">等待审核</option><option value="changed">内容已更新</option><option value="blocked">处理失败</option><option value="approved">已通过</option></select></label><label>处置状态<select value={disposition} onChange={(event) => setDisposition(event.target.value)}><option value="">全部</option><option value="unresolved">未处理</option><option value="published">已发表</option><option value="ignored">已忽略</option></select></label>{(state || disposition) && <button type="button" onClick={() => { setState(""); setDisposition(""); }}><X size={14} />清除筛选</button>}</div>
    {selected.size > 0 && <div className="batch-bar" role="region" aria-label="批量操作"><strong>已选择 {selected.size} 篇</strong><div className="batch-actions">{disposition === "ignored" ? <button className="primary" type="button">恢复管理</button> : <><button className="primary" type="button">标记已发表</button><button className="secondary" type="button">忽略</button></>}<button className="secondary" type="button" onClick={() => setSelected(new Set())}>取消选择</button></div></div>}
    <h2 className="sr-only">文章列表</h2>
    <div className="list-header selectable"><label className="select-all"><input ref={selectAll} type="checkbox" aria-label="选择当前已加载文章" aria-checked={someSelected && !allSelected ? "mixed" : allSelected} checked={allSelected} disabled={visibleItems.length === 0} onChange={() => setSelected(allSelected ? new Set() : new Set(visibleItems.map((article) => article.id)))} /></label><span>文章</span><span>修改时间</span><span>审核</span><span>Hugo</span><span>微信</span><span>操作</span></div>
    <div className="article-list">{items === null ? <div className="page-state">正在读取内容库…</div> : items.length === 0 ? <div className="empty-state compact"><h2>没有符合这些条件的文章</h2><p>调整搜索词或清除筛选后再试。</p></div> : items.map((article) => <ArticleRow key={article.id} article={article} selected={selected.has(article.id)} onSelectedChange={toggleArticle} onOpen={(id) => onNavigate(`/articles/${id}`)} />)}</div>
    {items !== null && nextCursor && <div className="library-more"><button type="button" className="secondary" disabled={loadingMore} onClick={loadMore}>{loadingMore ? "正在加载…" : "加载更多"}</button></div>}
  </div>;
}

function articleQuery(query: string, state: string, disposition: string) {
  const params = new URLSearchParams({ limit: "50" });
  if (query) params.set("q", query);
  if (state) params.set("state", state);
  if (disposition) params.set("disposition", disposition);
  return params;
}

function mergeArticles(current: ArticleSummary[], incoming: ArticleSummary[]) {
  const seen = new Set(current.map((article) => article.id));
  return [...current, ...incoming.filter((article) => !seen.has(article.id))];
}
