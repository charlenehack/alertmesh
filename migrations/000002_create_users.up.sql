CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(255) NOT NULL UNIQUE,
    email         VARCHAR(255) UNIQUE,
    display_name  VARCHAR(255),
    password_hash VARCHAR(255),
    source        VARCHAR(32)  NOT NULL DEFAULT 'local',
    external_id   VARCHAR(255),
    is_active     BOOLEAN      NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_users_external_id ON users (external_id);
