CREATE TABLE xiaohongshu_drafts (
  id TEXT PRIMARY KEY,
  article_id TEXT NOT NULL REFERENCES articles(id),
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  source_content_hash TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  body_html TEXT NOT NULL DEFAULT '',
  topics_json TEXT NOT NULL DEFAULT '[]',
  source_note TEXT NOT NULL DEFAULT '',
  comment_copy TEXT NOT NULL DEFAULT '',
  ai_model TEXT NOT NULL DEFAULT '',
  prompt_version TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('draft','published','stale')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  UNIQUE (id, article_id)
);

CREATE INDEX idx_xhs_drafts_article_time ON xiaohongshu_drafts(article_id, created_at DESC, id DESC);

CREATE TABLE xiaohongshu_renders (
  id TEXT PRIMARY KEY,
  draft_id TEXT NOT NULL REFERENCES xiaohongshu_drafts(id),
  article_id TEXT NOT NULL REFERENCES articles(id),
  template_id TEXT NOT NULL,
  template_version TEXT NOT NULL,
  viewport_width INTEGER NOT NULL,
  page_height INTEGER NOT NULL,
  html_hash TEXT NOT NULL,
  page_count INTEGER NOT NULL,
  state TEXT NOT NULL DEFAULT 'ready',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (draft_id, article_id) REFERENCES xiaohongshu_drafts(id, article_id)
);

CREATE TABLE xiaohongshu_events (
  id TEXT PRIMARY KEY,
  draft_id TEXT NOT NULL REFERENCES xiaohongshu_drafts(id),
  render_id TEXT REFERENCES xiaohongshu_renders(id),
  event_type TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX idx_xhs_events_draft_time ON xiaohongshu_events(draft_id, created_at DESC, id DESC);
