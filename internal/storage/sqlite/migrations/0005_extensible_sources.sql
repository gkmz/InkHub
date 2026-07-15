-- inkhub: foreign_keys_off
PRAGMA legacy_alter_table = ON;

ALTER TABLE sources RENAME TO sources_legacy;

CREATE TABLE sources (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  provider_type TEXT NOT NULL,
  root_path TEXT NOT NULL,
  config_json TEXT NOT NULL DEFAULT '{}',
  last_scan_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, provider_type),
  UNIQUE (id, workspace_id)
);

INSERT INTO sources(id,workspace_id,provider_type,root_path,config_json,last_scan_at,created_at,updated_at)
SELECT id,workspace_id,provider_type,root_path,config_json,last_scan_at,created_at,updated_at FROM sources_legacy;

DROP TABLE sources_legacy;
PRAGMA legacy_alter_table = OFF;
