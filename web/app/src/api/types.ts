export type ArticleState = "blocked" | "changed" | "incomplete" | "pending_review" | "approved";

export interface WorkspaceSummary {
  id: string;
  name: string;
}

export interface SessionResponse {
  has_workspace: boolean;
  workspace: WorkspaceSummary | null;
}

export interface ArticleSummary {
  id: string;
  title: string;
  directory: string;
  category: string;
  modified_at: string;
  state: ArticleState;
  hugo_state: string;
  wechat_state: string;
}

export interface ArticlePage {
  items: ArticleSummary[];
  next_cursor?: string;
}

export interface WorkspaceDraft {
  name: string;
  vault_path: string;
  hugo_path?: string;
  wechat_template: string;
  ai_enabled: boolean;
}

export interface JobStatus {
  id: string;
  state: "queued" | "running" | "succeeded" | "failed";
  progress: number;
  indexed?: number;
  failed?: number;
}
