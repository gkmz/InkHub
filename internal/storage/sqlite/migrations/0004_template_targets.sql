ALTER TABLE templates ADD COLUMN target TEXT NOT NULL DEFAULT 'wechat-html';
ALTER TABLE templates ADD COLUMN format TEXT NOT NULL DEFAULT 'css';
ALTER TABLE templates ADD COLUMN renderer TEXT NOT NULL DEFAULT 'wechat-html-v1';
UPDATE templates SET manifest_json=json_set(manifest_json,'$.target',target,'$.format',format,'$.renderer',renderer);
CREATE INDEX idx_templates_workspace_target_enabled ON templates(workspace_id,target,enabled);
