-- inkhub: foreign_keys_off
PRAGMA legacy_alter_table = ON;

ALTER TABLE articles RENAME TO articles_legacy;

CREATE TABLE articles (
  id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id), source_id TEXT NOT NULL REFERENCES sources(id),
  stable_id TEXT NOT NULL, relative_path TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '', series TEXT NOT NULL DEFAULT '', tags_json TEXT NOT NULL DEFAULT '[]',
  keywords_json TEXT NOT NULL DEFAULT '[]', slug TEXT NOT NULL DEFAULT '', cover TEXT NOT NULL DEFAULT '',
  source_mtime TEXT, source_size INTEGER NOT NULL DEFAULT 0, source_fingerprint TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '', frontmatter_hash TEXT NOT NULL DEFAULT '', indexed_at TEXT NOT NULL,
  deleted_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  UNIQUE (source_id, relative_path), UNIQUE (id, workspace_id),
  FOREIGN KEY (source_id, workspace_id) REFERENCES sources(id, workspace_id)
);

INSERT INTO articles (
  id,workspace_id,source_id,stable_id,relative_path,title,description,category,series,tags_json,keywords_json,
  slug,cover,source_mtime,source_size,source_fingerprint,content_hash,frontmatter_hash,indexed_at,deleted_at,created_at,updated_at
)
SELECT
  id,workspace_id,source_id,stable_id,relative_path,title,description,category,series,tags_json,keywords_json,
  slug,cover,source_mtime,source_size,source_fingerprint,content_hash,frontmatter_hash,indexed_at,deleted_at,created_at,updated_at
FROM articles_legacy;

DROP TABLE articles_legacy;

CREATE INDEX idx_articles_workspace_state_path ON articles(workspace_id, deleted_at, relative_path);
CREATE INDEX idx_articles_content_hash ON articles(content_hash);
CREATE UNIQUE INDEX idx_articles_workspace_stable_id ON articles(workspace_id, stable_id) WHERE stable_id <> '';

PRAGMA legacy_alter_table = OFF;
