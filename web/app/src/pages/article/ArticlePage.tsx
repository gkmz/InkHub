import { ArrowLeft, Bot, Check, CloudUpload, Send } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { generateArticleSuggestions, getArticle, getPublicationWorkflow, getTaxonomyOverview, reviewArticle, saveMetadata } from "../../api/client";
import { sanitizePreviewHTML } from "../../api/safeHTML";
import type { ArticleDetail, ArticleMetadata, TaxonomyOverview } from "../../api/types";
import { AISuggestions } from "../../components/AISuggestions";
import { Checks } from "../../components/Checks";
import { HugoPublishFlow } from "../../components/HugoPublishFlow";
import { MetadataForm } from "../../components/MetadataForm";
import type { TaxonomyFieldState } from "../../components/SingleTaxonomyField";
import { PublicationTrack } from "../../components/PublicationTrack";
import { PublicationHistory } from "../../components/PublicationHistory";
import { CreateTaxonomyTermDialog } from "../taxonomy/CreateTaxonomyTermDialog";

type MobileTab = "content" | "review" | "publish";
type TaxonomyFieldKind = "category" | "series";
type PendingTaxonomySelection = { kind: TaxonomyFieldKind; select: (name: string) => void } | null;

/** ArticlePage 以文章内容为主，右侧集中审核和发布工具。 */
export function ArticlePage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [tab, setTab] = useState<MobileTab>("content");
  const [notice, setNotice] = useState("");
  const [taxonomy, setTaxonomy] = useState<TaxonomyOverview | null>(null);
  const [taxonomyState, setTaxonomyState] = useState<TaxonomyFieldState>("loading");
  const [taxonomySelection, setTaxonomySelection] = useState<PendingTaxonomySelection>(null);
  const [generatingAI, setGeneratingAI] = useState(false);
  const [externalSuggestion, setExternalSuggestion] = useState<{ id: string; field: keyof ArticleMetadata; value: string } | null>(null);
  const [showHugoFlow, setShowHugoFlow] = useState(false);
  const [historyRefreshKey, setHistoryRefreshKey] = useState(0);
  const load = useCallback(() => getArticle(articleID).then(setArticle).catch((reason: Error) => setNotice(reason.message)), [articleID]);
  useEffect(() => { void load(); }, [load]);
  // 工作流读取属于增强能力：恢复未完成任务，但失败时不能阻断文章详情。
  useEffect(() => {
    const controller = new AbortController();
    setShowHugoFlow(false);
    void getPublicationWorkflow(articleID, controller.signal).then((workflow) => {
      if (workflow.hugo && workflow.hugo.state !== "published") setShowHugoFlow(true);
    }).catch((error: unknown) => {
      if (error instanceof DOMException && error.name === "AbortError") return;
    });
    return () => controller.abort();
  }, [articleID]);
  // Taxonomy 是文章编辑的增强数据，加载失败不能阻断文章详情或清空旧 Category/Series。
  useEffect(() => {
    const controller = new AbortController();
    void getTaxonomyOverview(controller.signal).then((overview) => {
      setTaxonomy(overview);
      setTaxonomyState(overview.state === "not_enabled" ? "not_enabled" : overview.state === "not_loaded" ? "unavailable" : "ready");
    }).catch((error: unknown) => {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setTaxonomyState("unavailable");
    });
    return () => controller.abort();
  }, []);
  if (!article) return <div className="page-state">{notice || "正在打开文章…"}</div>;
  // 旧版本或降级响应可能缺少 terms，页面必须退化为空候选而不是白屏。
  const taxonomyTerms = Array.isArray(taxonomy?.terms) ? taxonomy.terms : [];
  const categoryOptions = taxonomyTerms.filter((term) => term.kind === "category").map((term) => ({ key: term.key, name: term.name }));
  const seriesOptions = taxonomyTerms.filter((term) => term.kind === "series").map((term) => ({ key: term.key, name: term.name }));
  const tagOptions = taxonomyTerms.filter((term) => term.kind === "tag").map((term) => ({ key: term.key, name: term.name, usageCount: term.usage_count }));
  const canCreateTaxonomy = Boolean(taxonomy && !taxonomy.readonly && taxonomy.provider_id && taxonomy.revision);
  const primary = article.review_state !== "已通过" ? { label: "审核通过", icon: Check, action: async () => { await reviewArticle(article.id); setArticle({ ...article, review_state: "已通过", hugo_state: "需要同步" }); } } : article.hugo_state !== "已同步" ? { label: "同步到 Hugo", icon: CloudUpload, action: async () => setShowHugoFlow(true) } : { label: "准备微信内容", icon: Send, action: async () => onNavigate(`/articles/${article.id}/wechat`) };
  const PrimaryIcon = primary.icon;
  const hidePrimaryForHugoFlow = showHugoFlow && article.review_state === "已通过" && article.hugo_state !== "已同步";
  return <div className="article-page">
    <div className="article-toolbar"><button type="button" onClick={() => onNavigate("/library")}><ArrowLeft size={16} />返回内容库</button><span>{article.relative_path}</span></div>
    <PublicationTrack review={article.review_state} hugo={article.hugo_state} wechat={article.wechat_state} />
    <div className="mobile-tabs" role="tablist" aria-label="文章详情"><button role="tab" aria-selected={tab === "content"} onClick={() => setTab("content")}>内容</button><button role="tab" aria-selected={tab === "review"} onClick={() => setTab("review")}>审核</button><button role="tab" aria-selected={tab === "publish"} onClick={() => setTab("publish")}>发布</button></div>
    <div className="article-layout">
      <article className={`article-preview mobile-${tab}`}><p className="eyebrow">文章预览</p><h1>{article.metadata.title}</h1><p className="article-description">{article.metadata.description}</p><div className="prose" dangerouslySetInnerHTML={{ __html: sanitizePreviewHTML(article.preview_html) }} /></article>
      <aside className={`review-panel mobile-${tab}`}>
        <MetadataForm value={article.metadata} sourceChanged={article.source_changed} categoryOptions={categoryOptions} seriesOptions={seriesOptions} tagOptions={tagOptions} taxonomyState={taxonomyState} canCreateTaxonomy={canCreateTaxonomy} onCreateTaxonomy={(kind, select) => setTaxonomySelection({ kind, select })} externalSuggestion={externalSuggestion} onReload={load} onSave={async (metadata) => { const next = await saveMetadata(article.id, metadata); setArticle(next); setNotice("元数据已保存"); }} />
        <Checks items={article.checks} />
        {article.ai_configured ? <AISuggestions stale={article.suggestions_stale} suggestions={article.suggestions} generating={generatingAI} onAccept={(suggestion) => setExternalSuggestion({ id: `${suggestion.id}:${Date.now()}`, field: suggestion.field, value: suggestion.name })} onGenerate={async () => { setGeneratingAI(true); try { const result = await generateArticleSuggestions(article.id); setArticle({ ...article, ...result }); setNotice("AI Tag 已生成"); } catch (reason) { setNotice(reason instanceof Error ? reason.message : "AI Tag 生成失败"); } finally { setGeneratingAI(false); } }} /> : <section className="tool-section ai-unconfigured"><Bot size={17} /><span><b>AI 建议未启用</b><small>不影响手工审核</small></span><button onClick={() => onNavigate("/settings")}>配置 AI</button></section>}
        {!hidePrimaryForHugoFlow && <div className="primary-action"><button className="primary" onClick={() => void primary.action()}><PrimaryIcon size={17} />{primary.label}</button>{notice && <span role="status">{notice}</span>}</div>}
        {showHugoFlow && article.hugo_state !== "已同步" && <HugoPublishFlow articleID={article.id} contentHash={article.content_version} onPublished={async () => { setShowHugoFlow(false); setHistoryRefreshKey((value) => value + 1); await load(); }} />}
        <PublicationHistory articleID={article.id} refreshKey={historyRefreshKey} />
      </aside>
    </div>
    {taxonomy && taxonomySelection && <CreateTaxonomyTermDialog overview={taxonomy} kind={taxonomySelection.kind} noun={taxonomySelection.kind === "category" ? "类目" : "系列"} onClose={() => setTaxonomySelection(null)} onApplied={setTaxonomy} onCreated={(name) => { taxonomySelection.select(name); setTaxonomySelection(null); }} />}
  </div>;
}
