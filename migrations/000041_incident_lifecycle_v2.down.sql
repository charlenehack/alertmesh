DROP INDEX IF EXISTS idx_incidents_parent;
DROP INDEX IF EXISTS idx_incidents_status_last_alert;

ALTER TABLE incidents
    DROP COLUMN IF EXISTS last_notified_at,
    DROP COLUMN IF EXISTS notification_count,
    DROP COLUMN IF EXISTS auto_resolved_at,
    DROP COLUMN IF EXISTS parent_incident_id,
    DROP COLUMN IF EXISTS last_alert_at;
