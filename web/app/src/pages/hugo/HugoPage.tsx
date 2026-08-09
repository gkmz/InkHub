import { CloudUpload } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getArticle } from "../../api/client";
import { previewHasHeading } from "../../api/safeHTML";
import type { ArticleDetail } from "../../api/types";
import { HugoPublishFlow } from "../../components/HugoPublishFlow";
import type { HugoRenderPreview } from "../../components/HugoPublishFlow";
import { MarkdownPreview } from "../../components/MarkdownPreview";
import { PublicationPageFrame } from "../../components/PublicationPageFrame";
import { PublicationHistory } from "../../components/PublicationHistory";
import { renderHugoPreviewMermaid } from "./renderHugoPreviewMermaid";

/** HugoPage 将 Hugo 目录选择、预览和交付放在独立渠道页面中。 */
export function HugoPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [renderPreview, setRenderPreview] = useState<HugoRenderPreview | null>(null);
  const load = useCallback(() => getArticle(articleID).then(setArticle), [articleID]);
  useEffect(() => { void load(); }, [load]);

  if (!article) return <div className="page-state">正在打开 Hugo 发布页…</div>;
  const available = article.content_stage !== "draft" && article.review_state === "已通过";
  const manualPublished = article.disposition?.kind === "published" && article.disposition.channels.includes("hugo");
  const showMetadataTitle = !previewHasHeading(article.preview_html);
  return <div className="hugo-page">
    <PublicationPageFrame article={article} active="hugo" onNavigate={onNavigate}>
      <main className="hugo-page-main">
        <div className="hugo-page-layout">
          <aside className="hugo-page-sidebar" aria-label="Hugo 发布操作">
            <section className="hugo-page-intro"><CloudUpload size={20} /><div><p className="eyebrow">独立渠道</p><h2>同步到 Hugo</h2><p>选择发布目录，生成预览并确认写入博客。这个操作不会影响微信和小红书。</p></div></section>
            {!available ? <section className="channel-locked" role="status"><strong>审核通过后才能同步到 Hugo</strong><p>请先返回审核中心完善元数据并完成审核。</p><button className="secondary" type="button" onClick={() => onNavigate(`/articles/${articleID}`)}>返回审核中心</button></section> : <HugoPublishFlow articleID={article.id} contentHash={article.content_version} manualPublished={manualPublished} onRenderPreviewChange={setRenderPreview} onPublished={async () => { setRefreshKey((value) => value + 1); await load(); }} />}
            <PublicationHistory articleID={articleID} refreshKey={refreshKey} />
          </aside>
          <article className={`hugo-document${renderPreview ? " hugo-render-mode" : ""}`} aria-label="Hugo 发布内容">
            {renderPreview ? <div className="hugo-render-document"><header><div><p className="eyebrow">Hugo 渲染</p><h2>当前文章渲染结果</h2></div><span>{renderPreview.published ? "已同步" : renderPreview.expired ? "已过期" : "待确认"}</span></header>{/* 允许同源资源加载，但不开放脚本执行能力。 */}<iframe title="Hugo 当前文章渲染预览" src={renderPreview.url} loading="lazy" sandbox="allow-same-origin" referrerPolicy="no-referrer" onLoad={(event) => { void renderHugoPreviewMermaid(event.currentTarget); }} /></div> : <><p className="eyebrow">Hugo 页面预览</p>{showMetadataTitle && <h1>{article.metadata.title}</h1>}<p className="hugo-description">{article.metadata.description}</p><MarkdownPreview html={article.preview_html} className="prose" /></>}
          </article>
        </div>
      </main>
    </PublicationPageFrame>
  </div>;
}
