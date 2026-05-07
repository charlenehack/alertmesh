CREATE TABLE IF NOT EXISTS audit_logs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    VARCHAR(255),
    username   VARCHAR(255),
    action     VARCHAR(255) NOT NULL,
    resource   VARCHAR(255) NOT NULL,
    detail     JSONB,
    ip         VARCHAR(45),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
