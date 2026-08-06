const MIN_CONTENT_SCALE = 0.4;
const SCALE_PRECISION_STEPS = 10;

/** measureXiaohongshuContentScale 计算保持视觉宽度不变时，正文完整放入可用高度所需的缩放比例。 */
export function measureXiaohongshuContentScale(content: HTMLElement, availableHeight: number): number {
  if (availableHeight <= 0) return 1;
  const previousWidth = content.style.width;
  const previousTransform = content.style.transform;
  content.style.width = "100%";
  content.style.transform = "none";
  if (content.scrollHeight <= availableHeight) {
    content.style.width = previousWidth;
    content.style.transform = previousTransform;
    return 1;
  }

  let lower = MIN_CONTENT_SCALE;
  let upper = 1;
  // 反向扩大布局宽度再整体缩放，保证每页正文最终都占满相同的视觉宽度。
  for (let step = 0; step < SCALE_PRECISION_STEPS; step += 1) {
    const scale = (lower + upper) / 2;
    content.style.width = `${100 / scale}%`;
    if (content.scrollHeight * scale <= availableHeight) lower = scale;
    else upper = scale;
  }
  content.style.width = previousWidth;
  content.style.transform = previousTransform;
  return lower;
}

/** xiaohongshuScaledContentStyle 返回保持卡片视觉宽度一致的正文样式。 */
export function xiaohongshuScaledContentStyle(scale: number): { width: string; transform: string } {
  const safeScale = Math.max(MIN_CONTENT_SCALE, Math.min(1, scale));
  return {
    width: safeScale < 1 ? `${100 / safeScale}%` : "100%",
    transform: safeScale < 1 ? `scale(${safeScale})` : "none",
  };
}
