import type { ArticleDetail, ArticleMetadata, ArticlePage, DirectoryCandidate, JobStatus, SessionResponse, SettingsView, TaxonomyOverview, WorkspaceDraft } from "./types";

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
  return request<ArticlePage>("/dashboard", { signal });
}

/** listArticles 读取内容库稳定分页，并透传搜索与筛选。 */
export function listArticles(query: URLSearchParams, signal?: AbortSignal) {
  return request<ArticlePage>(`/articles?${query.toString()}`, { signal });
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

/** getPreparedWeChatHTML 读取当前文章已经准备完成的安全模板 HTML。 */
export function getPreparedWeChatHTML(articleID: string) {
  return request<{ html: string }>(`/wechat/content/${encodeURIComponent(articleID)}`);
}

/** getTaxonomyOverview 读取权威 taxonomy 状态及待治理问题。 */
export function getTaxonomyOverview(signal?: AbortSignal) {
  return request<TaxonomyOverview>("/taxonomy", { signal });
}

/** approveTaxonomyTerm 批准新词并请求更新明确的受影响文章。 */
export function approveTaxonomyTerm(issueID: string) {
  return request<{ state: string }>(`/taxonomy/issues/${encodeURIComponent(issueID)}/approve`, { method: "POST", body: "{}" });
}

/** getSettings 读取不含 Secret 明文的设置视图。 */
export function getSettings(signal?: AbortSignal) {
  return request<SettingsView>("/settings", { signal });
}

/** saveContentScope 保存当前 Source 的目录规则并返回重扫结果。 */
export function saveContentScope(contentRoots: string[], ignoredFolders: string[]) {
  return request<{ indexed: number; failed: number }>("/settings/content-scope", { method: "PUT", body: JSON.stringify({ content_roots: contentRoots, ignored_folders: ignoredFolders }) });
}

/** previewContentScope 计算保存目录规则将新增和移出的索引数量。 */
export function previewContentScope(contentRoots: string[], ignoredFolders: string[]) {
  return request<{ added: number; removed: number }>("/settings/content-scope/preview", { method: "POST", body: JSON.stringify({ content_roots: contentRoots, ignored_folders: ignoredFolders }) });
}
