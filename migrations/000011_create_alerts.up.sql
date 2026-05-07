CREATE TABLE IF NOT EXISTS alerts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID         NOT NULL REFERENCES incidents(id),
    source      VARCHAR(64)  NOT NULL,
    fingerprint VARCHAR(255) NOT NULL,
    labels      JSONB,
    annotations JSONB,
    starts_at   TIMESTAMPTZ,
    ends_at     TIMESTAMPTZ,
    status      VARCHAR(20)  NOT NULL,
    raw_payload JSONB,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alerts_incident_id ON alerts (incident_id);
CREATE INDEX IF NOT EXISTS idx_alerts_fingerprint  ON alerts (fingerprint);
