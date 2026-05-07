CREATE TABLE IF NOT EXISTS notification_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id  UUID        NOT NULL,
    channel_id   UUID        NOT NULL,
    channel_type VARCHAR(32) NOT NULL,
    status       VARCHAR(20) NOT NULL,
    error        TEXT,
    sent_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notif_log_incident ON notification_log (incident_id);
