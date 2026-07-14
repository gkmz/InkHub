import { expect, test } from "@playwright/test";

const pages = [
  ["article", "/articles/a4", "公众号排版模板设计记录"],
  ["taxonomy", "/taxonomy", "标签治理"],
  ["settings", "/settings", "设置"],
  ["wechat", "/articles/a4/wechat", "公众号排版模板设计记录"],
] as const;

for (const [name, path, heading] of pages) {
  test(`截图-${name}`, async ({ page }, testInfo) => {
    await page.goto(path);
    await expect(page.getByRole("heading", { name: heading }).first()).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath(`${name}.png`), fullPage: true });
  });
}
