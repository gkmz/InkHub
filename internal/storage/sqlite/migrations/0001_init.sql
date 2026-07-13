CREATE TABLE schema_comments (
  object_type TEXT NOT NULL CHECK (object_type IN ('table','column')),
  object_name TEXT NOT NULL,
  comment TEXT NOT NULL,
  PRIMARY KEY (object_type, object_name)
);

CREATE TABLE workspaces (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, data_dir TEXT NOT NULL,
  last_used_at TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

CREATE TABLE sources (
  id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  provider_type TEXT NOT NULL CHECK (provider_type = 'obsidian'), root_path TEXT NOT NULL,
  config_json TEXT NOT NULL DEFAULT '{}', last_scan_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, provider_type), UNIQUE (id, workspace_id)
);

CREATE TABLE articles (
  id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id), source_id TEXT NOT NULL REFERENCES sources(id),
  stable_id TEXT NOT NULL, relative_path TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '', series TEXT NOT NULL DEFAULT '', tags_json TEXT NOT NULL DEFAULT '[]',
  keywords_json TEXT NOT NULL DEFAULT '[]', slug TEXT NOT NULL DEFAULT '', cover TEXT NOT NULL DEFAULT '',
  source_mtime TEXT, source_size INTEGER NOT NULL DEFAULT 0, source_fingerprint TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '', frontmatter_hash TEXT NOT NULL DEFAULT '', indexed_at TEXT NOT NULL,
  deleted_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  UNIQUE (source_id, relative_path), UNIQUE (id, workspace_id), UNIQUE (stable_id, workspace_id),
  FOREIGN KEY (source_id, workspace_id) REFERENCES sources(id, workspace_id)
);

CREATE TABLE editorial_reviews (
  article_id TEXT PRIMARY KEY REFERENCES articles(id),
  state TEXT NOT NULL CHECK (state IN ('draft','incomplete','pending_review','approved','changed','blocked')),
  approved_content_hash TEXT, approved_frontmatter_hash TEXT, approved_at TEXT, approved_by TEXT,
  blocking_count INTEGER NOT NULL DEFAULT 0, recommended_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL
);

CREATE TABLE taxonomy_terms (
  id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  kind TEXT NOT NULL CHECK (kind IN ('category','series','tag','alias')), name TEXT NOT NULL, canonical_name TEXT NOT NULL,
  usage_count INTEGER NOT NULL DEFAULT 0, is_core INTEGER NOT NULL DEFAULT 0, allow_low_frequency INTEGER NOT NULL DEFAULT 0,
  source_revision TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, kind, canonical_name), UNIQUE (id, workspace_id)
);

CREATE TABLE article_taxonomies (
  article_id TEXT NOT NULL REFERENCES articles(id), taxonomy_term_id TEXT NOT NULL REFERENCES taxonomy_terms(id),
  workspace_id TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY (article_id, taxonomy_term_id),
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  FOREIGN KEY (taxonomy_term_id, workspace_id) REFERENCES taxonomy_terms(id, workspace_id)
);

CREATE TABLE provider_instances (
  id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id), provider_type TEXT NOT NULL,
  name TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, config_json TEXT NOT NULL DEFAULT '{}',
  capabilities_json TEXT NOT NULL DEFAULT '[]', created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, provider_type), UNIQUE (id, workspace_id)
);

CREATE TABLE publications (
  id TEXT PRIMARY KEY, article_id TEXT NOT NULL REFERENCES articles(id),
  provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id), workspace_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('never','prepared','copied','confirmed','published','failed')),
  content_hash TEXT NOT NULL DEFAULT '', provider_revision TEXT NOT NULL DEFAULT '', last_error_code TEXT,
  last_error_message TEXT, last_processed_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  UNIQUE (article_id, provider_instance_id),
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id)
);

CREATE TABLE publication_events (
  id TEXT PRIMARY KEY, publication_id TEXT NOT NULL REFERENCES publications(id), event_type TEXT NOT NULL,
  content_hash TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL
);

CREATE TABLE ai_suggestions (
  id TEXT PRIMARY KEY, article_id TEXT NOT NULL REFERENCES articles(id), input_content_hash TEXT NOT NULL,
  provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id), workspace_id TEXT NOT NULL,
  suggestion_json TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('pending','partially_accepted','accepted','rejected','expired','invalid')),
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id)
);

CREATE TABLE templates (
  id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id), template_id TEXT NOT NULL,
  version TEXT NOT NULL, source TEXT NOT NULL, manifest_json TEXT NOT NULL, install_path TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, template_id, version)
);

CREATE TABLE jobs (
  id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id), kind TEXT NOT NULL, dedupe_key TEXT,
  state TEXT NOT NULL CHECK (state IN ('queued','running','succeeded','failed','cancelled')),
  progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100), payload_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}', error_code TEXT, error_message TEXT, attempts INTEGER NOT NULL DEFAULT 0,
  available_at TEXT NOT NULL, started_at TEXT, finished_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

CREATE TABLE settings (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id), key TEXT NOT NULL, value_json TEXT NOT NULL,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY (workspace_id, key)
);

CREATE INDEX idx_articles_workspace_state_path ON articles(workspace_id, deleted_at, relative_path);
CREATE INDEX idx_articles_content_hash ON articles(content_hash);
CREATE INDEX idx_publications_article_state ON publications(article_id, state);
CREATE INDEX idx_publication_events_publication_time ON publication_events(publication_id, created_at);
CREATE INDEX idx_jobs_runnable ON jobs(state, available_at, created_at);
CREATE UNIQUE INDEX idx_jobs_active_dedupe ON jobs(workspace_id, dedupe_key)
  WHERE dedupe_key IS NOT NULL AND state IN ('queued','running');
