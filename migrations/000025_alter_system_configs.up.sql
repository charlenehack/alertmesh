-- Remove lifecycle timestamp columns from system_configs.
-- These rows are static system settings bootstrapped at startup; they do not
-- need audit timestamps.  A description column is added for self-documentation.
ALTER TABLE system_configs
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS deleted_at,
    ADD COLUMN IF NOT EXISTS description TEXT;
