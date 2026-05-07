-- Reverse the consumer_concurrency backfill.  Operators that explicitly raised
-- the value lose it on rollback; that's expected — `down` puts the schema back
-- to what code <000044 understood.
UPDATE data_sources
SET config = config - 'consumer_concurrency'
WHERE kind IN ('kafka', 'opensearch');
