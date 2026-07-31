import { render, screen } from "@testing-library/react";
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
  expect(screen.queryByRole("button", { name: /全部/ })).not.toBeInTheDocument();
  expect(screen.getByText("7 篇文章")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "采用 Tag SQLite" }));
  expect(accept).toHaveBeenCalledWith(expect.objectContaining({ name: "SQLite" }));
});

test("文章更新后 AI 建议过期且不能继续采用", () => {
  render(<AISuggestions stale suggestions={[{ id: "s1", field: "tags", name: "Agent", reason: "主题匹配", new_term: true, usage_count: 0 }]} onAccept={vi.fn()} onGenerate={vi.fn()} />);
  expect(screen.getByText("文章已更新，请重新分析")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "采用 Tag Agent" })).toBeDisabled();
});

test("文章页生成 AI Tag 并逐项加入草稿", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/suggestions") && init?.method === "POST") return Response.json({ suggestions: [{ id: "s1", field: "tags", name: "SQLite", reason: "数据层主题", new_term: false, usage_count: 7 }, { id: "s2", field: "tags", name: "Agent", reason: "智能体主题", new_term: true, usage_count: 0 }], suggestions_stale: false });
    if (url.endsWith("/taxonomy")) return Response.json(taxonomy);
    return Response.json({ ...article, ai_configured: true });
  });
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  await screen.findByRole("button", { name: "生成 AI Tag" });
  await userEvent.click(screen.getByRole("button", { name: "生成 AI Tag" }));
  expect(await screen.findByText("新 Tag")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "采用 Tag SQLite" }));
  expect(screen.getByText("Tags：Go、React → Go、React、SQLite")).toBeInTheDocument();
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

test("文章页显示长期忽略提示", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).endsWith("/taxonomy")
    ? Response.json(taxonomy)
    : Response.json({ ...article, disposition: { kind: "ignored", channels: [] } }));
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("此文章已忽略，可在内容库恢复")).toBeInTheDocument();
});

test("Hugo 任务失败时显示失败步骤和重试且不宣称成功", () => {
  render(<JobStatus state="failed" progress={68} stage="构建预览" message="Hugo build 未通过" onRetry={vi.fn()} />);
  expect(screen.getByText("构建预览失败")).toBeInTheDocument();
  expect(screen.getByText("Hugo build 未通过")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
  expect(screen.queryByText("同步完成")).not.toBeInTheDocument();
});

test("文章页同步 Hugo 时展开预览流程而不调用旧发布接口", async () => {
  const requests: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    requests.push(url);
    if (url.endsWith("/taxonomy")) return Response.json(taxonomy);
    if (url.endsWith("/hugo-sections")) return Response.json({ sections: [{ name: "posts", article_count: 8 }], existing_section: "", selection_locked: false });
    return Response.json({ ...article, review_state: "已通过", hugo_state: "需要同步" });
  });
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  await userEvent.click(await screen.findByRole("button", { name: "同步到 Hugo" }));
  expect(await screen.findByRole("combobox", { name: "发布目录" })).toHaveValue("posts");
  expect(requests.some((url) => url.endsWith("/publications"))).toBe(false);
});

test("文章页刷新后自动展开 Ready Hugo 预览", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/taxonomy")) return Response.json(taxonomy);
    if (url.endsWith("/publication-workflow")) return Response.json({ article_id: "article-1", hugo: { state: "ready", progress: 100, stage: "预览已准备", preview: { preview_id: "preview_ready", section: "posts", target_path: "content/posts/restored", change: "updated", files: [], diagnostics: [], state: "ready" } } });
    return Response.json({ ...article, review_state: "已通过", hugo_state: "需要同步" });
  });
  render(<ToastProvider><ArticlePage articleID="article-1" onNavigate={vi.fn()} /></ToastProvider>);
  expect(await screen.findByText("content/posts/restored")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认同步到 Hugo" })).toBeInTheDocument();
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
