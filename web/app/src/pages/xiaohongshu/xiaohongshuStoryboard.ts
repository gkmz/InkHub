import type { XiaohongshuScriptPage } from "../../api/types";

/** formatXiaohongshuStoryboard 将全部页面整理为可粘贴到外部生图流程的脚本。 */
export function formatXiaohongshuStoryboard(pages: XiaohongshuScriptPage[]) {
  return pages.map((page, index) => `第 ${index + 1} 页：${page.title}\n\n${page.prompt}`).join("\n\n---\n\n");
}
