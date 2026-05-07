DROP INDEX IF EXISTS idx_data_sources_deleted_at;
ALTER TABLE data_sources DROP COLUMN IF EXISTS deleted_at;
