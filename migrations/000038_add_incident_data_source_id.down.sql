DROP INDEX IF EXISTS idx_incidents_data_source;
ALTER TABLE incidents
    DROP COLUMN IF EXISTS data_source_id;
