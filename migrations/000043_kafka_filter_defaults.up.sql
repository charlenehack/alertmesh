-- Backfill / forward-compat: ensure every existing Kafka data source row has
-- the two new keys (`filter` + `mapping`) inside its config jsonb so the new
-- consumer can rely on them being present without per-row null guards.
--
-- `filter`  : empty string = "let everything through" (matches our default).
-- `mapping` : default Prometheus-shaped paths so a vanilla
--             `{"alertname":"X","severity":"P3","summary":"…"}` payload still
--             ingests without anyone touching the UI.
--
-- All upserts are idempotent — a second `migrate up` is a no-op because
-- COALESCE(config->'mapping', …) keeps any operator-customised mapping intact.

UPDATE data_sources
SET config = config
   || jsonb_build_object(
        'filter', COALESCE(config->>'filter', ''),
        'mapping', COALESCE(
            config->'mapping',
            jsonb_build_object(
              'alertname',   'alertname',
              'severity',    'severity',
              'summary',     'summary',
              'description', 'description',
              'starts_at',   'starts_at',
              'fingerprint', '',
              'status_path', '',
              'resolved_when','',
              'labels',      jsonb_build_object(),
              'annotations', jsonb_build_object()
            )
        )
   )
WHERE kind = 'kafka';
