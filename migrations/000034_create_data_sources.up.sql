-- data_sources is the unified registry of upstream systems alertmesh either
-- pulls alerts from (Kafka / OpenSearch / K8s) or queries on demand
-- (Prometheus, used by AI tools and the operator-facing PromQL Explore page).
--
-- Two separate jsonb / text columns by design:
--
--   config     – non-secret, per-kind structured config (endpoints, topic,
--                index name, watched namespaces, K8s event-type checkboxes, …).
--                ALWAYS visible in list/get responses; this is what the UI
--                renders and what `/data-sources/{id}/test` re-uses.
--
--   secret_enc – AES-256-GCM ciphertext of a small JSON object holding the
--                actual secrets (token / password / sasl_password / api_key).
--                NEVER returned by GET; the API surface only echoes back a
--                {"masked": true} sentinel and a list of which secret keys
--                are populated, so the UI can render "leave blank to keep".
--
-- The split mirrors the LLM-provider pattern (config columns vs `api_key`
-- text), so encryption / wire-encryption flows can reuse the existing
-- DecodeClientCipher + cfgcrypto.Encrypt helpers.

CREATE TABLE IF NOT EXISTS data_sources (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(128) NOT NULL UNIQUE,
    kind         VARCHAR(32)  NOT NULL,
    description  TEXT         NOT NULL DEFAULT '',
    is_enabled   BOOLEAN      NOT NULL DEFAULT FALSE,
    is_default   BOOLEAN      NOT NULL DEFAULT FALSE,

    -- Public connection endpoint (URL / brokers / api server) duplicated out
    -- of `config` for fast listing + indexing.  Always non-secret.
    endpoint     TEXT         NOT NULL DEFAULT '',

    -- Per-kind public structured config (no secrets).  See model docstring
    -- for the schema of each kind.
    config       JSONB        NOT NULL DEFAULT '{}'::JSONB,

    -- AES-256-GCM ciphertext of {"token":"…","password":"…",…}.  Empty when
    -- the source needs no credentials (e.g. in-cluster K8s, anonymous Prom).
    secret_enc   TEXT         NOT NULL DEFAULT '',

    -- Last-test result, updated by the /test endpoint.  last_test_ok is a
    -- nullable bool so the UI can show "never tested" distinctly from
    -- "tested and failed".
    last_error   TEXT,
    last_test_at TIMESTAMPTZ,
    last_test_ok BOOLEAN,

    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Defensive: keep the kind column to a known set so a typo in the API can't
-- silently store an orphan row that no connector knows how to handle.
ALTER TABLE data_sources
    DROP CONSTRAINT IF EXISTS data_sources_kind_check;
ALTER TABLE data_sources
    ADD  CONSTRAINT data_sources_kind_check
    CHECK (kind IN ('prometheus', 'k8s', 'opensearch', 'kafka'));

CREATE INDEX IF NOT EXISTS idx_data_sources_kind       ON data_sources (kind);
CREATE INDEX IF NOT EXISTS idx_data_sources_is_enabled ON data_sources (is_enabled);

-- At most one default per kind (mirrors the llm_providers.is_default story
-- so the AI can resolve "the default Prometheus" without ambiguity).
CREATE UNIQUE INDEX IF NOT EXISTS uniq_data_sources_default_per_kind
    ON data_sources (kind) WHERE is_default;
