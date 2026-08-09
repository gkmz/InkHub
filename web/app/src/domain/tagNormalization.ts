/** normalizeTag 将标签转换为跨渠道稳定的 lowercase kebab-case 名称。 */
export function normalizeTag(tag: string) {
  const lowered = tag.trim().toLocaleLowerCase();
  let result = "";
  let pendingSeparator = false;
  for (const character of lowered) {
    if (/\s/u.test(character) || character === "/" || character === "_" || character === "-") {
      pendingSeparator = result.length > 0;
      continue;
    }
    if (!/[\p{L}\p{N}.+#]/u.test(character)) {
      pendingSeparator = result.length > 0;
      continue;
    }
    if (pendingSeparator) result += "-";
    pendingSeparator = false;
    result += character;
  }
  return result.replace(/^-+|-+$/g, "");
}

/** normalizeTags 规范化并按最终名称去重，行为与服务端写入规则保持一致。 */
export function normalizeTags(tags: string[]) {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const tag of tags) {
    const normalized = normalizeTag(tag);
    if (!normalized || seen.has(normalized)) continue;
    seen.add(normalized);
    result.push(normalized);
  }
  return result;
}

/** tagNormalizationChanges 返回用户保存前需要了解的名称变化。 */
export function tagNormalizationChanges(tags: string[]) {
  const seen = new Set<string>();
  const changes: Array<{ source: string; target: string; duplicate: boolean }> = [];
  for (const source of tags) {
    const target = normalizeTag(source);
    const duplicate = target !== "" && seen.has(target);
    if (target) seen.add(target);
    if (source !== target || duplicate) changes.push({ source, target, duplicate });
  }
  return changes;
}
