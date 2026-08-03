import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { AISuggestions } from "../../components/AISuggestions";
import { MetadataForm } from "../../components/MetadataForm";
import { PublicationTrack } from "../../components/PublicationTrack";
import { WeChatActions } from "../../components/WeChatActions";
import { JobStatus } from "../../components/JobStatus";
import { ToastProvider } from "../../components/ToastProvider";
import { ArticlePage } from "./ArticlePage";

const metadata = {
  title: "本地优先的内容工作流",
  description: "旧摘要",
  category: "工程实践",
  series: "InkHub",
  tags: ["Go", "React"],
  keywords: ["本地优先"],
  slug: "local-first-content",
  cover: "",
};

const article = {
  id: "article-1", content_version: "hash-1", hugo_provider_id: "h1", wechat_provider_id: "w1", relative_path: "Areas/article.md", modified_at: "2026-07-15",
  metadata, preview_html: "<p>正文</p>", source_changed: false, review_state: "待审核", hugo_state: "尚未同步", wechat_state: "尚未准备",
  checks: [], ai_configured: false, suggestions: [], suggestions_stale: false, wechat_copied: false,
};

const taxonomy = {
  source: "Hugo", provider_id: "h1", provider_type: "hugo", state: "ready", revision: "revision-1", loaded_at: "2026-07-15", readonly: false,
  terms: [{ kind: "category", key: "engineering", name: "工程实践", usage_count: 3, metadata: {} }, { kind: "category", key: "product", name: "产品", usage_count: 2, metadata: {} }, { kind: "series", key: "inkhub", name: "InkHub", usage_count: 4, metadata: {} }, { kind: "series", key: "go-course", name: "Go 课程", usage_count: 2, metadata: {} }, { kind: "tag", key: "go", name: "Go", usage_count: 18, metadata: {} }, { kind: "tag", key: "sqlite", name: "SQLite", usage_count: 7, metadata: {} }], issues: [],
};

afterEach(() => vi.restoreAllMocks());

test("源文件变化后元数据表单禁止覆盖并提供重新加载", async () => {
  const save = vi.fn();
  render(<MetadataForm value={metadata} sourceChanged onSave={save} />);
  await userEvent.clear(screen.getByLabelText("标题"));
  await userEvent.type(screen.getByLabelText("标题"), "新标题");
  expect(screen.getByText("文章已在写作工具中更新")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存到文章" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "重新加载" })).toBeInTheDocument();
  expect(save).not.toHaveBeenCalled();
});

test("保存元数据前展示字段级变更摘要", async () => {
  render(<MetadataForm value={metadata} sourceChanged={false} onSave={vi.fn()} />);
  await userEvent.clear(screen.getByLabelText("Description"));
  await userEvent.type(screen.getByLabelText("Description"), "新摘要");
  expect(screen.getByText("Description：旧摘要 → 新摘要")).toBeInTheDocument();
});

test("采用 AI Description 建议只更新草稿而不立即保存", async () => {
  const save = vi.fn();
  render(<MetadataForm value={metadata} sourceChanged={false} externalSuggestion={{ id: "description-1", field: "description", value: "AI 生成的摘要" }} onSave={save} />);
  expect(await screen.findByDisplayValue("AI 生成的摘要")).toBeInTheDocument();
  expect(save).not.toHaveBeenCalled();
});

test("Keywords 建议替换数组，Tags 建议大小写不敏感去重", async () => {
  const view = render(<MetadataForm value={metadata} sourceChanged={false} externalSuggestion={{ id: "keywords-1", field: "keywords", value: ["go", "ai"] }} onSave={vi.fn()} />);
  expect(await screen.findByDisplayValue("go, ai")).toBeInTheDocument();
  view.rerender(<MetadataForm value={metadata} sourceChanged={false} externalSuggestion={{ id: "tags-1", field: "tags", value: "go" }} onSave={vi.fn()} />);
  expect(screen.getAllByText("Go")).toHaveLength(1);
});

test("批量采用多个 AI 建议时全部进入文章草稿", async () => {
  render(<MetadataForm value={metadata} sourceChanged={false} externalSuggestions={[
    { id: "description-1", field: "description", value: "新的摘要" },
    { id: "category-1", field: "category", value: "AI 应用开发" },
    { id: "tag-1", field: "tags", value: "AI" },
  ]} onSave={vi.fn()} />);
  expect(await screen.findByDisplayValue("新的摘要")).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "Category" })).toHaveValue("AI 应用开发");
  expect(screen.getByText("AI")).toBeInTheDocument();
});

test("文章基线刷新与 AI 采用同时发生时不丢失建议值", async () => {
  const view = render(<MetadataForm value={metadata} sourceChanged={false} externalSuggestions={[{ id: "description-1", field: "description", value: "AI 摘要" }]} onSave={vi.fn()} />);
  expect(await screen.findByDisplayValue("AI 摘要")).toBeInTheDocument();
  view.rerender(<MetadataForm value={{ ...metadata, title: "刷新后的标题" }} sourceChanged={false} externalSuggestions={[{ id: "description-1", field: "description", value: "AI 摘要" }]} onSave={vi.fn()} />);
  expect(await screen.findByDisplayValue("AI 摘要")).toBeInTheDocument();
  expect(screen.getByDisplayValue("刷新后的标题")).toBeInTheDocument();
});

test("Category 从 Hugo 快照选择并进入文章草稿", async () => {
  render(<MetadataForm value={metadata} sourceChanged={false} taxonomyState="ready" categoryOptions={[{ key: "engineering", name: "工程实践" }, { key: "product", name: "产品" }]} onSave={vi.fn()} />);
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Category" }), "产品");
  expect(screen.getByText("Category：工程实践 → 产品")).toBeInTheDocument();
});

test("Series 从 Hugo 快照选择并进入文章草稿", async () => {
  render(<MetadataForm value={metadata} sourceChanged={false} taxonomyState="ready" seriesOptions={[{ key: "inkhub", name: "InkHub" }, { key: "go-course", name: "Go 课程" }]} onSave={vi.fn()} />);
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Series" }), "Go 课程");
  expect(screen.getByText("Series：InkHub → Go 课程")).toBeInTheDocument();
});

test("Tags 从 Hugo 快照多选并允许创建新 Tag", async () => {
  const save = vi.fn();
  render(<MetadataForm value={metadata} sourceChanged={false} taxonomyState="ready" tagOptions={[{ key: "go", name: "Go", usageCount: 18 }, { key: "sqlite", name: "SQLite", usageCount: 7 }]} onSave={save} />);
  const input = screen.getByRole("combobox", { name: "搜索或创建 Tag" });
  await userEvent.click(input);
  expect(screen.getByRole("option", { name: "SQLite，7 篇文章" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("option", { name: "SQLite，7 篇文章" }));
  expect(screen.getByText("Tags：Go、React → Go、React、SQLite")).toBeInTheDocument();
  await userEvent.type(input, "New Topic{Enter}");
  expect(screen.getByText("Tags：Go、React → Go、React、SQLite、New Topic")).toBeInTheDocument();
  expect(save).not.toHaveBeenCalled();
});

test("Category 快照保留博客中未发现的文章旧值", () => {
  render(<MetadataForm value={{ ...metadata, category: "旧分类" }} sourceChanged={false} taxonomyState="ready" categoryOptions={[{ key: "engineering", name: "工程实践" }]} onSave={vi.fn()} />);
  expect(screen.getByRole("combobox", { name: "Category" })).toHaveValue("旧分类");
  expect(screen.getByRole("option", { name: "旧分类（博客中未发现）" })).toBeInTheDocument();
});

test("新建 Category 只回填草稿而不提前保存文章", async () => {
  const save = vi.fn();
  render(<MetadataForm value={metadata} sourceChanged={false} taxonomyState="ready" categoryOptions={[]} canCreateTaxonomy onCreateTaxonomy={(_kind, select) => select("AI")} onSave={save} />);
  await userEvent.click(screen.getByRole("button", { name: "新建类目" }));
  expect(screen.getByText("Category：工程实践 → AI")).toBeInTheDocument();
  expect(save).not.toHaveBeenCalled();
});

test("文章页读取 Hugo Category 且 taxonomy 失败不阻止审核", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy") ? Response.json(taxonomy) : Response.json(article));
  const view = render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByRole("combobox", { name: "Category" })).toHaveValue("工程实践");
  expect(screen.getByRole("option", { name: "产品" })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "Series" })).toHaveValue("InkHub");
  expect(screen.getByRole("option", { name: "Go 课程" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("combobox", { name: "搜索或创建 Tag" }));
  expect(screen.getByRole("option", { name: "SQLite，7 篇文章" })).toBeInTheDocument();
  view.unmount();

  vi.restoreAllMocks();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy") ? Response.json({ error: { message: "类目读取失败" } }, { status: 500 }) : Response.json(article));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("本地优先的内容工作流")).toBeInTheDocument();
  expect(screen.getByText("博客类目暂不可用")).toBeInTheDocument();
  expect(screen.getByText("博客系列暂不可用")).toBeInTheDocument();
});

test("正文有一级标题时文章预览不重复显示元数据标题", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, preview_html: "<h1>正文标题</h1><p>正文</p>" }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByRole("heading", { name: "正文标题", level: 1 })).toBeInTheDocument();
  expect(screen.queryByRole("heading", { name: metadata.title, level: 1 })).not.toBeInTheDocument();
});

test("文章页未连接博客时给出明确 Category 引导", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy") ? Response.json({ source: "尚未配置", state: "not_enabled", loaded_at: "-", readonly: true, terms: [], issues: [] }) : Response.json(article));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("尚未连接博客类目")).toBeInTheDocument();
  expect(screen.getByText("尚未连接博客系列")).toBeInTheDocument();
});

test("文章页创建 Category 后回填草稿并等待用户保存", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/taxonomy/terms/preview")) return Response.json({ provider_id: "h1", expected_revision: "revision-1", files: [{ relative_path: "content/categories/ai/_index.md", before: "", after: "---\ntitle: AI\n---\n" }] });
    if (url.endsWith("/taxonomy/terms/apply")) return Response.json({ ...taxonomy, revision: "revision-2", terms: [...taxonomy.terms, { kind: "category", key: "ai", name: "AI", usage_count: 0, metadata: {} }] });
    if (url.endsWith("/taxonomy")) return Response.json(taxonomy);
    if (init?.method === "PUT") return Response.json({ ...article, metadata: { ...metadata, category: "AI" } });
    return Response.json(article);
  });
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByRole("combobox", { name: "Category" });
  await userEvent.click(screen.getByRole("button", { name: "新建类目" }));
  await userEvent.type(screen.getByLabelText("类目名称"), "AI");
  await userEvent.click(screen.getByRole("button", { name: "预览变更" }));
  await screen.findByText("content/categories/ai/_index.md");
  await userEvent.click(screen.getByRole("button", { name: "确认创建类目" }));
  expect(screen.getByRole("combobox", { name: "Category" })).toHaveValue("AI");
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === "PUT")).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "保存到文章" }));
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === "PUT")).toBe(true);
});

test("文章页创建 Series 时提交正确 kind 且只回填 Series 草稿", async () => {
  const requestBodies: string[] = [];
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (init?.body) requestBodies.push(String(init.body));
    if (url.endsWith("/taxonomy/terms/preview")) return Response.json({ provider_id: "h1", expected_revision: "revision-1", files: [{ relative_path: "content/series/ai/_index.md", before: "", after: "---\ntitle: AI 系列\n---\n" }] });
    if (url.endsWith("/taxonomy/terms/apply")) return Response.json({ ...taxonomy, revision: "revision-2", terms: [...taxonomy.terms, { kind: "series", key: "ai", name: "AI 系列", usage_count: 0, metadata: {} }] });
    if (url.endsWith("/taxonomy")) return Response.json(taxonomy);
    return Response.json(article);
  });
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByRole("combobox", { name: "Series" });
  await userEvent.click(screen.getByRole("button", { name: "新建系列" }));
  await userEvent.type(screen.getByLabelText("系列名称"), "AI 系列");
  await userEvent.click(screen.getByRole("button", { name: "预览变更" }));
  expect(await screen.findByText("content/series/ai/_index.md")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "确认创建系列" }));
  expect(requestBodies.some((body) => JSON.parse(body).kind === "series")).toBe(true);
  expect(screen.getByRole("combobox", { name: "Series" })).toHaveValue("AI 系列");
  expect(screen.getByRole("combobox", { name: "Category" })).toHaveValue("工程实践");
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === "PUT")).toBe(false);
  expect(await screen.findByRole("status")).toHaveTextContent("系列已创建");
});

test("AI 建议只能逐字段采用并写入表单草稿", async () => {
  const accept = vi.fn();
  render(<AISuggestions stale={false} suggestions={[{ id: "s1", field: "tags", name: "SQLite", reason: "主题匹配", new_term: false, usage_count: 7 }]} onAccept={accept} onGenerate={vi.fn()} />);
  expect(screen.getByRole("heading", { name: "AI 建议中心" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "采用全部" })).toBeInTheDocument();
  expect(screen.getByText("7 篇文章")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "采用 SQLite" }));
  expect(accept).toHaveBeenCalledWith(expect.objectContaining({ name: "SQLite" }));
});

test("重新生成 AI 建议需要二次确认", async () => {
  const generate = vi.fn();
  render(<AISuggestions stale={false} suggestions={[{ id: "s1", field: "tags", name: "SQLite", reason: "主题匹配", new_term: false, usage_count: 7 }]} onAccept={vi.fn()} onGenerate={generate} />);
  await userEvent.click(screen.getByRole("button", { name: "生成 AI 建议" }));
  expect(generate).not.toHaveBeenCalled();
  expect(screen.getByText("重新生成会创建新的建议版本，继续吗？")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(generate).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: "生成 AI 建议" }));
  await userEvent.click(screen.getByRole("button", { name: "确认生成" }));
  expect(generate).toHaveBeenCalledTimes(1);
});

test("已忽略建议的显示切换位于建议中心标题栏", async () => {
  render(<AISuggestions stale={false} suggestions={[{ id: "s1", field: "tags", name: "SQLite", reason: "主题匹配", new_term: false, usage_count: 7 }]} onAccept={vi.fn()} onGenerate={vi.fn()} />);
  await userEvent.click(screen.getByRole("button", { name: "忽略 SQLite" }));
  const heading = screen.getByRole("heading", { name: "AI 建议中心" });
  const headingActions = heading.closest(".tool-heading");
  expect(headingActions).not.toBeNull();
  expect(within(headingActions as HTMLElement).getByRole("button", { name: "显示已忽略（1）" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "显示已忽略（1）" }));
  expect(within(headingActions as HTMLElement).getByRole("button", { name: "隐藏已忽略" })).toBeInTheDocument();
});

test("文章更新后 AI 建议标记过期但仍可继续采用", async () => {
  render(<AISuggestions stale suggestions={[{ id: "s1", field: "tags", name: "Agent", reason: "主题匹配", new_term: true, usage_count: 0 }]} onAccept={vi.fn()} onGenerate={vi.fn()} />);
  expect(screen.getByText("文章已更新，当前建议基于旧版本，仍可继续采用")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "采用 Agent" })).toBeEnabled();
});

test("过期建议提示可选重新生成并提供入口", async () => {
  const generate = vi.fn();
  render(<AISuggestions stale suggestions={[{ id: "s1", field: "tags", name: "Agent", reason: "主题匹配", new_term: true, usage_count: 0 }]} onAccept={vi.fn()} onGenerate={generate} />);
  expect(screen.getByText("文章已更新，当前建议基于旧版本，仍可继续采用")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "重新生成" }));
  expect(screen.getByText("重新生成会创建新的建议版本，继续吗？")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "确认生成" }));
  expect(generate).toHaveBeenCalledOnce();
});

test("AI 建议中心按字段展示字符串和数组建议", () => {
  render(<AISuggestions stale={false} suggestions={[
    { id: "description-1", field: "description", name: "新的描述", value: "新的描述", reason: "摘要", new_term: false, usage_count: 0 },
    { id: "category-1", field: "category", name: "AI 应用开发", reason: "分类", new_term: false, usage_count: 0 },
    { id: "series-1", field: "series", name: "入门系列", reason: "系列", new_term: false, usage_count: 0 },
    { id: "slug-1", field: "slug", name: "ai-guide", reason: "地址", new_term: false, usage_count: 0 },
    { id: "keywords-1", field: "keywords", name: "Go、AI", value: ["Go", "AI"], reason: "关键词", new_term: false, usage_count: 0 },
    { id: "tags-1", field: "tags", name: "agent", reason: "标签", new_term: true, usage_count: 0 },
  ]} onAccept={vi.fn()} onGenerate={vi.fn()} />);
  for (const label of ["Description", "Category", "Series", "Slug", "Keywords", "Tags"]) expect(screen.getByRole("heading", { name: label })).toBeInTheDocument();
  expect(screen.getByText("Go、AI")).toBeInTheDocument();
});

test("文章页生成 AI Tag 并逐项加入草稿", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/suggestions") && init?.method === "POST") return Response.json({ suggestions: [{ id: "s1", field: "tags", name: "SQLite", reason: "数据层主题", new_term: false, usage_count: 7 }, { id: "s2", field: "tags", name: "Agent", reason: "智能体主题", new_term: true, usage_count: 0 }], suggestions_stale: false });
    if (url.endsWith("/taxonomy")) return Response.json(taxonomy);
    return Response.json({ ...article, ai_configured: true });
  });
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByRole("button", { name: "生成 AI 建议" });
  await userEvent.click(screen.getByRole("button", { name: "生成 AI 建议" }));
  expect(await screen.findByText("新 Tag")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "采用 SQLite" }));
  expect(screen.getByText("Tags：Go、React → Go、React、SQLite")).toBeInTheDocument();
});

test("文章页采用 AI 分类和描述建议后更新表单草稿", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/suggestions") && init?.method === "POST") return Response.json({ suggestions: [{ id: "s-description", field: "description", name: "AI 摘要", reason: "", new_term: false, usage_count: 0 }, { id: "s-category", field: "category", name: "AI 应用开发", reason: "", new_term: false, usage_count: 0 }], suggestions_stale: false });
    if (url.endsWith("/taxonomy")) return Response.json(taxonomy);
    return Response.json({ ...article, ai_configured: true });
  });
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  await userEvent.click(await screen.findByRole("button", { name: "生成 AI 建议" }));
  await userEvent.click(await screen.findByRole("button", { name: "采用 AI 摘要" }));
  await userEvent.click(screen.getByRole("button", { name: "采用 AI 应用开发" }));
  expect(await screen.findByDisplayValue("AI 摘要")).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "Category" })).toHaveValue("AI 应用开发");
});

test("发布轨道不暴露内部 hash 和任务 ID", () => {
  render(<PublicationTrack review="已通过" hugo="需要同步" wechat="尚未准备" />);
  expect(screen.getByText("审核")).toBeInTheDocument();
  expect(screen.getByText("需要同步")).toBeInTheDocument();
  expect(screen.queryByText(/hash|job_/i)).not.toBeInTheDocument();
});

test("文章页显示当前版本已发表渠道提示", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, disposition: { kind: "published", channels: ["hugo", "wechat"] } }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("当前版本已标记为外部发表：Hugo、微信")).toBeInTheDocument();
});

test("小红书入口只出现在发布操作区，不出现在审核工具列表底部", async () => {
  const navigate = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, review_state: "已通过", hugo_state: "已同步", wechat_state: "尚未准备" }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={navigate} /></ToastProvider>);
  const publishButton = await screen.findByRole("button", { name: /发布到小红书/ });
  expect(screen.queryByRole("button", { name: "打开内容中心" })).not.toBeInTheDocument();
  await userEvent.click(publishButton);
  expect(navigate).toHaveBeenCalledWith("/articles/article-1/xiaohongshu");
});

test("文章页显示长期忽略提示", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, disposition: { kind: "ignored", channels: [] } }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("此文章已忽略，可在内容库恢复")).toBeInTheDocument();
});

test("文章页显示图片资源诊断但保留文章流程", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, content_stage: "ready", resource_diagnostics: [{ code: "source.image_unresolved", message: "图片引用无法解析: missing.png", blocking: false }] }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("图片引用需要处理")).toBeInTheDocument();
  expect(screen.getByText("图片引用无法解析: missing.png")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "审核通过" })).toBeInTheDocument();
});

test("Hugo 任务失败时显示失败步骤和重试且不宣称成功", () => {
  render(<JobStatus state="failed" progress={68} stage="构建预览" message="Hugo build 未通过" onRetry={vi.fn()} />);
  expect(screen.getByText("构建预览失败")).toBeInTheDocument();
  expect(screen.getByText("Hugo build 未通过")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
  expect(screen.queryByText("同步完成")).not.toBeInTheDocument();
});

test("审核页不展开 Hugo 流程并提供三个独立渠道入口", async () => {
  const navigate = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, review_state: "已通过", hugo_state: "需要同步", wechat_state: "尚未准备", xiaohongshu_state: "尚未准备" }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={navigate} /></ToastProvider>);

  expect(await screen.findByRole("button", { name: /同步到 Hugo/ })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /发布到微信/ })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /发布到小红书/ })).toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "发布目录" })).not.toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: /同步到 Hugo/ }));
  expect(navigate).toHaveBeenCalledWith("/articles/article-1/hugo");
});

test("已同步 Hugo 的文章仍可直接进入微信和小红书", async () => {
  const navigate = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, review_state: "已通过", hugo_state: "已同步", wechat_state: "尚未准备", xiaohongshu_state: "尚未准备" }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={navigate} /></ToastProvider>);

  await userEvent.click(await screen.findByRole("button", { name: /发布到微信/ }));
  await userEvent.click(screen.getByRole("button", { name: /发布到小红书/ }));
  expect(navigate).toHaveBeenNthCalledWith(1, "/articles/article-1/wechat");
  expect(navigate).toHaveBeenNthCalledWith(2, "/articles/article-1/xiaohongshu");
  expect(screen.queryByText("content/posts/restored")).not.toBeInTheDocument();
});

test("审核未通过时发布渠道可见但不可执行", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy") ? Response.json(taxonomy) : Response.json(article));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByRole("button", { name: /同步到 Hugo.*审核通过后可用/ })).toBeDisabled();
  expect(screen.getByRole("button", { name: /发布到微信.*审核通过后可用/ })).toBeDisabled();
  expect(screen.getByRole("button", { name: /发布到小红书.*审核通过后可用/ })).toBeDisabled();
});

test("微信必须先复制当前内容才能人工确认草稿", async () => {
  const copy = vi.fn().mockResolvedValue(undefined);
  const confirm = vi.fn();
  render(<WeChatActions copied={false} onCopy={copy} onConfirm={confirm} />);
  expect(screen.queryByRole("button", { name: "草稿已保存" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "复制格式化内容" }));
  expect(copy).toHaveBeenCalledOnce();
});

test("剪贴板失败后恢复复制按钮并提供 HTML 兜底", async () => {
  render(<WeChatActions copied={false} onCopy={vi.fn().mockRejectedValue(new Error("denied"))} onConfirm={vi.fn()} />);
  await userEvent.click(screen.getByRole("button", { name: "复制格式化内容" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("无法写入剪贴板");
  expect(screen.getByRole("button", { name: "复制格式化内容" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "查看 HTML" })).toBeInTheDocument();
});
