export type ArticleState = "draft" | "blocked" | "changed" | "incomplete" | "pending_review" | "approved";
export type ContentStage = "draft" | "ready";
export type ArticleDisposition = "published" | "ignored";
export type PublicationChannel = "hugo" | "wechat";

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
  content_version: string;
  disposition?: ArticleDisposition;
  content_stage: ContentStage;
  content_stage_issue?: string;
  next_action?: "retry" | "review" | "publish" | "view";
}

export interface ArticlePage {
  items: ArticleSummary[];
  next_cursor?: string;
  available_channels: Array<"hugo" | "wechat">;
}

export interface DashboardView {
  failed: ArticleSummary[];
  changed: ArticleSummary[];
  needs_review: ArticleSummary[];
  ready_to_publish: ArticleSummary[];
  latest_ready: ArticleSummary[];
  recently_handled: ArticleSummary[];
}

export interface BatchDispositionCommand {
  operation: "published" | "ignored" | "restore";
  articles: Array<{ id: string; content_version: string }>;
  channels?: PublicationChannel[];
}

export interface BatchDispositionResult {
  processed: number;
  changed: number;
  unchanged: number;
}

export interface WorkspaceDraft {
  name: string;
  vault_path: string;
  hugo_path?: string;
  wechat_template: string;
  ai_enabled: boolean;
  content_roots: string[];
  ignored_folders: string[];
  ignored_file_names: string[];
}

export interface DirectoryCandidate {
  path: string;
  markdown_count: number;
}

export interface JobStatus {
  id: string;
  state: "queued" | "running" | "succeeded" | "failed";
  progress: number;
  indexed?: number;
  failed?: number;
}

export interface HugoSectionView {
  sections: Array<{ name: string; article_count: number }>;
  existing_section: string;
  selection_locked: boolean;
}

export interface HugoPreviewView {
  id: string;
  content_hash: string;
  section: string;
  target_path: string;
  change: "added" | "updated";
  files: Array<{ relative_path: string; media_type: string; size: number }>;
  diagnostics: Array<{ code: string; level: string; message: string }>;
  preview_url?: string;
  expires_at?: string;
  state: "preparing" | "ready" | "expired" | "failed";
  job_id: string;
  error?: string;
}

export interface RecoveredHugoPreviewView {
  preview_id: string;
  section: string;
  target_path: string;
  change: "added" | "updated";
  files: Array<{ relative_path: string; media_type: string; size: number }>;
  diagnostics: Array<{ code: string; level: string; message: string }>;
  preview_url?: string;
  expires_at?: string;
  state: "preparing" | "ready" | "expired" | "failed";
  error?: string;
}

export interface PublicationWorkflowView {
  article_id: string;
  hugo: null | {
    state: "preparing" | "ready" | "expired" | "failed" | "delivering" | "published";
    progress: number;
    stage: string;
    error?: string;
    preview?: RecoveredHugoPreviewView;
    delivery?: { state: string; progress: number; stage: string; error?: string };
  };
}

export interface PublicationHistoryItem {
  id: string;
  channel: "hugo" | "wechat";
  state: "prepared" | "copied" | "confirmed" | "published" | "failed";
  title: string;
  detail: string;
  occurred_at: string;
}

export interface PublicationHistoryPage {
  items: PublicationHistoryItem[];
  next_cursor?: string;
}

export interface WeChatPlanView {
  plan_token: string;
  template_id: string;
  images: Array<{ reference: string; media_type: string; size: number; state: "upload" | "reuse" }>;
  diagnostics: Array<{ code: string; message: string; blocking: boolean }>;
  ready: boolean;
  expires_at: string;
}

export interface ArticleMetadata {
  title: string;
  description: string;
  category: string;
  series: string;
  tags: string[];
  keywords: string[];
  slug: string;
  cover: string;
}

export interface CheckResult {
  id: string;
  level: "blocking" | "recommended" | "optional" | "passed";
  title: string;
  detail: string;
  channel: string;
}

export interface AISuggestion {
  id: string;
  field: keyof ArticleMetadata;
  name: string;
  reason: string;
  new_term: boolean;
  usage_count: number;
}

export interface ArticleDetail {
  id: string;
  content_version: string;
  content_stage: ContentStage;
  content_stage_issue?: string;
  hugo_provider_id: string;
  wechat_provider_id: string;
  relative_path: string;
  modified_at: string;
  metadata: ArticleMetadata;
  preview_html: string;
  source_changed: boolean;
  review_state: string;
  hugo_state: string;
  wechat_state: string;
  checks: CheckResult[];
  ai_configured: boolean;
  suggestions: AISuggestion[];
  suggestions_stale: boolean;
  wechat_copied: boolean;
  disposition?: { kind: ArticleDisposition; channels: PublicationChannel[] };
}

export interface TemplateSummary {
  id: string;
  name: string;
  version: string;
  compatible: boolean;
}

export interface TaxonomyIssue {
  id: string;
  kind: "alias" | "unknown" | "low_frequency" | "too_many";
  term: string;
  similar: string[];
  affected: string[];
}

export interface TaxonomyOverview {
  source: string;
  provider_id?: string;
  provider_type?: string;
  state: "ready" | "failed" | "not_loaded" | "not_enabled";
  revision?: string;
  loaded_at: string;
  attempted_at?: string;
  readonly: boolean;
  error?: string;
  error_code?: string;
  terms: TaxonomyTerm[];
  issues: TaxonomyIssue[];
}

export interface TaxonomyTerm {
  kind: string;
  key: string;
  name: string;
  usage_count: number;
  metadata: Record<string, string>;
}

export interface TaxonomyTermCommand {
  provider_id: string;
  kind: string;
  key?: string;
  name: string;
  description: string;
  aliases: string[];
  expected_revision: string;
}

export interface TaxonomyChangePreview {
  provider_id: string;
  expected_revision: string;
  files: { relative_path: string; before: string; after: string }[];
}

export interface SettingsView {
  workspace_name: string;
  vault_path: string;
  content_roots: string[];
  ignored_folders: string[];
  directories: DirectoryCandidate[];
  ignored_file_names: string[];
  ai_enabled: boolean;
  ai_secret_saved: boolean;
  ai_base_url?: string;
  ai_model?: string;
  hugo_enabled: boolean;
  wechat_enabled: boolean;
  wechat_secret_saved: boolean;
  github_token_saved?: boolean;
  github_owner?: string;
  github_repository?: string;
  github_branch?: string;
  github_prefix?: string;
  default_template: string;
  templates: TemplateSummary[];
  diagnostics: { name: string; state: "正常" | "需要处理" | "未启用"; message: string }[];
}
