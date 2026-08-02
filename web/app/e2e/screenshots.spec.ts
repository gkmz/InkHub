import { expect, test } from "@playwright/test";

test.beforeEach(async ({ request }) => {
  await request.post("/api/v1/demo/reset");
});

const pages = [
  ["dashboard", "/", "工作台"],
  ["article", "/articles/a4", "公众号排版模板设计记录"],
  ["taxonomy", "/taxonomy", "博客类目已同步"],
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

test("截图-library-selection", async ({ page }, testInfo) => {
  await page.goto("/library");
  await page.getByRole("checkbox", { name: /选择文章 构建可靠/ }).click();
  await expect(page.getByText("已选择 1 篇")).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("library-selection.png"), fullPage: true });
});

test("截图-publish-dialog", async ({ page }, testInfo) => {
  await page.goto("/library");
  await page.getByRole("checkbox", { name: /选择文章 构建可靠/ }).click();
  await page.getByRole("button", { name: "标记已发表" }).click();
  await expect(page.getByRole("dialog", { name: "标记为已发表" })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("publish-dialog.png"), fullPage: true });
});
