-- inkhub: foreign_keys_off
ALTER TABLE article_taxonomies RENAME TO article_taxonomies_legacy;
ALTER TABLE taxonomy_terms RENAME TO taxonomy_terms_legacy;

CREATE TABLE taxonomy_terms (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  provider_instance_id TEXT,
  kind TEXT NOT NULL,
  external_key TEXT NOT NULL,
  name TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  usage_count INTEGER NOT NULL DEFAULT 0,
  source_revision TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  UNIQUE (provider_instance_id, kind, external_key),
  UNIQUE (id, workspace_id),
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id),
  CHECK (provider_instance_id IS NOT NULL OR source_revision = 'legacy')
);

INSERT INTO taxonomy_terms(
  id,workspace_id,provider_instance_id,kind,external_key,name,canonical_name,
  metadata_json,usage_count,source_revision,updated_at
)
SELECT
  legacy.id,legacy.workspace_id,
  (SELECT provider.id FROM provider_instances provider
   WHERE provider.workspace_id=legacy.workspace_id AND provider.provider_type='hugo'
   ORDER BY provider.id LIMIT 1),
  legacy.kind,legacy.canonical_name,legacy.name,legacy.canonical_name,
  json_object('is_core',legacy.is_core,'allow_low_frequency',legacy.allow_low_frequency),
  legacy.usage_count,
  CASE WHEN EXISTS(SELECT 1 FROM provider_instances provider WHERE provider.workspace_id=legacy.workspace_id AND provider.provider_type='hugo')
       THEN legacy.source_revision ELSE 'legacy' END,
  legacy.updated_at
FROM taxonomy_terms_legacy legacy;

CREATE TABLE article_taxonomies (
  article_id TEXT NOT NULL REFERENCES articles(id),
  taxonomy_term_id TEXT NOT NULL REFERENCES taxonomy_terms(id),
  workspace_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (article_id, taxonomy_term_id),
  FOREIGN KEY (article_id, workspace_id) REFERENCES articles(id, workspace_id),
  FOREIGN KEY (taxonomy_term_id, workspace_id) REFERENCES taxonomy_terms(id, workspace_id)
);

INSERT INTO article_taxonomies(article_id,taxonomy_term_id,workspace_id,created_at)
SELECT article_id,taxonomy_term_id,workspace_id,created_at FROM article_taxonomies_legacy;

DROP TABLE article_taxonomies_legacy;
DROP TABLE taxonomy_terms_legacy;

CREATE TABLE taxonomy_snapshots (
  provider_instance_id TEXT PRIMARY KEY REFERENCES provider_instances(id),
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  revision TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('ready','refreshing','failed')),
  complete INTEGER NOT NULL DEFAULT 0,
  last_error_code TEXT,
  last_error_message TEXT,
  last_attempt_at TEXT NOT NULL,
  last_success_at TEXT,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (provider_instance_id, workspace_id) REFERENCES provider_instances(id, workspace_id)
);

CREATE INDEX idx_taxonomy_terms_provider_kind ON taxonomy_terms(provider_instance_id,kind,name);
CREATE INDEX idx_taxonomy_snapshots_workspace_state ON taxonomy_snapshots(workspace_id,state);
