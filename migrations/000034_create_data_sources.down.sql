DROP INDEX IF EXISTS uniq_data_sources_default_per_kind;
DROP INDEX IF EXISTS idx_data_sources_is_enabled;
DROP INDEX IF EXISTS idx_data_sources_kind;
ALTER TABLE IF EXISTS data_sources DROP CONSTRAINT IF EXISTS data_sources_kind_check;
DROP TABLE IF EXISTS data_sources;
