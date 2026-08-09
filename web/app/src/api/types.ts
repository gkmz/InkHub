export type ArticleState = "draft" | "blocked" | "changed" | "incomplete" | "pending_review" | "approved";
export type ContentStage = "draft" | "ready";
export type ArticleDisposition = "published" | "ignored";
export type PublicationChannel = "hugo" | "wechat" | "xiaohongshu";
export type PublicationDisplayState = "blocked" | "not_configured" | "ready" | "running" | "completed" | "failed" | "stale";

export interface PublicationChannelSummary {
  channel: PublicationChannel;
  label: string;
  state: PublicationDisplayState;
  rawState: string;
  actionLabel: string;
}

export interface WorkspaceSummary {
  id: string;
  name: string;
}

export interface SessionResponse {
  has_workspace: boolean;
  workspace: WorkspaceSummary | null;
  initialization?: { required: boolean; job_id: string; state: "running" | "succeeded" | "failed" };
}

export interface ArticleSummary {
  id: string;
  title: string;
  filename?: string;
  directory: string;
  category: string;
  modified_at: string;
  state: ArticleState;
  hugo_state: string;
  wechat_state: string;
  xiaohongshu_state?: string;
  content_version: string;
  disposition?: ArticleDisposition;
  content_stage: ContentStage;
  content_stage_issue?: string;
  next_action?: "retry" | "review" | "publish" | "view";
}

export interface ArticlePage {
  items: ArticleSummary[];
  next_cursor?: string;
  available_channels: PublicationChannel[];
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

export interface HugoSiteInspection {
  root: string;
  content_dir: string;
  sections: Array<{ name: string; markdown_count: number }>;
}

export interface JobStatus {
  id: string;
  state: "queued" | "running" | "succeeded" | "failed";
  progress: number;
  indexed?: number;
  assigned_ids?: number;
  failed?: number;
  issues?: Array<{ article_path: string; code: string; message: string }>;
  error_message?: string;
}

export interface WorkspaceInitializationResult {
  indexed: number;
  assigned_ids: number;
  failed: number;
  issues: Array<{ article_path: string; code: string; message: string }>;
}

export interface HugoSectionView {
  sections: Array<{ name: string; article_count: number; directories?: Array<{ path: string; article_count: number }> }>;
  existing_section: string;
  existing_directory?: string;
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
  render_url?: string;
  expires_at?: string;
  state: "preparing" | "ready" | "expired" | "failed";
  job_id: string;
  error?: string;
  failure?: PublicationFailureView;
}

export interface PublicationFailureView {
  stage: "preflight" | "prepare" | "deliver" | string;
  code: string;
  message: string;
  action: string;
  retryable: boolean;
}

export interface RecoveredHugoPreviewView {
  preview_id: string;
  section: string;
  target_path: string;
  change: "added" | "updated";
  files: Array<{ relative_path: string; media_type: string; size: number }>;
  diagnostics: Array<{ code: string; level: string; message: string }>;
  preview_url?: string;
  render_url?: string;
  expires_at?: string;
  state: "preparing" | "ready" | "expired" | "failed";
  error?: string;
  failure?: PublicationFailureView;
}

export interface PublicationWorkflowView {
  article_id: string;
  hugo: null | {
    state: "preparing" | "ready" | "expired" | "failed" | "delivering" | "published";
    progress: number;
    stage: string;
    error?: string;
    failure?: PublicationFailureView;
    preview?: RecoveredHugoPreviewView;
    delivery?: { state: string; progress: number; stage: string; error?: string; failure?: PublicationFailureView };
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
  mermaid_theme: MermaidTheme;
  images: Array<{ reference: string; media_type: string; size: number; state: "upload" | "reuse" }>;
  diagnostics: Array<{ code: string; message: string; blocking: boolean }>;
  ready: boolean;
  expires_at: string;
}

export type MermaidTheme = "handdrawn" | "modern";

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
  value?: string | string[];
  reason: string;
  new_term: boolean;
  usage_count: number;
  accepted?: boolean;
  ignored?: boolean;
  status?: "pending" | "accepted" | "ignored";
}

export interface SuggestionHistoryItem {
  id: string;
  generated_at: string;
  model: string;
  input_content_hash: string;
  state: string;
  suggestion_count: number;
  current: boolean;
}

export interface SuggestionHistoryResponse {
  items: SuggestionHistoryItem[];
  latest_id?: string;
}

export interface SuggestionVersionView {
  id: string;
  generated_at: string;
  model: string;
  input_content_hash: string;
  state: string;
  suggestions: AISuggestion[];
  suggestions_stale: boolean;
}

export interface ArticleDetail {
  id: string;
  stable_id: string;
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
  xiaohongshu_enabled?: boolean;
  xiaohongshu_state?: string;
  checks: CheckResult[];
  ai_configured: boolean;
  suggestions: AISuggestion[];
  suggestions_id?: string;
  suggestions_generated_at?: string;
  suggestions_stale: boolean;
  wechat_copied: boolean;
  resource_diagnostics: Array<{ code: string; message: string; blocking: boolean }>;
  disposition?: { kind: ArticleDisposition; channels: PublicationChannel[] };
}

export interface XiaohongshuDraft {
  id: string;
  article_id: string;
  source_content_hash: string;
  mode: XiaohongshuDraftMode;
  title: string;
  body_html: string;
  pages: XiaohongshuPage[];
  script_pages: XiaohongshuScriptPage[];
  topics: string;
  source_note: string;
  comment_copy: string;
  ai_model: string;
  prompt_version: string;
  state: "draft" | "published" | "stale";
  stale: boolean;
  created_at: string;
  updated_at: string;
}

export type XiaohongshuDraftMode = "long_card" | "visual_script";

export interface XiaohongshuScriptPage {
  id: string;
  title: string;
  prompt: string;
}

export interface XiaohongshuKnowledgePoint {
  id: string;
  kind: "claim" | "fact" | "step" | "warning" | "example" | "conclusion";
  summary: string;
  source_evidence: string;
}

export interface XiaohongshuRewriteOutline {
  content_hash: string;
  knowledge_points: XiaohongshuKnowledgePoint[];
}

export type XiaohongshuBlockKind = "paragraph" | "heading" | "image" | "code" | "table" | "text";

export interface XiaohongshuBlock {
  id: string;
  kind: XiaohongshuBlockKind;
  html: string;
  splittable: boolean;
}

export interface XiaohongshuPage {
  id: string;
  blocks: XiaohongshuBlock[];
  measured_height: number;
}

export interface XiaohongshuView {
  article_id: string;
  current_content_hash: string;
  template_id: string;
  mode: XiaohongshuDraftMode;
  state: string;
  latest: XiaohongshuDraft | null;
  history: XiaohongshuDraft[];
  diagnostics: { code: string; message: string; blocking: boolean }[];
}

export interface ObsidianSettingsView {
  attachment_location: string;
  attachment_path: string;
  link_format: string;
  use_markdown_links: boolean;
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
  excluded_sections: string[];
  ai_enabled: boolean;
  ai_secret_saved: boolean;
  ai_base_url?: string;
  ai_model?: string;
  hugo_enabled: boolean;
  hugo_path?: string;
  hugo_base_url?: string;
  hugo_valid?: boolean;
  hugo_bundle_count?: number;
  hugo_linked_count?: number;
  hugo_unlinked_count?: number;
  hugo_conflict_count?: number;
  wechat_enabled: boolean;
  wechat_secret_saved: boolean;
  github_token_saved?: boolean;
  github_owner?: string;
  github_repository?: string;
  github_branch?: string;
  github_prefix?: string;
  default_template: string;
  templates: TemplateSummary[];
  xiaohongshu_enabled: boolean;
  xiaohongshu_template: string;
  xiaohongshu_templates: TemplateSummary[];
  obsidian_settings?: ObsidianSettingsView;
  diagnostics: { name: string; state: "正常" | "需要处理" | "未启用"; message: string }[];
}

export interface HugoTakeoverCandidate {
  bundle_path: string;
  article_path?: string;
  title: string;
  stable_id?: string;
  status: "matched" | "conflict" | "unmatched";
  match_reason?: string;
}

export interface HugoTakeoverReport {
  bundle_count: number;
  linked_count: number;
  matched_count: number;
  conflict_count: number;
  unmatched_count: number;
  articles_missing_id: number;
  source_issue_count: number;
  source_issues: { article_path: string; code: string; message: string }[];
  candidates: HugoTakeoverCandidate[];
}
