import { Image, LayoutTemplate, Palette } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { confirmWeChatDraft, getArticle, getPreparedWeChatHTML, markWeChatCopied } from "../../api/client";
import { previewHasHeading } from "../../api/safeHTML";
import type { ArticleDetail, MermaidTheme } from "../../api/types";
import { WeChatActions } from "../../components/WeChatActions";
import { MarkdownPreview } from "../../components/MarkdownPreview";
import { WeChatPlan } from "./WeChatPlan";
import { PublicationPageFrame } from "../../components/PublicationPageFrame";
import { copyFormattedHTML } from "../../platform/clipboard";
import { formatWeChatReferences } from "./wechatReferences";

/** WeChatPreviewPage 展示最终模板效果并保持复制和草稿确认分离。 */
export function WeChatPreviewPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [mermaidTheme, setMermaidTheme] = useState<MermaidTheme>("handdrawn");
  const [preparedHTML, setPreparedHTML] = useState("");
  const [preparing, setPreparing] = useState(false);
  const timer = useRef<number | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    // 刷新已准备页面时直接恢复微信产物，不能退回未经转换的通用预览。
    void getArticle(articleID).then(async (current) => {
      if (!mounted.current) return;
      setArticle(current);
      if (isPreparedState(current.wechat_state)) {
        const prepared = await getPreparedWeChatHTML(current.id);
        if (mounted.current) setPreparedHTML(formatWeChatReferences(prepared.html));
      }
    });
  }, [articleID]);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; if (timer.current !== null) window.clearTimeout(timer.current); };
  }, [articleID]);
  const previewHTML = useMemo(() => formatWeChatReferences(preparedHTML || article?.preview_html || ""), [article?.preview_html, preparedHTML]);
  if (!article) return <div className="page-state">正在准备微信预览…</div>;
  const showMetadataTitle = !previewHasHeading(previewHTML);
  const pollPrepared = async () => {
    const current = await getArticle(articleID);
    if (!mounted.current) return;
    setArticle(current);
    if (isPreparedState(current.wechat_state)) {
      const prepared = await getPreparedWeChatHTML(current.id);
      if (mounted.current) { setPreparedHTML(formatWeChatReferences(prepared.html)); setPreparing(false); }
      return;
    }
    if (current.wechat_state.includes("失败")) { setPreparing(false); return; }
    timer.current = window.setTimeout(() => void pollPrepared(), 500);
  };
  const settingsDisabled = preparing || preparedHTML !== "";
  const deliveryActions = <WeChatActions
    html={preparedHTML}
    copied={article.wechat_copied}
    onCopy={async () => { await copyFormattedHTML(preparedHTML); await markWeChatCopied(article); }}
    onConfirm={async () => { await confirmWeChatDraft(article); }}
  />;

  return <div className="wechat-page"><PublicationPageFrame article={article} active="wechat" onNavigate={onNavigate}>
    {article.review_state !== "已通过" ? <main><section className="channel-locked" role="status"><h2>审核通过后才能准备微信内容</h2><p>请先返回审核中心完善元数据并完成审核。</p><button className="secondary" type="button" onClick={() => onNavigate(`/articles/${articleID}`)}>返回审核中心</button></section></main> : <main>
      <aside>
        <section className="wechat-content-settings" aria-label="微信内容设置">
          <label><span><LayoutTemplate size={15} />排版模板</span><select value="default" disabled aria-label="排版模板"><option value="default">InkHub 墨绿</option></select></label>
          <div className="wechat-mermaid-setting"><span><Palette size={15} />Mermaid 样式</span><div className="segmented-control" role="group" aria-label="Mermaid 样式">{(["handdrawn", "modern"] as MermaidTheme[]).map((theme) => <button key={theme} type="button" aria-pressed={mermaidTheme === theme} disabled={settingsDisabled} onClick={() => setMermaidTheme(theme)}>{theme === "handdrawn" ? "手绘" : "现代"}</button>)}</div></div>
        </section>
        {preparing ? <p><Image size={16} />正在上传图片并生成微信内容…</p> : !preparedHTML ? <WeChatPlan articleID={articleID} templateID="default" mermaidTheme={mermaidTheme} onConfirmed={() => { setPreparing(true); void pollPrepared(); }} /> : <section className="wechat-ready"><h2>内容已准备</h2><p><Image size={16} />图片与模板处理完成</p>{deliveryActions}</section>}
      </aside>
      <article className="wechat-document template-green">{showMetadataTitle && <h1>{article.metadata.title}</h1>}<p className="wechat-description">{article.metadata.description}</p><MarkdownPreview html={previewHTML} mermaidTheme={mermaidTheme} /></article>
    </main>}
  </PublicationPageFrame></div>;
}

function isPreparedState(value: string) {
  return value.includes("已准备") || value.includes("已复制") || value.includes("已确认");
}
