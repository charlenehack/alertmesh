CREATE TABLE IF NOT EXISTS ai_analyses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL REFERENCES incidents(id),
    task_id     UUID NOT NULL REFERENCES ai_tasks(id),
    report      TEXT NOT NULL,
    summary     TEXT,
    root_cause  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_ai_analyses_incident ON ai_analyses (incident_id);
