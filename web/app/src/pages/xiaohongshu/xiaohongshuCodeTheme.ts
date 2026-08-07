/** XiaohongshuCodeTheme 描述模板内代码块容器和 Chroma token 色值。 */
export interface XiaohongshuCodeTheme {
  backgroundColor: string;
  borderColor: string;
  textColor: string;
  rules: ReadonlyArray<{ selectors: readonly string[]; declarations: string }>;
}

/** TOKYO_NIGHT_CODE_THEME 精确复用 old 分支的 Tokyo Night Night 配色。 */
export const TOKYO_NIGHT_CODE_THEME: XiaohongshuCodeTheme = {
  backgroundColor: "#1a1b26",
  borderColor: "#2a2d3e",
  textColor: "#c0caf5",
  rules: [
    { selectors: ["w"], declarations: "color:#3b4261" },
    { selectors: ["err", "ne", "nv", "vc", "vg", "vi", "vm", "gd", "gr", "gt"], declarations: "color:#f7768e" },
    { selectors: ["c", "ch", "cm", "c1", "cs", "sd"], declarations: "color:#565f89;font-style:italic" },
    { selectors: ["cp", "cpf", "kn", "nb", "bp", "nc", "nd", "nf", "fm", "nt", "nn"], declarations: "color:#7aa2f7" },
    { selectors: ["kd"], declarations: "color:#7aa2f7;font-weight:bold" },
    { selectors: ["k", "kp", "kr", "se"], declarations: "color:#bb9af7" },
    { selectors: ["kc", "no", "ss", "m", "mb", "mf", "mh", "mi", "il", "mo"], declarations: "color:#ff9e64" },
    { selectors: ["kt"], declarations: "color:#2ac3de" },
    { selectors: ["gp"], declarations: "color:#2ac3de;font-weight:bold" },
    { selectors: ["o"], declarations: "color:#89ddff" },
    { selectors: ["ow"], declarations: "color:#89ddff;font-weight:bold" },
    { selectors: ["na", "ni", "py"], declarations: "color:#73daca" },
    { selectors: ["nl"], declarations: "color:#7dcfff" },
    { selectors: ["l", "s", "sa", "sb", "sc", "dl", "s2", "sh", "sx", "s1", "gi"], declarations: "color:#9ece6a" },
    { selectors: ["ld", "si"], declarations: "color:#e0af68" },
    { selectors: ["sr"], declarations: "color:#b4f9f8" },
    { selectors: ["go"], declarations: "color:#565f89" },
    { selectors: ["gh", "gu"], declarations: "color:#7aa2f7;font-weight:bold" },
    { selectors: ["ge"], declarations: "font-style:italic" },
    { selectors: ["gs"], declarations: "font-weight:bold" },
  ],
};

/** xiaohongshuCodeThemeCSS 为指定模板容器生成带作用域的代码主题。 */
export function xiaohongshuCodeThemeCSS(theme: XiaohongshuCodeTheme, scope: string): string {
  if (!scope.trim()) throw new Error("小红书代码主题缺少作用域");
  const container = `${scope} pre.chroma{background:${theme.backgroundColor};border-color:${theme.borderColor};color:${theme.textColor}}`;
  const tokens = theme.rules.map((rule) => `${rule.selectors.map((selector) => `${scope} .chroma .${selector}`).join(",")}{${rule.declarations}}`).join("");
  return container + tokens;
}
