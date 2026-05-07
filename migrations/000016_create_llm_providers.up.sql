CREATE TABLE IF NOT EXISTS llm_providers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255)  NOT NULL,
    provider    VARCHAR(32)   NOT NULL,
    base_url    VARCHAR(512),
    api_key     TEXT          NOT NULL,
    model       VARCHAR(255)  NOT NULL,
    temperature REAL          NOT NULL DEFAULT 0.1,
    is_default  BOOLEAN       NOT NULL DEFAULT false,
    is_enabled  BOOLEAN       NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
