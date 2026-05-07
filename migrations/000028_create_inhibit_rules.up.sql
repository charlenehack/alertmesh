-- Inhibit rules: source alert silences target alerts when active.
CREATE TABLE IF NOT EXISTS inhibit_rules (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    source_matchers JSONB        NOT NULL,
    target_matchers JSONB        NOT NULL,
    equal           JSONB        NOT NULL DEFAULT '[]',
    is_enabled      BOOLEAN      NOT NULL DEFAULT true,
    description     TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);
