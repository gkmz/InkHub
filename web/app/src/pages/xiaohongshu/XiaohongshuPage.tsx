import { Download, History, LayoutTemplate, RefreshCw, Save, Send, Sparkles } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getArticle, getXiaohongshu, markXiaohongshuPublished, outlineXiaohongshuDraft, rewriteXiaohongshuDraft, saveXiaohongshuDraft, saveXiaohongshuRender } from "../../api/client";
import type { ArticleDetail, XiaohongshuDraft, XiaohongshuView } from "../../api/types";
import { PublicationPageFrame } from "../../components/PublicationPageFrame";
import { inlineXiaohongshuImages, renderXiaohongshuMermaidImages, stripXiaohongshuTitle } from "./xiaohongshuAdapter";
import { buildXiaohongshuPages, flattenXiaohongshuPages, getXiaohongshuTemplate, XIAOHONGSHU_DEFAULT_TEMPLATE } from "./xiaohongshuLayout";
import { XiaohongshuCardEditor } from "./XiaohongshuCardEditor";
import { measureXiaohongshuContentScale, xiaohongshuScaledContentStyle } from "./xiaohongshuSizing";

type RewriteStage = "idle" | "outline" | "rewrite";

/** XiaohongshuPage 提供页面块编辑、模板切换、图片导出和人工发布确认。 */
export function XiaohongshuPage({ articleID, onNavigate }: { articleID: string; onNavigate: (path: string) => void }) {
  const [article, setArticle] = useState<ArticleDetail | null>(null);
  const [view, setView] = useState<XiaohongshuView | null>(null);
  const [draft, setDraft] = useState<XiaohongshuDraft | null>(null);
  const template = XIAOHONGSHU_DEFAULT_TEMPLATE.id;
  const [saving, setSaving] = useState(false);
  const [rewriteStage, setRewriteStage] = useState<RewriteStage>("idle");
  const [exporting, setExporting] = useState(false);
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
  const generate = async () => {
    if (!window.confirm("AI 会先提取原文知识点，再改写为适合小红书阅读的笔记，并创建新版本。当前版本会保留在历史中。继续吗？")) return;
    setRewriteStage("outline"); setMessage("");
    try {
      const outline = await outlineXiaohongshuDraft(articleID);
      setRewriteStage("rewrite");
      const next = ensurePages(await rewriteXiaohongshuDraft(articleID, outline), "", template);
      setDraft(next);
      setView((current) => current ? { ...current, latest: next, history: [next, ...current.history] } : current);
      setMessage("已生成小红书笔记，原版本仍保留在历史中");
    } catch (error) { setMessage(error instanceof Error ? error.message : "AI 改写失败"); }
    finally { setRewriteStage("idle"); }
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
    setExporting(true); setMessage("");
    try {
      const selectedTemplate = getXiaohongshuTemplate(template);
      // 导出前统一生成 Mermaid 图片，保证预览和最终图片集使用同一份内容。
      const pageHTML = await Promise.all(draft.pages.map(async (page) => renderXiaohongshuMermaidImages(page.blocks.map((block) => block.html).join(""))));
      const hash = await digest(pageHTML.join(""));
      await saveXiaohongshuRender(articleID, { draft_id: draft.id, template_id: selectedTemplate.id, template_version: "1", viewport_width: selectedTemplate.viewportWidth, page_height: selectedTemplate.pageHeight, html_hash: hash, page_count: pageHTML.length });
      const { default: JSZip } = await import("jszip");
      const archive = new JSZip();
      for (let index = 0; index < pageHTML.length; index += 1) {
        const dataURL = await snapshotPage(pageHTML[index], index === 0 ? draft.title : "", index, pageHTML.length, selectedTemplate);
        // 多页内容统一打包，避免浏览器拦截第十张之后的连续自动下载。
        archive.file(`xiaohongshu-${String(index + 1).padStart(2, "0")}.png`, dataURL.split(",")[1] ?? "", { base64: true });
      }
      const archiveURL = URL.createObjectURL(await archive.generateAsync({ type: "blob", compression: "DEFLATE", compressionOptions: { level: 6 } }));
      const link = document.createElement("a"); link.href = archiveURL; link.download = `xiaohongshu-${draft.id}.zip`; link.click();
      window.setTimeout(() => URL.revokeObjectURL(archiveURL), 1000);
      setMessage(`已导出 ${pageHTML.length} 张图片（ZIP）；标题和文案请在 InkHub 中复制到小红书`);
    } catch (error) {
      setMessage(error instanceof Error ? `导出失败：${error.message}` : "导出失败");
    } finally {
      setExporting(false);
    }
  };
  const publish = async () => {
    if (!draft || !window.confirm("确认图片已手动上传到小红书，并将此版本标记为已发布？")) return;
    try { await markXiaohongshuPublished(articleID, draft.id); await load(); setMessage("已记录小红书发布"); }
    catch (error) { setMessage(error instanceof Error ? error.message : "发布确认失败"); }
  };

  const rewriting = rewriteStage !== "idle";
  const rewriteLabel = rewriteStage === "outline" ? "正在提取知识点" : rewriteStage === "rewrite" ? "正在改写笔记" : "AI 一键改写";

  return <div className="xiaohongshu-page"><PublicationPageFrame article={article} active="xiaohongshu" onNavigate={onNavigate}>
    {article.review_state !== "已通过" ? <section className="channel-locked" role="status"><h2>审核通过后才能准备小红书内容</h2><p>请先返回审核中心完善元数据并完成审核。</p><button className="secondary" type="button" onClick={() => onNavigate(`/articles/${articleID}`)}>返回审核中心</button></section> : <>
      <section className="xiaohongshu-content-toolbar" aria-label="小红书内容工具">
        <div className="xiaohongshu-content-summary">
          <strong>内容版本</strong>
          <span>{draft ? `${draft.state === "published" ? "已发布" : "草稿"}${draft.stale ? " · 原文已更新" : ""}` : "尚未生成草稿"}</span>
        </div>
        <div className="xiaohongshu-actions">
          <button className="secondary" type="button" aria-controls="xiaohongshu-history" aria-expanded={showHistory} onClick={() => setShowHistory((value) => !value)}><History size={15} />历史版本</button>
          <button className="secondary" type="button" onClick={() => void generate()} disabled={rewriting}><RefreshCw size={15} />{rewriteLabel}</button>
        </div>
        {showHistory && <aside id="xiaohongshu-history" className="xiaohongshu-history"><div className="tool-heading"><h2>版本历史</h2><button className="back" type="button" onClick={() => setShowHistory(false)}>关闭</button></div>{view.history.map((item) => <button key={item.id} type="button" className={`xiaohongshu-history-item${draft?.id === item.id ? " active" : ""}`} onClick={() => { setDraft(ensurePages(item, article.preview_html, template)); setShowHistory(false); }}><strong>{item.title || "未命名草稿"}</strong><span>{item.state}{item.stale ? " · 内容已更新" : ""}</span><time>{new Intl.DateTimeFormat("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(item.created_at))}</time></button>)}</aside>}
      </section>
      <section className="xiaohongshu-settings" aria-label="小红书发布设置">
        <div className="tool-heading"><h2>小红书文案草稿</h2><span className="template-current"><LayoutTemplate size={15} />{XIAOHONGSHU_DEFAULT_TEMPLATE.label}</span></div>
        {!draft ? <div className="empty-state compact wide"><Sparkles size={24} /><p>还没有小红书草稿</p><button className="primary" onClick={() => void generate()} disabled={rewriting}><Sparkles size={15} />{rewriteLabel}</button></div> : <>
          <label className="wide">标题<input value={draft.title} onChange={(event) => update({ title: event.target.value })} /></label>
          {draft.ai_model === "inkhub-adapter-v1" && <p className="inline-status wide">这是原文分页版本，可使用“AI 一键改写”生成适合小红书传播的笔记。</p>}
          <label>话题<textarea value={draft.topics} onChange={(event) => update({ topics: event.target.value })} placeholder="#AI编程 #效率工具" /></label>
          <label>来源说明<textarea value={draft.source_note} onChange={(event) => update({ source_note: event.target.value })} /></label>
          <label className="wide">评论区文案<textarea value={draft.comment_copy} onChange={(event) => update({ comment_copy: event.target.value })} /></label>
          <div className="xiaohongshu-settings-actions"><button className="primary" onClick={() => void save()} disabled={saving}><Save size={15} />{saving ? "保存中" : "保存草稿"}</button><button className="secondary" onClick={() => void publish()} disabled={draft.stale || draft.state === "published"}><Send size={15} />标记已发布</button></div>
        </>}
        {message && <p className="inline-status wide" role="status">{message}</p>}
      </section>
      {draft && <main className="xiaohongshu-layout"><section className="xiaohongshu-editor-shell"><XiaohongshuCardEditor pages={draft.pages} template={template} title={draft.title} onPagesChange={(pages) => update({ pages, body_html: flattenXiaohongshuPages(pages) })} onSelectionChange={() => undefined} /><div className="xiaohongshu-export"><span>预计 {draft.pages.length} 页图片</span><button className="primary" onClick={() => void exportImages()} disabled={exporting}><Download size={15} />{exporting ? "导出中" : "导出图片集"}</button></div></section></main>}
    </>}
  </PublicationPageFrame></div>;
}

/** digest 计算导出内容指纹，确保渲染记录对应当前页面内容。 */
async function digest(value: string) { const data = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value)); return Array.from(new Uint8Array(data)).map((byte) => byte.toString(16).padStart(2, "0")).join(""); }

/** snapshotPage 使用与卡片编辑器相同的页面 HTML 生成单张 PNG。 */
async function snapshotPage(html: string, title: string, index: number, pages: number, template: ReturnType<typeof getXiaohongshuTemplate>) {
  const background = template.backgroundColor;
  const titleHTML = title ? `<h1 class="xiaohongshu-snapshot-title">${escapeText(title)}</h1>` : "";
  const inlinedHTML = await inlineXiaohongshuImages(html);
  const svg = await buildXiaohongshuSnapshotSVG(inlinedHTML, titleHTML, index, pages, template, background);
  // Base64 data URL 同时支持大体积内容，并保持 Canvas 可导出。
  const sourceURL = await blobToDataURL(new Blob([svg], { type: "image/svg+xml;charset=utf-8" }));
  const image = new Image(); image.src = sourceURL;
  await new Promise<void>((resolve, reject) => { image.onload = () => resolve(); image.onerror = () => reject(new Error("图片渲染失败")); });
  const canvas = document.createElement("canvas"); canvas.width = template.viewportWidth * 2; canvas.height = template.pageHeight * 2;
  const context = canvas.getContext("2d"); if (!context) throw new Error("无法创建图片画布");
  context.scale(2, 2); context.drawImage(image, 0, 0, template.viewportWidth, template.pageHeight);
  return canvas.toDataURL("image/png");
}

/** blobToDataURL 将导出源编码为不会污染 Canvas 的内联地址。 */
function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(new Error("导出内容编码失败"));
    reader.readAsDataURL(blob);
  });
}

/** buildXiaohongshuSnapshotSVG 使用预览同款适配规则生成完整且宽度一致的导出内容。 */
async function buildXiaohongshuSnapshotSVG(html: string, titleHTML: string, index: number, pages: number, template: ReturnType<typeof getXiaohongshuTemplate>, background: string) {
  const svgNamespace = "http://www.w3.org/2000/svg";
  const xhtmlNamespace = "http://www.w3.org/1999/xhtml";
  const svg = document.createElementNS(svgNamespace, "svg");
  svg.setAttribute("width", String(template.viewportWidth)); svg.setAttribute("height", String(template.pageHeight));
  const foreignObject = document.createElementNS(svgNamespace, "foreignObject");
  foreignObject.setAttribute("width", "100%"); foreignObject.setAttribute("height", "100%");
  const root = document.createElementNS(xhtmlNamespace, "div");
  root.setAttribute("style", `box-sizing:border-box;width:${template.viewportWidth}px;height:${template.pageHeight}px;overflow:hidden;background:${background}`);
  root.innerHTML = `<style>${xiaohongshuSnapshotStyles(template)}</style><div class="xiaohongshu-snapshot" style="width:${template.viewportWidth}px;height:${template.pageHeight}px;padding:${template.paddingY}px ${template.paddingX}px;background:${background}"><small>${index + 1} / ${pages}</small>${titleHTML}<div class="xiaohongshu-snapshot-content">${html}</div></div>`;
  const measurementHost = document.createElement("div");
  measurementHost.style.cssText = "position:fixed;left:-100000px;top:0;visibility:hidden;pointer-events:none;";
  measurementHost.appendChild(root); document.body.appendChild(measurementHost);
  try {
    await Promise.all(Array.from(root.querySelectorAll("img")).map(async (image) => {
      if (!image.complete) await image.decode().catch(() => undefined);
    }));
    const snapshot = root.querySelector<HTMLElement>(".xiaohongshu-snapshot");
    const content = root.querySelector<HTMLElement>(".xiaohongshu-snapshot-content");
    if (snapshot && content) {
      const availableHeight = snapshot.getBoundingClientRect().bottom - template.paddingY - content.getBoundingClientRect().top;
      const fittedStyle = xiaohongshuScaledContentStyle(measureXiaohongshuContentScale(content, availableHeight));
      content.style.width = fittedStyle.width; content.style.transform = fittedStyle.transform;
    }
  } finally {
    root.remove(); measurementHost.remove();
  }
  foreignObject.appendChild(root); svg.appendChild(foreignObject);
  return new XMLSerializer().serializeToString(svg);
}

/** xiaohongshuSnapshotStyles 使用模板 token 生成独立导出样式。 */
function xiaohongshuSnapshotStyles(template: ReturnType<typeof getXiaohongshuTemplate>) {
  return `*{box-sizing:border-box}.xiaohongshu-snapshot{overflow:hidden;color:${template.textColor};font-family:${template.bodyFontFamily};box-shadow:inset 0 4px ${template.accentColor}}.xiaohongshu-snapshot>small{display:block;margin-bottom:14px;padding-bottom:8px;border-bottom:1px solid ${template.borderColor};color:${template.secondaryAccentColor};font:600 10px/1.2 ${template.headingFontFamily};text-align:right}.xiaohongshu-snapshot-title{margin:0 0 18px;color:${template.headingColor};font:750 25px/1.34 ${template.headingFontFamily}}.xiaohongshu-snapshot-content{font-size:15.5px;line-height:1.85;overflow-wrap:anywhere;white-space:normal;transform-origin:top left}.xiaohongshu-snapshot-content p{margin:0 0 13px}.xiaohongshu-snapshot-content h2,.xiaohongshu-snapshot-content h3,.xiaohongshu-snapshot-content h4{margin:18px 0 9px;color:${template.headingColor};font-family:${template.headingFontFamily};line-height:1.42}.xiaohongshu-snapshot-content a{color:${template.accentColor};text-decoration-color:${template.secondaryAccentColor};text-underline-offset:2px}.xiaohongshu-snapshot-content ul,.xiaohongshu-snapshot-content ol{padding-left:22px}.xiaohongshu-snapshot-content li::marker{color:${template.secondaryAccentColor}}.xiaohongshu-snapshot-content blockquote{margin:15px 0;padding:9px 12px;border-left:3px solid ${template.secondaryAccentColor};background:#e8eee9;color:#52635c}.xiaohongshu-snapshot-content img,.xiaohongshu-snapshot-content svg,.xiaohongshu-snapshot-content video,.xiaohongshu-snapshot-content canvas{display:block;max-width:100%;height:auto;margin:12px auto;border-radius:4px}.xiaohongshu-snapshot-content img{box-shadow:0 5px 14px rgba(34,58,49,.14)}.xiaohongshu-snapshot-content pre{max-width:100%;margin:13px 0;overflow:hidden;white-space:pre-wrap;overflow-wrap:anywhere;padding:11px;border:1px solid ${template.borderColor};border-radius:5px;background:#e8eeea;color:#24362f;font:12px/1.65 ui-monospace,SFMono-Regular,Menlo,monospace}.xiaohongshu-snapshot-content :not(pre)>code{padding:1px 4px;border-radius:3px;background:#e5ece7;color:#704638;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}.xiaohongshu-table-card{display:grid;gap:9px;margin:13px 0;padding:12px 12px 12px 14px;border:1px solid ${template.borderColor};border-left:3px solid ${template.secondaryAccentColor};border-radius:6px;background:#fbfcfa}.xiaohongshu-table-field{display:grid;gap:2px;min-width:0}.xiaohongshu-table-field strong{color:${template.mutedColor};font:600 11px/1.4 ${template.headingFontFamily}}.xiaohongshu-table-field span{min-width:0;line-height:1.65;overflow-wrap:anywhere}.xiaohongshu-mermaid-image{width:100%;overflow:hidden;margin:14px 0;padding:8px;border:1px solid ${template.borderColor};border-radius:6px;background:#fbfcfa}.xiaohongshu-mermaid-image svg{width:100%;max-width:100%;height:auto;margin:0 auto}`;
}

function escapeText(value: string) { return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char] ?? char)); }

/** ensurePages 使用当前模板规则把正文统一转换为最新分页。 */
function ensurePages(next: XiaohongshuDraft, sourceHTML: string, templateID: string): XiaohongshuDraft {
  const bodyHTML = stripXiaohongshuTitle(next.body_html || sourceHTML || "<p>请输入正文</p>");
  // 页面布局规则会随模板和字体调整，加载草稿时从正文重新分页，避免旧版 pages_json 保留过大的底部空白。
  const pages = buildXiaohongshuPages(bodyHTML, templateID);
  return { ...next, body_html: bodyHTML, pages };
}
