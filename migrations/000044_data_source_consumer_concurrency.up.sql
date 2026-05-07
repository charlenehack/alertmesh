-- Per-data-source consumer concurrency: backfill `consumer_concurrency: 1`
-- for every existing kafka / opensearch row so the new KafkaManager (and the
-- forthcoming Phase-4 OpenSearch poller) can read the key unconditionally.
--
-- 1 = previous behaviour (one Reader / one goroutine per row); the UI lets
-- operators raise this up to 8 when the topic's partition count warrants it.
-- COALESCE keeps any operator-customised value intact, so re-running this
-- migration is idempotent.

UPDATE data_sources
SET config = config
   || jsonb_build_object(
        'consumer_concurrency',
        COALESCE((config->>'consumer_concurrency')::int, 1)
      )
WHERE kind IN ('kafka', 'opensearch');
