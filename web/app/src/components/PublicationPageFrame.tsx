import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import type { ArticleDetail } from "../api/types";
import { PublicationChannelNav } from "./PublicationChannelNav";
import { PublicationTrack } from "./PublicationTrack";

interface PublicationPageFrameProps {
  article: ArticleDetail;
  active: "hugo" | "wechat" | "xiaohongshu";
  onNavigate: (path: string) => void;
  toolbarContent?: ReactNode;
  children: ReactNode;
}

/** PublicationPageFrame 让各发布渠道复用审核页的文章上下文和渠道导航。 */
export function PublicationPageFrame({ article, active, onNavigate, toolbarContent, children }: PublicationPageFrameProps) {
  return <div className="article-page publication-page">
    <div className="article-toolbar">
      <button type="button" onClick={() => onNavigate("/library")}><ArrowLeft size={16} />返回内容库</button>
      <span>{article.relative_path}</span>
      {toolbarContent && <div className="publication-toolbar-tools">{toolbarContent}</div>}
    </div>
    <PublicationTrack review={article.review_state} hugo={article.hugo_state} wechat={article.wechat_state} xiaohongshu={article.xiaohongshu_state ?? "尚未准备"} />
    <section className="publication-center" aria-label="发布渠道">
      <div className="section-heading"><div><p className="eyebrow">审核完成后可独立处理</p><h2>发布渠道</h2></div><span>{article.review_state === "已通过" ? "请选择需要处理的渠道" : "先完成审核"}</span></div>
      <PublicationChannelNav article={article} active={active} onNavigate={onNavigate} />
    </section>
    {children}
  </div>;
}
