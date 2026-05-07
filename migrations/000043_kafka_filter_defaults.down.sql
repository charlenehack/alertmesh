-- Strip the two new keys we backfilled in the up migration.  Any value an
-- operator hand-edited under `filter` / `mapping` is gone after rollback —
-- they live only in the live config jsonb, no audit table.

UPDATE data_sources
SET config = (config - 'filter') - 'mapping'
WHERE kind = 'kafka';
