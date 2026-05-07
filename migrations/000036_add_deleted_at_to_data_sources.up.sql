-- data_sources was the only table created without GORM's soft-delete column,
-- causing every Create / Save to fail with:
--   ERROR: column "deleted_at" of relation "data_sources" does not exist (42703)
-- because internal/model/data_source.go embeds Timestamps (which carries
-- gorm.DeletedAt) just like every other UUID-keyed model in the codebase.
ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_data_sources_deleted_at
    ON data_sources (deleted_at);
