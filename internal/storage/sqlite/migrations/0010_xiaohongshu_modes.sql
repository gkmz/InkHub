ALTER TABLE xiaohongshu_drafts ADD COLUMN mode TEXT NOT NULL DEFAULT 'long_card' CHECK(mode IN ('long_card','visual_script'));
ALTER TABLE xiaohongshu_drafts ADD COLUMN script_pages_json TEXT NOT NULL DEFAULT '[]';

CREATE INDEX idx_xhs_drafts_article_mode_time ON xiaohongshu_drafts(article_id, mode, created_at DESC, id DESC);
