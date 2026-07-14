import type { ArticlePage, JobStatus, SessionResponse, WorkspaceDraft } from "./types";

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

/** pickDirectory 请求本机进程打开系统目录选择器。 */
export function pickDirectory() {
  return request<{ path: string }>("/directories/pick", { method: "POST", body: "{}" });
}

/** getJob 恢复页面刷新前已经提交的扫描任务。 */
export function getJob(jobID: string, signal?: AbortSignal) {
  return request<JobStatus>(`/jobs/${encodeURIComponent(jobID)}`, { signal });
}
