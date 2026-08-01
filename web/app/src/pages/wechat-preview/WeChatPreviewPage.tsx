import { ArrowLeft, Image, LayoutTemplate } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { confirmWeChatDraft, getArticle, getPreparedWeChatHTML, markWeChatCopied } from "../../api/client";
import type { ArticleDetail } from "../../api/types";
import { WeChatActions } from "../../components/WeChatActions";
import { MarkdownPreview } from "../../components/MarkdownPreview";
import { WeChatPlan } from "./WeChatPlan";

/** WeChatPreviewPage 展示最终模板效果并保持复制和草稿确认分离。 */
export function WeChatPreviewPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [template, setTemplate] = useState("default");
  const [preparedHTML, setPreparedHTML] = useState("");
  const [preparing, setPreparing] = useState(false);
  const timer = useRef<number | null>(null);
  const mounted = useRef(true);
  useEffect(() => { void getArticle(articleID).then(setArticle); }, [articleID]);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; if (timer.current !== null) window.clearTimeout(timer.current); };
  }, [articleID]);
  if (!article) return <div className="page-state">正在准备微信预览…</div>;
  const pollPrepared = async () => {
    const current = await getArticle(articleID);
    if (!mounted.current) return;
    setArticle(current);
    if (current.wechat_state.includes("已准备") || current.wechat_state.includes("已复制") || current.wechat_state.includes("已确认")) {
      const prepared = await getPreparedWeChatHTML(current.id);
      if (mounted.current) { setPreparedHTML(prepared.html); setPreparing(false); }
      return;
    }
    if (current.wechat_state.includes("失败")) { setPreparing(false); return; }
    timer.current = window.setTimeout(() => void pollPrepared(), 500);
  };
  return <div className="wechat-page"><header><button onClick={() => onNavigate(`/articles/${articleID}`)}><ArrowLeft size={16} />返回文章</button><label><LayoutTemplate size={16} />模板<select value={template} disabled={preparing || preparedHTML !== ""} onChange={(event) => setTemplate(event.target.value)}><option value="default">InkHub Default</option><option value="minimal">InkHub Minimal</option></select></label>{preparedHTML && <WeChatActions copied={article.wechat_copied} onCopy={async () => { await navigator.clipboard?.writeText(preparedHTML); await markWeChatCopied(article); }} onConfirm={async () => { await confirmWeChatDraft(article); }} />}</header><main><aside>{preparing ? <p><Image size={16} />正在上传图片并生成微信内容…</p> : !preparedHTML ? <WeChatPlan articleID={articleID} templateID={template} onConfirmed={() => { setPreparing(true); void pollPrepared(); }} /> : <><h2>内容已准备</h2><p><Image size={16} />图片与模板处理完成</p></>}</aside><article className={`wechat-document template-${template}`}><h1>{article.metadata.title}</h1><p className="wechat-description">{article.metadata.description}</p><MarkdownPreview html={preparedHTML || article.preview_html} /></article></main></div>;
}
