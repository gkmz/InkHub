import type { ArticleDetail, ArticleMetadata, ArticlePage, BatchDispositionCommand, BatchDispositionResult, DashboardView, DirectoryCandidate, HugoPreviewView, HugoSectionView, JobStatus, MermaidTheme, PublicationHistoryPage, PublicationWorkflowView, SessionResponse, SettingsView, SuggestionHistoryResponse, SuggestionVersionView, TaxonomyChangePreview, TaxonomyOverview, TaxonomyTermCommand, WeChatPlanView, WorkspaceDraft, XiaohongshuDraft, XiaohongshuRewriteOutline, XiaohongshuView } from "./types";
import type { HugoTakeoverReport } from "./types";
import type { XiaohongshuDraftMode } from "./types";

/** APIError 保留服务端稳定错误码，页面只展示可理解的中文消息。 */
export class APIError extends Error {
  constructor(public readonly code: string, message: string, public readonly status: number) {
    super(message);
  }
}

async function request<T extends object>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: init?.body ? { "Content-Type": "application/json", ...init.headers } : init?.headers,
  });
  const body = await response.json() as T | { error?: { code?: string; message?: string } };
  if (!response.ok) {
    const error = "error" in body ? body.error : undefined;
    throw new APIError(error?.code ?? "request.failed", error?.message ?? "请求失败", response.status);
  }
  return body as T;
}

/** getSession 读取最近工作区，用于决定是否进入初始化。 */
export function getSession(signal?: AbortSignal) {
  return request<SessionResponse>("/session", { signal });
}

/** getDashboard 获取按处理优先级组织的稿件。 */
export function getDashboard(signal?: AbortSignal) {
  return request<DashboardView>("/dashboard", { signal });
}

/** refreshWorkspace 重新扫描最近工作区的 Markdown 内容并返回索引统计。 */
export function refreshWorkspace() {
  return request<{ indexed: number; failed: number }>("/workspace/refresh", { method: "POST", body: "{}" });
}

/** listArticles 读取内容库稳定分页，并透传搜索与筛选。 */
export function listArticles(query: URLSearchParams, signal?: AbortSignal) {
  return request<ArticlePage>(`/articles?${query.toString()}`, { signal });
}

/** batchDisposition 原子提交当前用户已选择文章的管理处置。 */
export function batchDisposition(command: BatchDispositionCommand) {
  return request<BatchDispositionResult>("/articles/batch-disposition", {
    method: "POST",
    body: JSON.stringify(command),
  });
}

/** createWorkspace 幂等创建工作区并返回扫描任务。 */
export function createWorkspace(draft: WorkspaceDraft, idempotencyKey: string) {
  return request<{ workspace: { id: string; name: string }; job_id: string }>("/workspaces", {
    method: "POST",
    headers: { "Idempotency-Key": idempotencyKey },
    body: JSON.stringify(draft),
  });
}

/** pickDirectory 按固定用途请求本机进程打开系统目录选择器。 */
export function pickDirectory(purpose: "vault" | "hugo") {
  return request<{ path: string }>("/directories/pick", { method: "POST", body: JSON.stringify({ purpose }) });
}

/** inspectDirectories 只读取 Vault 目录级 Markdown 数量。 */
export function inspectDirectories(vaultPath: string) {
  return request<{ directories: DirectoryCandidate[] }>("/directories/inspect", { method: "POST", body: JSON.stringify({ vault_path: vaultPath }) });
}

/** getJob 恢复页面刷新前已经提交的扫描任务。 */
export function getJob(jobID: string, signal?: AbortSignal) {
  return request<JobStatus>(`/jobs/${encodeURIComponent(jobID)}`, { signal });
}

/** getArticle 读取文章详情和当前审核、渠道状态。 */
export function getArticle(articleID: string, signal?: AbortSignal) {
  return request<ArticleDetail>(`/articles/${encodeURIComponent(articleID)}`, { signal });
}

/** getHugoSections 读取 Hugo 当前可选一级发布目录。 */
export function getHugoSections(articleID: string, signal?: AbortSignal) {
  return request<HugoSectionView>(`/articles/${encodeURIComponent(articleID)}/hugo-sections`, { signal });
}

/** getPublicationWorkflow 恢复当前文章版本的 Hugo 发布状态。 */
export function getPublicationWorkflow(articleID: string, signal?: AbortSignal) {
  return request<PublicationWorkflowView>(`/articles/${encodeURIComponent(articleID)}/publication-workflow`, { signal });
}

/** getPublicationHistory 稳定分页读取 Hugo 与微信统一历史。 */
export function getPublicationHistory(articleID: string, cursor = "", signal?: AbortSignal) {
  const query = new URLSearchParams({ limit: "20" });
  if (cursor) query.set("cursor", cursor);
  return request<PublicationHistoryPage>(`/articles/${encodeURIComponent(articleID)}/publication-history?${query.toString()}`, { signal });
}

/** getWeChatPlan 只读生成当前模板和本地图片准备清单。 */
export function getWeChatPlan(articleID: string, templateID: string, mermaidTheme: MermaidTheme, signal?: AbortSignal) {
  return request<WeChatPlanView>(`/articles/${encodeURIComponent(articleID)}/wechat-plans`, { method: "POST", body: JSON.stringify({ template_id: templateID, mermaid_theme: mermaidTheme }), signal });
}

/** confirmWeChatPlan 使用服务端签名计划创建准备任务。 */
export function confirmWeChatPlan(articleID: string, planToken: string, signal?: AbortSignal) {
  return request<{ state: "queued" }>(`/articles/${encodeURIComponent(articleID)}/wechat-plans/confirm`, { method: "POST", body: JSON.stringify({ plan_token: planToken }), signal });
}

/** createHugoPreview 为指定内容版本和 Page Bundle 目录生成可确认的 staging Artifact。 */
export function createHugoPreview(articleID: string, contentHash: string, section: string, directory = "", refreshKey = "") {
  return request<{ id: string; job_id: string; state: string }>(`/articles/${encodeURIComponent(articleID)}/hugo-previews`, { method: "POST", body: JSON.stringify({ content_hash: contentHash, section, directory, refresh_key: refreshKey }) });
}

/** getHugoPreview 读取脱敏后的 Artifact 摘要。 */
export function getHugoPreview(previewID: string, signal?: AbortSignal) {
  return request<HugoPreviewView>(`/hugo-previews/${encodeURIComponent(previewID)}`, { signal });
}

/** confirmHugoPreview 确认交付已经审阅的同一 Artifact。 */
export function confirmHugoPreview(previewID: string) {
  return request<{ job_id: string; state: string }>(`/hugo-previews/${encodeURIComponent(previewID)}/confirm`, { method: "POST", body: "{}" });
}

/** generateArticleSuggestions 主动请求当前文章的结构化 AI 建议。 */
export function generateArticleSuggestions(articleID: string) {
  return request<Pick<ArticleDetail, "suggestions" | "suggestions_stale" | "suggestions_id" | "suggestions_generated_at">>(`/articles/${encodeURIComponent(articleID)}/suggestions`, { method: "POST", body: "{}" });
}

/** getSuggestionHistory 查询当前文章的 AI 建议生成历史。 */
export function getSuggestionHistory(articleID: string, signal?: AbortSignal) {
  return request<SuggestionHistoryResponse>(`/articles/${encodeURIComponent(articleID)}/suggestions?limit=20`, { signal });
}

/** getSuggestionVersion 读取指定 AI 建议版本的只读详情。 */
export function getSuggestionVersion(articleID: string, suggestionID: string, signal?: AbortSignal) {
  return request<SuggestionVersionView>(`/articles/${encodeURIComponent(articleID)}/suggestions/${encodeURIComponent(suggestionID)}`, { signal });
}

/** updateSuggestionItems 持久化指定建议版本的一批采用或忽略动作。 */
export function updateSuggestionItems(articleID: string, suggestionID: string, action: "accepted" | "ignored", itemIDs: string[]) {
  return request<SuggestionVersionView>(`/articles/${encodeURIComponent(articleID)}/suggestions/${encodeURIComponent(suggestionID)}/actions`, {
    method: "POST",
    body: JSON.stringify({ action, item_ids: itemIDs }),
  });
}

/** saveMetadata 使用源文件指纹约束写回，冲突由服务端拒绝。 */
export function saveMetadata(articleID: string, metadata: ArticleMetadata) {
  return request<ArticleDetail>(`/articles/${encodeURIComponent(articleID)}/metadata`, { method: "PUT", body: JSON.stringify({ metadata }) });
}

/** reviewArticle 审核当前内容版本。 */
export function reviewArticle(articleID: string) {
  return request<{ state: string }>(`/articles/${encodeURIComponent(articleID)}/review`, { method: "POST", body: "{}" });
}

/** startPublication 创建指定渠道任务。 */
export function startPublication(article: Pick<ArticleDetail, "id" | "content_version" | "hugo_provider_id" | "wechat_provider_id">, channel: "hugo" | "wechat") {
  return request<{ job_id: string }>("/publications", { method: "POST", body: JSON.stringify({ article_id: article.id, channel, provider_instance_id: channel === "hugo" ? article.hugo_provider_id : article.wechat_provider_id, content_hash: article.content_version }) });
}

/** confirmWeChatDraft 记录当前内容已人工保存到公众号草稿。 */
export function confirmWeChatDraft(article: Pick<ArticleDetail, "id" | "content_version" | "wechat_provider_id">) {
  return request<{ state: string }>("/wechat/confirm", { method: "POST", body: JSON.stringify({ article_id: article.id, provider_instance_id: article.wechat_provider_id, content_hash: article.content_version }) });
}

/** markWeChatCopied 仅在浏览器成功写入当前模板 HTML 后记录复制状态。 */
export function markWeChatCopied(article: Pick<ArticleDetail, "id" | "content_version" | "wechat_provider_id">) {
  return request<{ state: string }>("/wechat/copied", { method: "POST", body: JSON.stringify({ article_id: article.id, provider_instance_id: article.wechat_provider_id, content_hash: article.content_version }) });
}

/** getXiaohongshu 读取小红书当前草稿和版本历史。 */
export function getXiaohongshu(articleID: string, mode: XiaohongshuDraftMode = "long_card", signal?: AbortSignal) {
  return request<XiaohongshuView>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu?mode=${encodeURIComponent(mode)}`, { signal });
}

/** generateXiaohongshuDraft 调用 AI 提炼并生成新的小红书草稿版本，不覆盖历史。 */
export function generateXiaohongshuDraft(articleID: string) {
  return request<XiaohongshuDraft>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu/drafts/generate`, { method: "POST", body: "{}" });
}

/** outlineXiaohongshuDraft 从原文提取必须由最终笔记覆盖的知识清单。 */
export function outlineXiaohongshuDraft(articleID: string) {
  return request<XiaohongshuRewriteOutline>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu/drafts/outline`, { method: "POST", body: "{}" });
}

/** rewriteXiaohongshuDraft 使用知识清单生成并保存新的小红书笔记版本。 */
export function rewriteXiaohongshuDraft(articleID: string, outline: XiaohongshuRewriteOutline) {
  return request<XiaohongshuDraft>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu/drafts/rewrite`, {
    method: "POST",
    body: JSON.stringify({ content_hash: outline.content_hash, knowledge_points: outline.knowledge_points }),
  });
}

/** generateXiaohongshuStoryboard 生成逐页生图提示词和配套发布短文。 */
export function generateXiaohongshuStoryboard(articleID: string) {
  return request<XiaohongshuDraft>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu/drafts/storyboard`, { method: "POST", body: "{}" });
}

/** saveXiaohongshuDraft 保存用户整体编辑后的小红书草稿。 */
export function saveXiaohongshuDraft(articleID: string, draft: Pick<XiaohongshuDraft, "id" | "mode" | "title" | "body_html" | "pages" | "script_pages" | "topics" | "source_note" | "comment_copy">) {
	return request<XiaohongshuDraft>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu/drafts`, { method: "POST", body: JSON.stringify({ draft_id: draft.id, mode: draft.mode, title: draft.title, body_html: draft.body_html, pages: draft.pages, script_pages: draft.script_pages, topics: draft.topics, source_note: draft.source_note, comment_copy: draft.comment_copy }) });
}

/** saveXiaohongshuRender 记录浏览器完成的手机模板渲染版本。 */
export function saveXiaohongshuRender(articleID: string, input: { draft_id: string; template_id: string; template_version: string; viewport_width: number; page_height: number; html_hash: string; page_count: number }) {
  return request<{ id: string; draft_id: string; state: string; page_count: number }>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu/renders`, { method: "POST", body: JSON.stringify(input) });
}

/** markXiaohongshuPublished 记录用户已经手动上传图片并发布。 */
export function markXiaohongshuPublished(articleID: string, draftID: string) {
  return request<{ state: string }>(`/articles/${encodeURIComponent(articleID)}/xiaohongshu/published`, { method: "POST", body: JSON.stringify({ draft_id: draftID }) });
}

/** getPreparedWeChatHTML 读取当前文章已经准备完成的安全模板 HTML。 */
export function getPreparedWeChatHTML(articleID: string) {
  return request<{ html: string }>(`/wechat/content/${encodeURIComponent(articleID)}`);
}

/** getTaxonomyOverview 读取权威 taxonomy 状态及待治理问题。 */
export function getTaxonomyOverview(signal?: AbortSignal) {
  return request<TaxonomyOverview>("/taxonomy", { signal });
}

/** refreshTaxonomy 从已配置博客重新生成类目快照。 */
export function refreshTaxonomy() {
  return request<TaxonomyOverview>("/taxonomy/refresh", { method: "POST", body: "{}" });
}

/** previewTaxonomyTerm 由 Provider 生成待写入文件，不接受客户端构造的文件内容。 */
export function previewTaxonomyTerm(command: TaxonomyTermCommand) {
  return request<TaxonomyChangePreview>("/taxonomy/terms/preview", { method: "POST", body: JSON.stringify(command) });
}

/** applyTaxonomyTerm 按预览使用的 revision 重新规划并应用类目变更。 */
export function applyTaxonomyTerm(command: TaxonomyTermCommand) {
  return request<TaxonomyOverview>("/taxonomy/terms/apply", { method: "POST", body: JSON.stringify(command) });
}

/** getSettings 读取不含 Secret 明文的设置视图。 */
export function getSettings(signal?: AbortSignal) {
  return request<SettingsView>("/settings", { signal });
}

/** saveAISettings 保存 OpenAI-compatible 非敏感配置和可选的新 API Key。 */
export function saveAISettings(input: { enabled: boolean; base_url: string; model: string; api_key: string }) {
  return request<{ ai_enabled: boolean; ai_secret_saved: boolean }>("/settings/ai", { method: "PUT", body: JSON.stringify(input) });
}

/** saveWeChatSettings 保存微信图片仓库非敏感配置和可选的新 Token。 */
export function saveWeChatSettings(input: { enabled: boolean; template: string; github_owner: string; github_repository: string; github_branch: string; github_prefix: string; github_token: string }) {
  return request<Partial<SettingsView>>("/settings/wechat", { method: "PUT", body: JSON.stringify(input) });
}

/** saveXiaohongshuSettings 保存小红书启用状态和默认模板。 */
export function saveXiaohongshuSettings(input: { enabled: boolean; template: string }) {
  return request<Partial<SettingsView>>("/settings/xiaohongshu", { method: "PUT", body: JSON.stringify(input) });
}

/** saveHugoSettings 校验并保存当前工作区的 Hugo 本机目录。 */
export function saveHugoSettings(input: { enabled: boolean; path: string; base_url: string }) {
  return request<Partial<SettingsView>>("/settings/hugo", { method: "PUT", body: JSON.stringify(input) });
}

/** previewHugoTakeover 只扫描历史内容并返回确定匹配和冲突。 */
export function previewHugoTakeover() {
  return request<HugoTakeoverReport>("/settings/hugo/takeover/preview", { method: "POST", body: "{}" });
}

/** confirmHugoTakeover 补齐 Stable ID 并写入历史 Hugo Bundle 关联。 */
export function confirmHugoTakeover() {
  return request<{ assigned_ids: number; recovered_articles: number; linked_bundles: number; remaining_source_issues: number; state: string }>("/settings/hugo/takeover/confirm", { method: "POST", body: "{}" });
}

/** saveContentScope 保存当前 Source 的目录规则并返回重扫结果。 */
export function saveContentScope(contentRoots: string[], ignoredFolders: string[], ignoredFileNames: string[]) {
  return request<{ indexed: number; failed: number }>("/settings/content-scope", { method: "PUT", body: JSON.stringify({ content_roots: contentRoots, ignored_folders: ignoredFolders, ignored_file_names: ignoredFileNames }) });
}

/** previewContentScope 计算保存目录规则将新增和移出的索引数量。 */
export function previewContentScope(contentRoots: string[], ignoredFolders: string[], ignoredFileNames: string[]) {
  return request<{ added: number; removed: number }>("/settings/content-scope/preview", { method: "POST", body: JSON.stringify({ content_roots: contentRoots, ignored_folders: ignoredFolders, ignored_file_names: ignoredFileNames }) });
}

/** saveCrossReferenceSections 保存交叉引用段落标题列表。 */
export function saveCrossReferenceSections(sections: string[]) {
  return request<{ cross_reference_sections: string[] }>("/settings/cross-reference", { method: "PUT", body: JSON.stringify({ sections }) });
}
