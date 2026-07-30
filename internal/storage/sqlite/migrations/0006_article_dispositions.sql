CREATE TABLE article_dispositions (
  article_id TEXT PRIMARY KEY REFERENCES articles(id),
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  kind TEXT NOT NULL CHECK (kind IN ('published','ignored')),
  content_hash TEXT NOT NULL DEFAULT '',
  cleared_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id)
);

CREATE INDEX idx_article_dispositions_workspace_kind
  ON article_dispositions(workspace_id, kind, cleared_at, updated_at);
