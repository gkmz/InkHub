import { AlertTriangle, ArrowLeft, Bot, Check } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { generateArticleSuggestions, getArticle, getSuggestionHistory, getSuggestionVersion, getTaxonomyOverview, reviewArticle, saveMetadata, updateSuggestionItems } from "../../api/client";
import { previewHasHeading } from "../../api/safeHTML";
import type { ArticleDetail, ArticleMetadata, PublicationChannel, SuggestionHistoryItem, SuggestionVersionView, TaxonomyOverview } from "../../api/types";
import { AISuggestions } from "../../components/AISuggestions";
import { Checks } from "../../components/Checks";
import { MetadataForm } from "../../components/MetadataForm";
import type { TaxonomyFieldState } from "../../components/SingleTaxonomyField";
import { PublicationTrack } from "../../components/PublicationTrack";
import { PublicationHistory } from "../../components/PublicationHistory";
import { PublicationChannelNav } from "../../components/PublicationChannelNav";
import { SuggestionHistory } from "../../components/SuggestionHistory";
import { CreateTaxonomyTermDialog } from "../taxonomy/CreateTaxonomyTermDialog";
import { MarkdownPreview } from "../../components/MarkdownPreview";

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
  const [externalSuggestions, setExternalSuggestions] = useState<Array<{ id: string; field: keyof ArticleMetadata; value: string | string[] }>>([]);
  const [suggestionHistoryOpen, setSuggestionHistoryOpen] = useState(false);
  const [suggestionHistoryLoaded, setSuggestionHistoryLoaded] = useState(false);
  const [suggestionHistoryItems, setSuggestionHistoryItems] = useState<SuggestionHistoryItem[]>([]);
  const [selectedSuggestionVersion, setSelectedSuggestionVersion] = useState<SuggestionVersionView | null>(null);
  const [suggestionHistoryLoading, setSuggestionHistoryLoading] = useState(false);
  const [suggestionVersionLoading, setSuggestionVersionLoading] = useState(false);
  const [suggestionHistoryError, setSuggestionHistoryError] = useState("");
  const load = useCallback(() => getArticle(articleID).then(setArticle).catch((reason: Error) => setNotice(reason.message)), [articleID]);
  useEffect(() => { setExternalSuggestions([]); void load(); }, [load]);
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
  const loadSuggestionHistory = async () => {
    setSuggestionHistoryLoading(true);
    setSuggestionHistoryError("");
    try {
      const result = await getSuggestionHistory(articleID);
      setSuggestionHistoryItems(result.items);
      setSuggestionHistoryLoaded(true);
    } catch (reason) {
      setSuggestionHistoryError(reason instanceof Error ? reason.message : "建议历史读取失败");
    } finally {
      setSuggestionHistoryLoading(false);
    }
  };
  const openSuggestionHistory = () => {
    setSuggestionHistoryOpen(true);
    if (!suggestionHistoryLoaded && !suggestionHistoryLoading) void loadSuggestionHistory();
  };
  const selectSuggestionVersion = async (suggestionID: string) => {
    setSuggestionVersionLoading(true);
    try {
      setSelectedSuggestionVersion(await getSuggestionVersion(articleID, suggestionID));
    } catch (reason) {
      setSuggestionHistoryError(reason instanceof Error ? reason.message : "建议版本读取失败");
    } finally {
      setSuggestionVersionLoading(false);
    }
  };
  const updateSuggestionAction = async (action: "accepted" | "ignored", suggestionIDs: string[]) => {
    // 兼容旧版生成接口暂时缺少 suggestions_id 的响应，真实版本始终由后端返回该 ID。
    if (!article?.suggestions_id) return;
    await updateSuggestionItems(articleID, article.suggestions_id, action, suggestionIDs);
    setArticle((current) => current ? { ...current, suggestions: current.suggestions.filter((suggestion) => !suggestionIDs.includes(suggestion.id)) } : current);
    setSuggestionHistoryLoaded(false);
    if (suggestionHistoryOpen) void loadSuggestionHistory();
  };
  if (!article) return <div className="page-state">{notice || "正在打开文章…"}</div>;
  // 旧版本或降级响应可能缺少 terms，页面必须退化为空候选而不是白屏。
  const taxonomyTerms = Array.isArray(taxonomy?.terms) ? taxonomy.terms : [];
  // 兼容旧版 API 响应，资源诊断缺省时视为空数组。
  const resourceDiagnostics = Array.isArray(article.resource_diagnostics) ? article.resource_diagnostics : [];
  const categoryOptions = taxonomyTerms.filter((term) => term.kind === "category").map((term) => ({ key: term.key, name: term.name }));
  const seriesOptions = taxonomyTerms.filter((term) => term.kind === "series").map((term) => ({ key: term.key, name: term.name }));
  const tagOptions = taxonomyTerms.filter((term) => term.kind === "tag").map((term) => ({ key: term.key, name: term.name, usageCount: term.usage_count }));
  const canCreateTaxonomy = Boolean(taxonomy && !taxonomy.readonly && taxonomy.provider_id && taxonomy.revision);
  const isDraft = article.content_stage === "draft";
  const showMetadataTitle = !previewHasHeading(article.preview_html);
  const primary = isDraft || !article.stable_id || article.review_state === "已通过" ? null : { label: "审核通过", icon: Check, action: async () => { await reviewArticle(article.id); await load(); setNotice("审核已通过，可以选择发布渠道"); } };
  const PrimaryIcon = primary?.icon;
  return <div className="article-page">
    <div className="article-toolbar"><button type="button" onClick={() => onNavigate("/library")}><ArrowLeft size={16} />返回内容库</button><span>{article.relative_path}</span></div>
    {isDraft && <section className="draft-guidance" role="status"><strong>草稿</strong><span>文章仍在创作阶段，不会进入审核或发布流程。</span><code>publish.status: ready</code>{article.content_stage_issue && <small>{article.content_stage_issue}</small>}</section>}
    {resourceDiagnostics.length > 0 && <section className="resource-guidance" role="status"><AlertTriangle size={16} /><div><strong>图片引用需要处理</strong>{resourceDiagnostics.map((diagnostic) => <p key={`${diagnostic.code}:${diagnostic.message}`}>{diagnostic.message}</p>)}<small>文章内容阶段不受影响，但发布前必须修复这些引用。</small></div></section>}
    <PublicationTrack review={article.review_state} hugo={article.hugo_state} wechat={article.wechat_state} xiaohongshu={article.xiaohongshu_state ?? "尚未准备"} />
    {!isDraft && <section className="publication-center" aria-label="发布渠道"><div className="section-heading"><div><p className="eyebrow">审核完成后可独立处理</p><h2>发布渠道</h2></div><span>{article.review_state === "已通过" ? "请选择需要处理的渠道" : "先完成审核"}</span></div><PublicationChannelNav article={article} active="review" onNavigate={onNavigate} /></section>}
    {article.disposition && <p className={`article-disposition state-${article.disposition.kind}`}>
      {article.disposition.kind === "ignored"
        ? "此文章已忽略，可在内容库恢复"
        : `当前版本已标记为外部发表：${article.disposition.channels.map((channel: PublicationChannel) => channel === "hugo" ? "Hugo" : channel === "wechat" ? "微信" : "小红书").join("、")}`}
    </p>}
    <div className="mobile-tabs" role="tablist" aria-label="文章详情"><button role="tab" aria-selected={tab === "content"} onClick={() => setTab("content")}>内容</button><button role="tab" aria-selected={tab === "review"} onClick={() => setTab("review")}>审核</button><button role="tab" aria-selected={tab === "publish"} onClick={() => setTab("publish")}>发布</button></div>
    <div className="article-layout">
      <article className={`article-preview mobile-${tab}`}><p className="eyebrow">文章预览</p>{showMetadataTitle && <h1>{article.metadata.title}</h1>}<p className="article-description">{article.metadata.description}</p><MarkdownPreview html={article.preview_html} className="prose" /></article>
      <aside className={`review-panel mobile-${tab}`}>
        {!isDraft && primary && PrimaryIcon && <div className="review-command-bar"><div><span className="eyebrow">审核</span><strong>{primary.label}</strong><small>确认元数据和检查结果后完成审核</small></div><div className="review-command-actions"><button className="primary" onClick={() => void primary.action()}><PrimaryIcon size={17} />{primary.label}</button></div>{notice && <span className="review-command-notice" role="status">{notice}</span>}</div>}
        <MetadataForm value={article.metadata} sourceChanged={article.source_changed} identityReady={Boolean(article.stable_id)} categoryOptions={categoryOptions} seriesOptions={seriesOptions} tagOptions={tagOptions} taxonomyState={taxonomyState} canCreateTaxonomy={canCreateTaxonomy} onCreateTaxonomy={(kind, select) => setTaxonomySelection({ kind, select })} externalSuggestions={externalSuggestions} onReload={() => { setExternalSuggestions([]); void load(); }} onSave={async (metadata) => { const next = await saveMetadata(article.id, metadata); setArticle(next); setExternalSuggestions([]); setNotice("元数据已保存"); }} />
        <Checks items={article.checks} />
        {article.ai_configured ? <><AISuggestions stale={article.suggestions_stale} suggestions={article.suggestions} generating={generatingAI} historyCount={suggestionHistoryItems.length} onOpenHistory={openSuggestionHistory} onAction={updateSuggestionAction} onAccept={(suggestion) => setExternalSuggestions((current) => [...current, { id: `${suggestion.id}:${Date.now()}`, field: suggestion.field, value: suggestion.value ?? suggestion.name }])} onGenerate={async () => { setExternalSuggestions([]); setGeneratingAI(true); try { const result = await generateArticleSuggestions(article.id); setArticle({ ...article, ...result }); setSuggestionHistoryLoaded(false); setNotice("AI 建议已生成"); } catch (reason) { setNotice(reason instanceof Error ? reason.message : "AI 建议生成失败"); } finally { setGeneratingAI(false); } }} />{suggestionHistoryOpen && <SuggestionHistory items={suggestionHistoryItems} selected={selectedSuggestionVersion} loading={suggestionHistoryLoading} detailLoading={suggestionVersionLoading} error={suggestionHistoryError} onSelect={(id) => void selectSuggestionVersion(id)} onRetry={() => void loadSuggestionHistory()} onClose={() => setSuggestionHistoryOpen(false)} />}</> : <section className="tool-section ai-unconfigured"><Bot size={17} /><span><b>AI 建议未启用</b><small>不影响手工审核</small></span><button onClick={() => onNavigate("/settings")}>配置 AI</button></section>}
        <PublicationHistory articleID={article.id} refreshKey={0} />
      </aside>
    </div>
    {taxonomy && taxonomySelection && <CreateTaxonomyTermDialog overview={taxonomy} kind={taxonomySelection.kind} noun={taxonomySelection.kind === "category" ? "类目" : "系列"} onClose={() => setTaxonomySelection(null)} onApplied={setTaxonomy} onCreated={(name) => { taxonomySelection.select(name); setTaxonomySelection(null); }} />}
  </div>;
}
