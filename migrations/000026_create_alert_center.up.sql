-- Silence policies
CREATE TABLE IF NOT EXISTS silence_policies (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    comment     TEXT,
    matchers    JSONB        NOT NULL,
    starts_at   TIMESTAMPTZ  NOT NULL,
    ends_at     TIMESTAMPTZ  NOT NULL,
    created_by  VARCHAR(255) NOT NULL,
    is_active   BOOLEAN      NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);

-- Notification message templates
CREATE TABLE IF NOT EXISTS notification_templates (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(255) NOT NULL,
    channel_type VARCHAR(32)  NOT NULL,
    subject      VARCHAR(512),
    body         TEXT         NOT NULL,
    is_default   BOOLEAN      NOT NULL DEFAULT false,
    description  TEXT,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    UNIQUE (name)
);

-- Aggregation policies
CREATE TABLE IF NOT EXISTS aggregation_policies (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    matchers        JSONB        NOT NULL,
    group_by        JSONB,
    group_wait      INTEGER      NOT NULL DEFAULT 30,
    group_interval  INTEGER      NOT NULL DEFAULT 300,
    repeat_interval INTEGER      NOT NULL DEFAULT 3600,
    is_enabled      BOOLEAN      NOT NULL DEFAULT true,
    description     TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);
