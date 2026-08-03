import { ArrowLeft, CloudUpload } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getArticle } from "../../api/client";
import type { ArticleDetail } from "../../api/types";
import { HugoPublishFlow } from "../../components/HugoPublishFlow";
import { PublicationChannelNav } from "../../components/PublicationChannelNav";
import { PublicationHistory } from "../../components/PublicationHistory";

/** HugoPage 将 Hugo 目录选择、预览和交付放在独立渠道页面中。 */
export function HugoPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const load = useCallback(() => getArticle(articleID).then(setArticle), [articleID]);
  useEffect(() => { void load(); }, [load]);

  if (!article) return <div className="page-state">正在打开 Hugo 发布页…</div>;
  const available = article.content_stage !== "draft" && article.review_state === "已通过";
  return <div className="hugo-page">
    <header className="channel-page-header"><button className="back" type="button" onClick={() => onNavigate(`/articles/${articleID}`)}><ArrowLeft size={16} />返回审核</button><div><p className="eyebrow">Hugo 发布</p><h1>{article.metadata.title}</h1><small>{article.review_state} · {article.hugo_state}</small></div></header>
    <PublicationChannelNav article={article} active="hugo" onNavigate={onNavigate} />
    <main className="hugo-page-main">
      <section className="hugo-page-intro"><CloudUpload size={20} /><div><p className="eyebrow">独立渠道</p><h2>同步到 Hugo</h2><p>选择发布目录，生成预览并确认写入博客。这个操作不会影响微信和小红书。</p></div></section>
      {!available ? <section className="channel-locked" role="status"><strong>审核通过后才能同步到 Hugo</strong><p>请先返回审核中心完善元数据并完成审核。</p><button className="secondary" type="button" onClick={() => onNavigate(`/articles/${articleID}`)}>返回审核中心</button></section> : <HugoPublishFlow articleID={article.id} contentHash={article.content_version} onPublished={async () => { setRefreshKey((value) => value + 1); await load(); }} />}
      <PublicationHistory articleID={articleID} refreshKey={refreshKey} />
    </main>
  </div>;
}
