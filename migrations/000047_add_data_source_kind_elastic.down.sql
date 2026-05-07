-- Reverting drops `elastic` from the allowed-kind set.  Any existing
-- elastic rows must be migrated to a different kind first or this
-- ALTER will fail; that is intentional (down-migrations should never
-- silently delete user data).

ALTER TABLE data_sources DROP CONSTRAINT IF EXISTS data_sources_kind_check;
ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_kind_check
    CHECK (kind IN ('prometheus', 'k8s', 'opensearch', 'kafka'));
