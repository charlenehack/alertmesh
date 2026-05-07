-- JSON mapping from signed webhook JSON body → RawAlert (gjson paths).
-- See docs/log-alert-denoising.md §3 and ingestion.WebhookMapping.
ALTER TABLE webhook_sources
    ADD COLUMN IF NOT EXISTS mapping JSONB NOT NULL DEFAULT '{}';
