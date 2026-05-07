CREATE TABLE IF NOT EXISTS incidents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT         NOT NULL,
    severity     VARCHAR(10)  NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'open',
    source       VARCHAR(64)  NOT NULL,
    labels       JSONB,
    group_key    VARCHAR(255),
    assignee_id  UUID,
    opened_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    acked_at     TIMESTAMPTZ,
    resolved_at  TIMESTAMPTZ,
    ai_status    VARCHAR(20)  NOT NULL DEFAULT 'pending',
    ai_report_id UUID,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_incidents_severity  ON incidents (severity);
CREATE INDEX IF NOT EXISTS idx_incidents_status    ON incidents (status);
CREATE INDEX IF NOT EXISTS idx_incidents_group_key ON incidents (group_key);
