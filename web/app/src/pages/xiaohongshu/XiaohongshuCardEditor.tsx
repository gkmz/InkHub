import { ArrowLeft, ArrowRight } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { sanitizePreviewHTML } from "../../api/safeHTML";
import type { XiaohongshuPage } from "../../api/types";

/** XiaohongshuCardEditorProps 描述卡片编辑器的受控输入输出。 */
export interface XiaohongshuCardEditorProps {
  pages: XiaohongshuPage[];
  template: string;
  title?: string;
  onPagesChange: (pages: XiaohongshuPage[]) => void;
  onSelectionChange: (pageID: string) => void;
}

/** XiaohongshuCardEditor 将小红书正文呈现为可左右浏览的独立卡片。 */
export function XiaohongshuCardEditor({ pages, template, title, onPagesChange, onSelectionChange }: XiaohongshuCardEditorProps) {
  const pageRefs = useRef(new Map<string, HTMLElement>());
  const [selectedIndex, setSelectedIndex] = useState(0);
  const scrollToPage = (index: number) => {
    const target = pages[Math.max(0, Math.min(pages.length - 1, index))];
    if (!target) return;
    const nextIndex = pages.findIndex((item) => item.id === target.id);
    pageRefs.current.get(target.id)?.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "center" });
    setSelectedIndex(nextIndex);
    onSelectionChange(target.id);
  };

  return <section className={`xiaohongshu-card-editor template-${template}`} aria-label="小红书卡片编辑器">
    <div className="xiaohongshu-card-toolbar">
      <span>{pages.length} 页卡片</span>
    </div>
    {pages.length === 0 ? <p className="empty-state compact">暂无可编辑卡片</p> : null}
    <div className="xiaohongshu-card-stage">
      <button className="xiaohongshu-card-nav previous" type="button" aria-label="上一页" onClick={() => scrollToPage(selectedIndex - 1)} disabled={pages.length < 2 || selectedIndex === 0}><ArrowLeft size={18} /></button>
      <div className="xiaohongshu-card-scroller" role="list" aria-label="小红书页面列表">
        {pages.map((page, index) => <XiaohongshuEditablePage
          key={page.id}
          page={page}
          index={index}
          total={pages.length}
          title={index === 0 ? title : undefined}
          setPageRef={(element) => element ? pageRefs.current.set(page.id, element) : pageRefs.current.delete(page.id)}
          template={template}
          onFocus={() => { setSelectedIndex(index); onSelectionChange(page.id); }}
          onChange={(html) => onPagesChange(pages.map((item) => item.id === page.id ? updateFirstBlock(item, html) : item))}
        />)}
      </div>
      <button className="xiaohongshu-card-nav next" type="button" aria-label="下一页" onClick={() => scrollToPage(selectedIndex + 1)} disabled={pages.length < 2 || selectedIndex === pages.length - 1}><ArrowRight size={18} /></button>
    </div>
  </section>;
}

function XiaohongshuEditablePage({ page, index, total, template, title, setPageRef, onFocus, onChange }: {
  page: XiaohongshuPage;
  index: number;
  total: number;
  template: string;
  title?: string;
  setPageRef: (element: HTMLElement | null) => void;
  onFocus: () => void;
  onChange: (html: string) => void;
}) {
  const contentRef = useRef<HTMLDivElement>(null);
  const contentFrameRef = useRef<HTMLDivElement>(null);
  const initializedRef = useRef(false);
  const [scale, setScale] = useState(1);
  const html = page.blocks.map((block) => block.html).join("");
  const safeHTML = sanitizePreviewHTML(html);

  // 小红书卡片不提供正文滚动，内容超出时按可用高度整体缩放，保证每张图都能完整查看。
  const measureScale = useCallback(() => {
    const content = contentRef.current;
    const frame = contentFrameRef.current;
    if (!content || !frame) return;
    const naturalHeight = content.scrollHeight;
    const availableHeight = frame.clientHeight;
    setScale(availableHeight > 0 && naturalHeight > availableHeight ? Math.min(1, availableHeight / naturalHeight) : 1);
  }, []);

  useLayoutEffect(() => {
    const content = contentRef.current;
    if (!content) return;
    if (!initializedRef.current || document.activeElement !== content) {
      if (content.innerHTML !== safeHTML) content.innerHTML = safeHTML;
      initializedRef.current = true;
    }
    measureScale();
  }, [measureScale, safeHTML]);

  useEffect(() => {
    const frame = contentFrameRef.current;
    if (!frame) return;
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measureScale);
    observer.observe(frame);
    return () => observer.disconnect();
  }, [measureScale]);

  return <article ref={setPageRef} className={`xiaohongshu-card-page template-${template}`} role="listitem" aria-label={`第 ${index + 1} 页，共 ${total} 页`} onClick={onFocus}>
    <div className="xiaohongshu-card-page-label">{index + 1} / {total}</div>
    {title ? <h1 className="xiaohongshu-card-title">{title}</h1> : null}
    <div ref={contentFrameRef} className="xiaohongshu-card-content-frame">
      <div
        ref={contentRef}
        className="xiaohongshu-card-content"
        contentEditable
        suppressContentEditableWarning
        role="textbox"
        aria-label={`第 ${index + 1} 页正文`}
        aria-multiline="true"
        style={{ transform: `scale(${scale})` }}
        onFocus={onFocus}
        onInput={(event) => { onChange(sanitizePreviewHTML(event.currentTarget.innerHTML)); requestAnimationFrame(measureScale); }}
      />
    </div>
  </article>;
}

function updateFirstBlock(page: XiaohongshuPage, html: string): XiaohongshuPage {
  if (page.blocks.length === 0) {
    return { ...page, blocks: [{ id: `${page.id}-block-1`, kind: "paragraph", html, splittable: true }] };
  }
  // 卡片编辑是整页所见即所得，保存时将用户编辑后的页面压成一个可继续编辑的正文块，避免旧块重复渲染。
  return { ...page, blocks: [{ ...page.blocks[0], kind: "paragraph", html, splittable: true }] };
}
