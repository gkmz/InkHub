ALTER TABLE articles ADD COLUMN body_hash TEXT DEFAULT '';
ALTER TABLE editorial_reviews ADD COLUMN approved_body_hash TEXT DEFAULT '';

-- 旧版本没有独立正文 hash，保留旧内容版本作为迁移期间的兼容基线。
UPDATE articles SET body_hash=content_hash WHERE body_hash='';
UPDATE editorial_reviews SET approved_body_hash=approved_content_hash WHERE approved_body_hash='';
