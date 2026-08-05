import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test.beforeEach(async ({ request }) => {
  await request.post("/api/v1/demo/reset");
});

test("文章可手工审核并进入 Hugo 同步", async ({ page }) => {
  await page.goto("/articles/a4");
  await expect(page.getByRole("heading", { name: "公众号排版模板设计记录" })).toBeVisible();
  const reviewTab = page.getByRole("tab", { name: "审核" });
  if (await reviewTab.isVisible()) await reviewTab.click();
  await page.getByRole("button", { name: "审核通过" }).click();
  await expect(page.getByRole("button", { name: "同步到 Hugo" })).toBeVisible();
  await page.getByRole("button", { name: "同步到 Hugo" }).click();
  await page.getByRole("combobox", { name: "发布目录" }).selectOption("posts");
  await page.getByRole("button", { name: "生成发布预览" }).click();
  await expect(page.getByText("content/posts/wechat-template-design")).toBeVisible();
  await page.getByRole("button", { name: "确认同步到 Hugo" }).click();
  await expect(page.getByText("文章已同步到 Hugo")).toBeVisible();
});

test("刷新文章页后恢复 Hugo 预览并展示统一发布历史", async ({ page }) => {
  await page.goto("/articles/a2");
  const openPublishTab = async () => {
    if ((page.viewportSize()?.width ?? 1000) <= 420) await page.getByRole("tab", { name: "发布" }).click();
  };
  await openPublishTab();
  await expect(page.getByText("content/posts/restored-writing-system")).toBeVisible();
  await expect(page.getByRole("button", { name: "确认同步到 Hugo" })).toBeVisible();

  await page.reload();
  await openPublishTab();
  await expect(page.getByText("content/posts/restored-writing-system")).toBeVisible();
  await page.getByText("发布历史").click();
  await expect(page.getByText("已同步到 Hugo")).toBeVisible();
  await expect(page.getByText("已确认保存微信草稿")).toBeVisible();
});

test("类目管理展示 Hugo 标签及文章数量", async ({ page }) => {
  await page.goto("/taxonomy");
  await page.getByRole("tab", { name: "标签" }).click();
  await expect(page.getByRole("strong").filter({ hasText: "local-first" })).toBeVisible();
  await expect(page.getByText("2 篇文章")).toBeVisible();
});

test("微信模板切换、复制和人工确认保持独立", async ({ page }) => {
  await page.addInitScript(() => Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async () => undefined } }));
  await page.goto("/articles/a2/wechat");
  await page.getByLabel(/模板/).selectOption("minimal");
  await expect(page.locator(".wechat-document")).toHaveClass(/template-minimal/);
  await expect(page.getByRole("button", { name: "草稿已保存" })).toHaveCount(0);
  await expect(page.getByText("images/cover.png")).toBeVisible();
  await page.getByRole("button", { name: "确认并准备" }).click();
  await expect(page.getByRole("button", { name: "复制格式化内容" })).toBeVisible();
  await page.getByRole("button", { name: "复制格式化内容" }).click();
  await expect(page.getByRole("button", { name: "草稿已保存" })).toBeVisible();
});

test("内容库批量标记渠道已发表并从工作台最近处理查看", async ({ page }) => {
  await page.goto("/library");
  await page.getByRole("checkbox", { name: /选择文章 公众号排版模板/ }).click();
  await page.getByRole("checkbox", { name: /选择文章 内容哈希/ }).click();
  await page.getByRole("button", { name: "标记已发表" }).click();
  await page.getByRole("checkbox", { name: "Hugo" }).click();
  await page.getByRole("checkbox", { name: "微信" }).click();
  await page.getByRole("button", { name: "确认标记" }).click();
  await expect(page.getByText("已处理 2 篇文章")).toBeVisible();
  await page.getByRole("link", { name: "工作台" }).first().click();
  const recentlyHandled = page.getByRole("heading", { name: "最近处理" }).locator("..").locator("..");
  await expect(recentlyHandled.getByText("公众号排版模板设计记录")).toBeVisible();
});

test("内容库忽略文章后默认隐藏并可筛选恢复", async ({ page }) => {
  await page.goto("/library");
  await page.getByRole("checkbox", { name: /选择文章 SQLite/ }).click();
  await page.getByRole("button", { name: "忽略" }).click();
  await page.getByRole("button", { name: "确认忽略" }).click();
  await expect(page.getByText("已处理 1 篇文章").last()).toBeVisible();
  await expect(page.getByText("SQLite 在桌面应用中的取舍")).toHaveCount(0);

  await page.getByLabel("处置状态").selectOption("ignored");
  await page.getByRole("checkbox", { name: /选择文章 SQLite/ }).click();
  await page.getByRole("button", { name: "恢复管理" }).click();
  await page.getByRole("button", { name: "确认恢复" }).click();
  await expect(page.getByText("已处理 1 篇文章").last()).toBeVisible();
  await expect(page.getByText("SQLite 在桌面应用中的取舍")).toHaveCount(0);
});

test("当前页面无严重可访问性问题和横向溢出", async ({ page }) => {
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "设置" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations.filter((item) => item.impact === "critical" || item.impact === "serious")).toEqual([]);
});
