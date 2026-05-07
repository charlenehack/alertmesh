ALTER TABLE data_sources
    DROP CONSTRAINT IF EXISTS data_sources_ai_enabled_kind_chk;
ALTER TABLE data_sources
    DROP COLUMN IF EXISTS ai_enabled;
