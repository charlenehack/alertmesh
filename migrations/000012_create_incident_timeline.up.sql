CREATE TABLE IF NOT EXISTS incident_timeline (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID         NOT NULL REFERENCES incidents(id),
    action      VARCHAR(64)  NOT NULL,
    from_status VARCHAR(20),
    to_status   VARCHAR(20),
    user_id     VARCHAR(255),
    username    VARCHAR(255),
    message     TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_timeline_incident_id ON incident_timeline (incident_id);
