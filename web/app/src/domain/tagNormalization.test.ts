import { expect, test } from "vitest";
import { normalizeTag, normalizeTags, tagNormalizationChanges } from "./tagNormalization";

test("标签统一转换为 lowercase kebab-case", () => {
  expect(normalizeTag("Coding Agent")).toBe("coding-agent");
  expect(normalizeTag("Superpowers")).toBe("superpowers");
  expect(normalizeTag("AI 编程")).toBe("ai-编程");
  expect(normalizeTag("开发效率")).toBe("开发效率");
});

test("规范化后自动合并重复标签并生成提示", () => {
  expect(normalizeTags(["Coding Agent", "coding_agent", "AI 编程"])).toEqual(["coding-agent", "ai-编程"]);
  expect(tagNormalizationChanges(["Coding Agent", "coding_agent"])).toEqual([
    { source: "Coding Agent", target: "coding-agent", duplicate: false },
    { source: "coding_agent", target: "coding-agent", duplicate: true },
  ]);
});
