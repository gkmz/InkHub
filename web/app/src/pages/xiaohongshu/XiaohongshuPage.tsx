import { Download, History, LayoutTemplate, RefreshCw, Save, Send, Sparkles } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getArticle, getXiaohongshu, generateXiaohongshuDraft, markXiaohongshuPublished, saveXiaohongshuDraft, saveXiaohongshuRender } from "../../api/client";
import type { ArticleDetail, XiaohongshuDraft, XiaohongshuView } from "../../api/types";
import { PublicationPageFrame } from "../../components/PublicationPageFrame";
import { stripXiaohongshuTitle } from "./xiaohongshuAdapter";
import { buildXiaohongshuPages, flattenXiaohongshuPages, getXiaohongshuTemplate, XIAOHONGSHU_TEMPLATES } from "./xiaohongshuLayout";
import { XiaohongshuCardEditor } from "./XiaohongshuCardEditor";

/** XiaohongshuPage 提供页面块编辑、模板切换、图片导出和人工发布确认。 */
export function XiaohongshuPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [view, setView] = useState<XiaohongshuView | null>(null);
  const [draft, setDraft] = useState<XiaohongshuDraft | null>(null);
  const [template, setTemplate] = useState("mobile-clean");
  const [saving, setSaving] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [message, setMessage] = useState("");
  const [showHistory, setShowHistory] = useState(false);

  const load = useCallback(async () => {
    const [nextArticle, nextView] = await Promise.all([getArticle(articleID), getXiaohongshu(articleID)]);
    setArticle(nextArticle);
    setView(nextView);
    setDraft(nextView.latest ? ensurePages(nextView.latest, nextArticle.preview_html, "mobile-clean") : null);
  }, [articleID]);

  useEffect(() => { void load(); }, [load]);

  if (!article || !view) return <div className="page-state">正在打开小红书内容中心…</div>;
  const update = (patch: Partial<XiaohongshuDraft>) => setDraft((current) => current ? { ...current, ...patch } : current);
  const changeTemplate = (nextTemplate: string) => {
    setTemplate(nextTemplate);
    setDraft((current) => {
      if (!current) return current;
      const html = flattenXiaohongshuPages(current.pages) || current.body_html;
      return { ...current, body_html: html, pages: buildXiaohongshuPages(html, nextTemplate) };
    });
  };
  const generate = async () => {
    if (!window.confirm("AI 会从文章中提炼小红书标题、短文案和话题，并创建新的草稿版本。旧版本会保留。继续吗？")) return;
    setGenerating(true); setMessage("");
    try {
      const next = ensurePages(await generateXiaohongshuDraft(articleID), "", template);
      setDraft(next);
      setView((current) => current ? { ...current, latest: next, history: [next, ...current.history] } : current);
      setMessage("已生成新草稿，旧版本仍保留在历史中");
    } catch (error) { setMessage(error instanceof Error ? error.message : "生成失败"); }
    finally { setGenerating(false); }
  };
  const save = async () => {
    if (!draft) return;
    setSaving(true); setMessage("");
    try {
      const bodyHTML = flattenXiaohongshuPages(draft.pages) || draft.body_html;
      const next = await saveXiaohongshuDraft(articleID, { ...draft, body_html: bodyHTML });
      setDraft(ensurePages(next, "", template));
      setView((current) => current ? { ...current, latest: next } : current);
      setMessage("草稿已保存");
    } catch (error) { setMessage(error instanceof Error ? error.message : "保存失败"); }
    finally { setSaving(false); }
  };
  const exportImages = async () => {
    if (!draft || draft.pages.length === 0) return;
    const selectedTemplate = getXiaohongshuTemplate(template);
    const pageHTML = draft.pages.map((page) => page.blocks.map((block) => block.html).join(""));
    const hash = await digest(pageHTML.join(""));
    await saveXiaohongshuRender(articleID, { draft_id: draft.id, template_id: selectedTemplate.id, template_version: "1", viewport_width: selectedTemplate.viewportWidth, page_height: selectedTemplate.pageHeight, html_hash: hash, page_count: pageHTML.length });
    for (let index = 0; index < pageHTML.length; index += 1) {
      const dataURL = await snapshotPage(pageHTML[index], index === 0 ? draft.title : "", index, pageHTML.length, selectedTemplate);
      const link = document.createElement("a"); link.href = dataURL; link.download = `xiaohongshu-${draft.id}-${String(index + 1).padStart(2, "0")}.png`; link.click();
    }
    setMessage(`已导出 ${pageHTML.length} 张图片；标题和文案请在 InkHub 中复制到小红书`);
  };
  const publish = async () => {
    if (!draft || !window.confirm("确认图片已手动上传到小红书，并将此版本标记为已发布？")) return;
    try { await markXiaohongshuPublished(articleID, draft.id); await load(); setMessage("已记录小红书发布"); }
    catch (error) { setMessage(error instanceof Error ? error.message : "发布确认失败"); }
  };

  return <div className="xiaohongshu-page"><PublicationPageFrame article={article} active="xiaohongshu" onNavigate={onNavigate} toolbarContent={<div className="xiaohongshu-actions"><button className="secondary" onClick={() => setShowHistory((value) => !value)}><History size={15} />历史</button><button className="secondary" onClick={() => void generate()} disabled={generating}><RefreshCw size={15} />{generating ? "AI 提取中" : "重新 AI 提取"}</button></div>}>
    {article.review_state !== "已通过" ? <section className="channel-locked" role="status"><h2>审核通过后才能准备小红书内容</h2><p>请先返回审核中心完善元数据并完成审核。</p><button className="secondary" type="button" onClick={() => onNavigate(`/articles/${articleID}`)}>返回审核中心</button></section> : <>
      <section className="xiaohongshu-settings" aria-label="小红书发布设置">
        <div className="tool-heading"><h2>小红书文案草稿</h2><label className="template-select"><LayoutTemplate size={15} />模板<select value={template} onChange={(event) => changeTemplate(event.target.value)}>{XIAOHONGSHU_TEMPLATES.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}</select></label></div>
        {!draft ? <div className="empty-state compact wide"><Sparkles size={24} /><p>还没有小红书草稿</p><button className="primary" onClick={() => void generate()}><Sparkles size={15} />AI 提取文案</button></div> : <>
          <label className="wide">标题<input value={draft.title} onChange={(event) => update({ title: event.target.value })} /></label>
          {draft.ai_model === "inkhub-adapter-v1" && <p className="inline-status wide">这是旧版完整文章草稿，点击“重新 AI 提取”生成短文案。</p>}
          <label>话题<textarea value={draft.topics} onChange={(event) => update({ topics: event.target.value })} placeholder="#AI编程 #效率工具" /></label>
          <label>来源说明<textarea value={draft.source_note} onChange={(event) => update({ source_note: event.target.value })} /></label>
          <label className="wide">评论区文案<textarea value={draft.comment_copy} onChange={(event) => update({ comment_copy: event.target.value })} /></label>
          <div className="xiaohongshu-settings-actions"><button className="primary" onClick={() => void save()} disabled={saving}><Save size={15} />{saving ? "保存中" : "保存草稿"}</button><button className="secondary" onClick={() => void publish()} disabled={draft.stale || draft.state === "published"}><Send size={15} />标记已发布</button></div>
          {message && <p className="inline-status wide" role="status">{message}</p>}
        </>}
      </section>
      {draft && <main className="xiaohongshu-layout"><section className="xiaohongshu-editor-shell"><XiaohongshuCardEditor pages={draft.pages} template={template} title={draft.title} onPagesChange={(pages) => update({ pages, body_html: flattenXiaohongshuPages(pages) })} onSelectionChange={() => undefined} /><div className="xiaohongshu-export"><span>预计 {draft.pages.length} 页图片</span><button className="primary" onClick={() => void exportImages()}><Download size={15} />导出图片集</button></div></section></main>}
    </>}
    {showHistory && <aside className="xiaohongshu-history"><div className="tool-heading"><h2>版本历史</h2><button className="back" onClick={() => setShowHistory(false)}>关闭</button></div>{view.history.map((item) => <button key={item.id} className={`xiaohongshu-history-item${draft?.id === item.id ? " active" : ""}`} onClick={() => { setDraft(ensurePages(item, article.preview_html, template)); setShowHistory(false); }}><strong>{item.title || "未命名草稿"}</strong><span>{item.state}{item.stale ? " · 内容已更新" : ""}</span><time>{new Intl.DateTimeFormat("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(item.created_at))}</time></button>)}</aside>}
  </PublicationPageFrame></div>;
}

/** digest 计算导出内容指纹，确保渲染记录对应当前页面内容。 */
async function digest(value: string) { const data = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value)); return Array.from(new Uint8Array(data)).map((byte) => byte.toString(16).padStart(2, "0")).join(""); }

/** snapshotPage 使用与卡片编辑器相同的页面 HTML 生成单张 PNG。 */
async function snapshotPage(html: string, title: string, index: number, pages: number, template: ReturnType<typeof getXiaohongshuTemplate>) {
  const background = template.id === "mobile-paper" ? "#fffaf1" : "#ffffff";
  const titleHTML = title ? `<h1 style="font-size:24px;line-height:1.3;margin:0 0 18px">${escapeText(title)}</h1>` : "";
  const source = `<div xmlns="http://www.w3.org/1999/xhtml" style="box-sizing:border-box;width:${template.viewportWidth}px;height:${template.pageHeight}px;overflow:hidden;background:${background}"><div style="box-sizing:border-box;width:${template.viewportWidth}px;height:${template.pageHeight}px;padding:${template.paddingY}px ${template.paddingX}px;background:${background};color:#17201b;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif"><small style="display:block;margin-bottom:14px;color:#849087;font-size:10px;text-align:right">${index + 1} / ${pages}</small>${titleHTML}<div style="font-size:15px;line-height:1.75;overflow-wrap:anywhere;white-space:normal">${stripXiaohongshuTitle(html)}</div></div></div>`;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${template.viewportWidth}" height="${template.pageHeight}"><foreignObject width="100%" height="100%">${source}</foreignObject></svg>`;
  const image = new Image(); image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`; await new Promise<void>((resolve, reject) => { image.onload = () => resolve(); image.onerror = () => reject(new Error("图片渲染失败")); });
  const canvas = document.createElement("canvas"); canvas.width = template.viewportWidth * 2; canvas.height = template.pageHeight * 2; const context = canvas.getContext("2d"); if (!context) throw new Error("无法创建图片画布"); context.scale(2, 2); context.drawImage(image, 0, 0, template.viewportWidth, template.pageHeight); return canvas.toDataURL("image/png");
}

function escapeText(value: string) { return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char] ?? char)); }

/** ensurePages 为旧版只保存正文 HTML 的草稿补齐页面块。 */
function ensurePages(next: XiaohongshuDraft, sourceHTML: string, templateID: string): XiaohongshuDraft {
  if (Array.isArray(next.pages) && next.pages.length > 0) return next;
  const bodyHTML = stripXiaohongshuTitle(next.body_html || sourceHTML || "<p>请输入正文</p>");
  return { ...next, body_html: bodyHTML, pages: buildXiaohongshuPages(bodyHTML, templateID) };
}
