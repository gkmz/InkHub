import { ArrowLeft, ArrowRight, Copy } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { XiaohongshuScriptPage } from "../../api/types";

interface XiaohongshuStoryboardEditorProps {
  pages: XiaohongshuScriptPage[];
  onPagesChange: (pages: XiaohongshuScriptPage[]) => void;
  onCopy: (value: string, successMessage: string) => void;
}

/** XiaohongshuStoryboardEditor 提供与长文卡片一致的逐页分镜浏览和编辑体验。 */
export function XiaohongshuStoryboardEditor({ pages, onPagesChange, onCopy }: XiaohongshuStoryboardEditorProps) {
  const pageRefs = useRef(new Map<string, HTMLElement>());
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    if (selectedIndex >= pages.length) setSelectedIndex(Math.max(0, pages.length - 1));
  }, [pages.length, selectedIndex]);

  const scrollToPage = (index: number) => {
    const nextIndex = Math.max(0, Math.min(pages.length - 1, index));
    const page = pages[nextIndex];
    if (!page) return;
    pageRefs.current.get(page.id)?.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "center" });
    setSelectedIndex(nextIndex);
  };

  const updatePage = (pageID: string, patch: Partial<XiaohongshuScriptPage>) => {
    onPagesChange(pages.map((page) => page.id === pageID ? { ...page, ...patch } : page));
  };

  return <section className="xiaohongshu-card-editor xiaohongshu-storyboard-editor" aria-label="图文分镜编辑器">
    <div className="xiaohongshu-card-toolbar"><span>{pages.length} 页分镜</span></div>
    {pages.length === 0 ? <p className="empty-state compact">暂无可编辑分镜</p> : null}
    <div className="xiaohongshu-card-stage">
      <button className="xiaohongshu-card-nav previous" type="button" aria-label="上一页" onClick={() => scrollToPage(selectedIndex - 1)} disabled={pages.length < 2 || selectedIndex === 0}><ArrowLeft size={18} /></button>
      <div className="xiaohongshu-card-scroller" role="list" aria-label="分镜页面列表">
        {pages.map((page, index) => <article
          key={page.id}
          ref={(element) => {
            if (element) pageRefs.current.set(page.id, element);
            else pageRefs.current.delete(page.id);
          }}
          className="xiaohongshu-card-page xiaohongshu-storyboard-page"
          role="listitem"
          aria-label={`第 ${index + 1} 页分镜，共 ${pages.length} 页`}
          onClick={() => setSelectedIndex(index)}
        >
          <div className="xiaohongshu-card-page-label">{index + 1} / {pages.length}</div>
          <label className="xiaohongshu-storyboard-title">分镜名称<input value={page.title} onChange={(event) => updatePage(page.id, { title: event.target.value })} /></label>
          <label className="xiaohongshu-storyboard-prompt">生图提示词<textarea value={page.prompt} onChange={(event) => updatePage(page.id, { prompt: event.target.value })} /></label>
          <button className="secondary xiaohongshu-storyboard-copy" type="button" onClick={() => onCopy(page.prompt, `已复制第 ${index + 1} 页提示词`)}><Copy size={15} />复制本页</button>
        </article>)}
      </div>
      <button className="xiaohongshu-card-nav next" type="button" aria-label="下一页" onClick={() => scrollToPage(selectedIndex + 1)} disabled={pages.length < 2 || selectedIndex === pages.length - 1}><ArrowRight size={18} /></button>
    </div>
  </section>;
}
