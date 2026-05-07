ALTER TABLE data_sources DROP CONSTRAINT IF EXISTS data_sources_ai_auto_trigger_chk;
ALTER TABLE data_sources DROP COLUMN IF EXISTS ai_auto_trigger;
