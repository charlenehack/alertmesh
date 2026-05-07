-- Escalation policies: bump incident severity when not acknowledged within ack_timeout seconds.
CREATE TABLE IF NOT EXISTS escalation_policies (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(255) NOT NULL,
    from_severity VARCHAR(32)  NOT NULL,
    to_severity   VARCHAR(32)  NOT NULL,
    ack_timeout   INTEGER      NOT NULL,
    is_enabled    BOOLEAN      NOT NULL DEFAULT true,
    description   TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);
