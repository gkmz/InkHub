import { CloudUpload } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getArticle } from "../../api/client";
import { previewHasHeading } from "../../api/safeHTML";
import type { ArticleDetail } from "../../api/types";
import { HugoPublishFlow } from "../../components/HugoPublishFlow";
import { MarkdownPreview } from "../../components/MarkdownPreview";
import { PublicationPageFrame } from "../../components/PublicationPageFrame";
import { PublicationHistory } from "../../components/PublicationHistory";

/** HugoPage 将 Hugo 目录选择、预览和交付放在独立渠道页面中。 */
export function HugoPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const load = useCallback(() => getArticle(articleID).then(setArticle), [articleID]);
  useEffect(() => { void load(); }, [load]);

  if (!article) return <div className="page-state">正在打开 Hugo 发布页…</div>;
  const available = article.content_stage !== "draft" && article.review_state === "已通过";
  const showMetadataTitle = !previewHasHeading(article.preview_html);
  return <div className="hugo-page">
    <PublicationPageFrame article={article} active="hugo" onNavigate={onNavigate}>
      <main className="hugo-page-main">
        <div className="hugo-page-layout">
          <aside className="hugo-page-sidebar" aria-label="Hugo 发布操作">
            <section className="hugo-page-intro"><CloudUpload size={20} /><div><p className="eyebrow">独立渠道</p><h2>同步到 Hugo</h2><p>选择发布目录，生成预览并确认写入博客。这个操作不会影响微信和小红书。</p></div></section>
            {!available ? <section className="channel-locked" role="status"><strong>审核通过后才能同步到 Hugo</strong><p>请先返回审核中心完善元数据并完成审核。</p><button className="secondary" type="button" onClick={() => onNavigate(`/articles/${articleID}`)}>返回审核中心</button></section> : <HugoPublishFlow articleID={article.id} contentHash={article.content_version} onPublished={async () => { setRefreshKey((value) => value + 1); await load(); }} />}
            <PublicationHistory articleID={articleID} refreshKey={refreshKey} />
          </aside>
          <article className="hugo-document" aria-label="Hugo 发布内容">
            <p className="eyebrow">Hugo 页面预览</p>
            {showMetadataTitle && <h1>{article.metadata.title}</h1>}
            <p className="hugo-description">{article.metadata.description}</p>
            <MarkdownPreview html={article.preview_html} className="prose" />
          </article>
        </div>
      </main>
    </PublicationPageFrame>
  </div>;
}
