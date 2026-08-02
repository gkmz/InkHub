import { ArrowLeft, Download, History, Image as ImageIcon, RefreshCw, Save, Send, Sparkles } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { getArticle, getXiaohongshu, generateXiaohongshuDraft, markXiaohongshuPublished, saveXiaohongshuDraft, saveXiaohongshuRender } from "../../api/client";
import { sanitizePreviewHTML } from "../../api/safeHTML";
import type { ArticleDetail, XiaohongshuDraft, XiaohongshuView } from "../../api/types";
import { adaptXiaohongshuHTML } from "./xiaohongshuAdapter";

const VIEWPORT_WIDTH = 375;
const PAGE_HEIGHT = 667;

/** XiaohongshuPage 提供完整草稿编辑、手机模板预览、图片导出和人工发布确认。 */
export function XiaohongshuPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [view, setView] = useState<XiaohongshuView | null>(null);
  const [draft, setDraft] = useState<XiaohongshuDraft | null>(null);
  const [template, setTemplate] = useState("mobile-clean");
  const [saving, setSaving] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [message, setMessage] = useState("");
  const [showHistory, setShowHistory] = useState(false);
  const previewRef = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    const [nextArticle, nextView] = await Promise.all([getArticle(articleID), getXiaohongshu(articleID)]);
    setArticle(nextArticle); setView(nextView); setDraft(nextView.latest);
  }, [articleID]);
  useEffect(() => { void load(); }, [load]);

  const pageCount = useMemo(() => {
    const text = draft?.body_html ?? "";
    return Math.max(1, Math.ceil(Math.max(text.length * 0.42, PAGE_HEIGHT) / PAGE_HEIGHT));
  }, [draft?.body_html]);

  if (!article || !view) return <div className="page-state">正在打开小红书内容中心…</div>;
  const update = (patch: Partial<XiaohongshuDraft>) => setDraft((current) => current ? { ...current, ...patch } : current);
  const generate = async () => {
    if (!window.confirm("重新生成会创建新的小红书草稿版本，旧版本会保留。继续吗？")) return;
    setGenerating(true); setMessage("");
    try { const next = await generateXiaohongshuDraft(articleID); setDraft(next); setView((current) => current ? { ...current, latest: next, history: [next, ...current.history] } : current); setMessage("已生成新草稿，旧版本仍保留在历史中"); }
    catch (error) { setMessage(error instanceof Error ? error.message : "生成失败"); }
    finally { setGenerating(false); }
  };
  const save = async () => {
    if (!draft) return;
    setSaving(true); setMessage("");
    try { const next = await saveXiaohongshuDraft(articleID, draft); setDraft(next); setMessage("草稿已保存"); }
    catch (error) { setMessage(error instanceof Error ? error.message : "保存失败"); }
    finally { setSaving(false); }
  };
  const exportImages = async () => {
    if (!draft || !previewRef.current) return;
    const adaptation = adaptXiaohongshuHTML(draft.body_html, VIEWPORT_WIDTH - 44);
    const html = adaptation.html;
    const pages = Math.max(1, Math.ceil((previewRef.current.scrollHeight || PAGE_HEIGHT) / PAGE_HEIGHT));
    const hash = await digest(html);
    await saveXiaohongshuRender(articleID, { draft_id: draft.id, template_id: template, template_version: "1", viewport_width: VIEWPORT_WIDTH, page_height: PAGE_HEIGHT, html_hash: hash, page_count: pages });
    for (let index = 0; index < pages; index += 1) {
      const dataURL = await snapshotPage(html, draft.title, index, pages);
      const link = document.createElement("a"); link.href = dataURL; link.download = `xiaohongshu-${draft.id}-${String(index + 1).padStart(2, "0")}.png`; link.click();
    }
    setMessage(`已导出 ${pages} 张图片${adaptation.convertedTables > 0 ? `，${adaptation.convertedTables} 个宽表格已转为文本` : ""}；标题和文案请在 InkHub 中复制到小红书`);
  };
  const publish = async () => {
    if (!draft || !window.confirm("确认图片已手动上传到小红书，并将此版本标记为已发布？")) return;
    try { await markXiaohongshuPublished(articleID, draft.id); await load(); setMessage("已记录小红书发布"); }
    catch (error) { setMessage(error instanceof Error ? error.message : "发布确认失败"); }
  };
  return <div className="xiaohongshu-page">
    <header className="xiaohongshu-toolbar"><button className="back" onClick={() => onNavigate(`/articles/${articleID}`)}><ArrowLeft size={16} />返回文章</button><div className="xiaohongshu-heading"><Sparkles size={17} /><strong>小红书内容中心</strong><span>{view.state}</span></div><div className="xiaohongshu-actions"><button className="secondary" onClick={() => setShowHistory((value) => !value)}><History size={15} />历史</button><button className="secondary" onClick={() => void generate()} disabled={generating}><RefreshCw size={15} />{generating ? "生成中" : "重新生成"}</button></div></header>
    <div className="xiaohongshu-layout">
      <section className="xiaohongshu-editor"><div className="tool-heading"><h2>完整草稿</h2><span>整体编辑后手动保存</span></div>
        {!draft ? <div className="empty-state compact"><Sparkles size={24} /><p>还没有小红书草稿</p><button className="primary" onClick={() => void generate()}><Sparkles size={15} />生成草稿</button></div> : <>
          <label>标题<input value={draft.title} onChange={(event) => update({ title: event.target.value })} /></label>
          <label>正文 HTML<textarea className="xiaohongshu-body-input" value={draft.body_html} onChange={(event) => update({ body_html: event.target.value })} /></label>
          <label>话题（每行一个）<textarea value={draft.topics.join("\n")} onChange={(event) => update({ topics: event.target.value.split("\n").map((item) => item.trim()).filter(Boolean) })} /></label>
          <label>来源说明<textarea value={draft.source_note} onChange={(event) => update({ source_note: event.target.value })} /></label>
          <label>评论区文案<textarea value={draft.comment_copy} onChange={(event) => update({ comment_copy: event.target.value })} /></label>
          <div className="xiaohongshu-editor-actions"><button className="primary" onClick={() => void save()} disabled={saving}><Save size={15} />{saving ? "保存中" : "保存草稿"}</button><button className="secondary" onClick={() => void publish()} disabled={draft.stale || draft.state === "published"}><Send size={15} />标记已发布</button></div>
          {message && <p className="inline-status">{message}</p>}
        </>}
      </section>
      <section className="xiaohongshu-preview"><div className="tool-heading"><h2><ImageIcon size={16} />手机预览</h2><label className="template-select">模板<select value={template} onChange={(event) => setTemplate(event.target.value)}><option value="mobile-clean">Mobile Clean</option><option value="mobile-paper">Mobile Paper</option></select></label></div><p className="xiaohongshu-preview-hint">固定 375 × 667 手机视口，代码自动换行；宽表格会转为结构化文本。</p><div className={`xiaohongshu-phone template-${template}`} ref={previewRef}><h1>{draft?.title || article.metadata.title}</h1><div className="xiaohongshu-rendered" dangerouslySetInnerHTML={{ __html: sanitizePreviewHTML(draft?.body_html || article.preview_html) }} /></div><div className="xiaohongshu-export"><span>预计 {pageCount} 页图片</span><button className="primary" onClick={() => void exportImages()} disabled={!draft}><Download size={15} />导出图片集</button></div></section>
    </div>
    {showHistory && <aside className="xiaohongshu-history"><div className="tool-heading"><h2>版本历史</h2><button className="back" onClick={() => setShowHistory(false)}>关闭</button></div>{view.history.map((item) => <button key={item.id} className={`xiaohongshu-history-item${draft?.id === item.id ? " active" : ""}`} onClick={() => { setDraft(item); setShowHistory(false); }}><strong>{item.title || "未命名草稿"}</strong><span>{item.state}{item.stale ? " · 内容已更新" : ""}</span><time>{new Intl.DateTimeFormat("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(item.created_at))}</time></button>)}</aside>}
  </div>;
}

async function digest(value: string) { const data = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value)); return Array.from(new Uint8Array(data)).map((byte) => byte.toString(16).padStart(2, "0")).join(""); }

async function snapshotPage(html: string, title: string, index: number, pages: number) {
  const source = `<div xmlns="http://www.w3.org/1999/xhtml" style="box-sizing:border-box;width:${VIEWPORT_WIDTH}px;height:${PAGE_HEIGHT}px;overflow:hidden;background:#fff"><div style="box-sizing:border-box;width:${VIEWPORT_WIDTH}px;min-height:${pages * PAGE_HEIGHT}px;padding:28px 22px;background:#fff;color:#17201b;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;transform:translateY(-${index * PAGE_HEIGHT}px)"><h1 style="font-size:24px;line-height:1.3;margin:0 0 18px">${escapeText(title)}</h1><div style="font-size:15px;line-height:1.75;overflow-wrap:anywhere;white-space:normal">${html}</div><small style="display:block;margin-top:18px;color:#849087">${index + 1} / ${pages}</small></div></div>`;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${VIEWPORT_WIDTH}" height="${PAGE_HEIGHT}"><foreignObject width="100%" height="100%">${source}</foreignObject></svg>`;
  const image = new Image(); image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`; await new Promise<void>((resolve, reject) => { image.onload = () => resolve(); image.onerror = () => reject(new Error("图片渲染失败")); });
  const canvas = document.createElement("canvas"); canvas.width = VIEWPORT_WIDTH * 2; canvas.height = PAGE_HEIGHT * 2; const context = canvas.getContext("2d"); if (!context) throw new Error("无法创建图片画布"); context.scale(2, 2); context.drawImage(image, 0, 0, VIEWPORT_WIDTH, PAGE_HEIGHT); return canvas.toDataURL("image/png");
}

function escapeText(value: string) { return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char] ?? char)); }
