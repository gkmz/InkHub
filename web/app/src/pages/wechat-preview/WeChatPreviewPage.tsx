import { ArrowLeft, Image, LayoutTemplate } from "lucide-react";
import { useEffect, useState } from "react";
import { confirmWeChatDraft, getArticle, getJob, getPreparedWeChatHTML, markWeChatCopied, startPublication } from "../../api/client";
import { sanitizePreviewHTML } from "../../api/safeHTML";
import type { ArticleDetail } from "../../api/types";
import { WeChatActions } from "../../components/WeChatActions";

/** WeChatPreviewPage 展示最终模板效果并保持复制和草稿确认分离。 */
export function WeChatPreviewPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [template, setTemplate] = useState("default");
  useEffect(() => { void getArticle(articleID).then(setArticle); }, [articleID]);
  if (!article) return <div className="page-state">正在准备微信预览…</div>;
  const safeHTML = sanitizePreviewHTML(article.preview_html);
  return <div className="wechat-page"><header><button onClick={() => onNavigate(`/articles/${articleID}`)}><ArrowLeft size={16} />返回文章</button><label><LayoutTemplate size={16} />模板<select value={template} onChange={(event) => setTemplate(event.target.value)}><option value="default">InkHub Default</option><option value="minimal">InkHub Minimal</option></select></label><WeChatActions copied={article.wechat_copied} onCopy={async () => { const queued=await startPublication(article, "wechat"); await waitForJob(queued.job_id); const prepared=await getPreparedWeChatHTML(article.id); await navigator.clipboard?.writeText(prepared.html); await markWeChatCopied(article); }} onConfirm={async () => { await confirmWeChatDraft(article); }} /></header><main><aside><h2>准备清单</h2><p><Image size={16} />本篇没有需要上传的本地图片</p><p>模板：InkHub {template === "default" ? "Default" : "Minimal"}</p></aside><article className={`wechat-document template-${template}`}><h1>{article.metadata.title}</h1><p className="wechat-description">{article.metadata.description}</p><div dangerouslySetInnerHTML={{ __html: safeHTML }} /></article></main></div>;
}

async function waitForJob(jobID: string) {
  for (let attempt=0; attempt<80; attempt+=1) {
    const job=await getJob(jobID);
    if (job.state==="succeeded") return;
    if (job.state==="failed") throw new Error("微信内容准备失败");
    await new Promise((resolve)=>window.setTimeout(resolve,100));
  }
  throw new Error("微信内容准备超时");
}
