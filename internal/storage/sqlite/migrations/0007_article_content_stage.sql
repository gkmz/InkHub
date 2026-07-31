ALTER TABLE articles ADD COLUMN content_stage TEXT NOT NULL DEFAULT 'draft' CHECK (content_stage IN ('draft','ready'));
ALTER TABLE articles ADD COLUMN content_stage_issue TEXT NOT NULL DEFAULT '';
