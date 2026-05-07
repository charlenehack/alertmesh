-- Trusted webhook sources for /api/v1/alerts/webhook/{source}.
--
-- Each row represents one external alert source (e.g. a third-party script,
-- cloud-monitor adapter) that posts alerts via signed HTTP requests using
-- HTTP Message Signatures (RFC 9421) with an Ed25519 keypair.
--
-- The PRIVATE key is generated server-side, returned to the user ONCE on
-- create / rotate, and is never persisted; only the PEM-encoded PKIX public
-- key is stored here. `client_id` is the RFC 9421 keyid clients must use.
CREATE TABLE IF NOT EXISTS webhook_sources (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(255) NOT NULL UNIQUE,        -- == path {source}
    client_id    VARCHAR(64)  NOT NULL UNIQUE,        -- RFC 9421 keyid (auto "ws_<16hex>")
    public_key   TEXT         NOT NULL,               -- PEM Ed25519 PKIX
    allow_skew   INT          NOT NULL DEFAULT 300,   -- created ±N seconds
    is_enabled   BOOLEAN      NOT NULL DEFAULT true,
    description  TEXT,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webhook_sources_name      ON webhook_sources (name);
CREATE INDEX IF NOT EXISTS idx_webhook_sources_client_id ON webhook_sources (client_id);
CREATE INDEX IF NOT EXISTS idx_webhook_sources_deleted_at ON webhook_sources (deleted_at);
