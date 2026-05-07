CREATE TABLE IF NOT EXISTS ai_conversations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID         NOT NULL,
    role        VARCHAR(20)  NOT NULL,
    content     TEXT         NOT NULL,
    user_id     VARCHAR(255),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_ai_conv_incident ON ai_conversations (incident_id);
