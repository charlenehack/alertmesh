CREATE TABLE IF NOT EXISTS ai_tasks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID         NOT NULL REFERENCES incidents(id),
    status      VARCHAR(20)  NOT NULL DEFAULT 'pending',
    priority    INTEGER      NOT NULL DEFAULT 0,
    error       TEXT,
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_ai_tasks_incident ON ai_tasks (incident_id);
