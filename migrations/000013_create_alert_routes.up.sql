CREATE TABLE IF NOT EXISTS alert_routes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    priority    INTEGER      NOT NULL DEFAULT 0,
    matchers    JSONB        NOT NULL,
    group_by    JSONB,
    channel_ids JSONB,
    is_enabled  BOOLEAN      NOT NULL DEFAULT true,
    description TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
