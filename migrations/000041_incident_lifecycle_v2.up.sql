-- Incident lifecycle v2: progressive repeat + auto resolve + reopen window.
--
-- Adds the bookkeeping columns the new flow needs.  Backwards-compatible:
-- every column is nullable (or has a sensible default) and existing
-- incidents are backfilled with last_alert_at = opened_at so the staleness
-- reaper does not immediately auto-resolve them on first scan after deploy.
ALTER TABLE incidents
    ADD COLUMN IF NOT EXISTS last_alert_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS parent_incident_id UUID REFERENCES incidents(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS auto_resolved_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS notification_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_notified_at   TIMESTAMPTZ;

-- Partial index used by the staleness reaper hot path; only open/ack rows
-- are candidates so the index stays small even with a long incident history.
CREATE INDEX IF NOT EXISTS idx_incidents_status_last_alert
    ON incidents (status, last_alert_at)
    WHERE status IN ('open', 'ack', 'in_progress');

-- Used by the UI to render the "延续自 #xxx" link on incidents that were
-- created after a parent's reopen window had closed.
CREATE INDEX IF NOT EXISTS idx_incidents_parent
    ON incidents (parent_incident_id)
    WHERE parent_incident_id IS NOT NULL;

-- Backfill so the reaper does not see NULL < (now - timeout) for legacy rows.
UPDATE incidents SET last_alert_at = opened_at WHERE last_alert_at IS NULL;
