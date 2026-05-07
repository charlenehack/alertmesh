-- Register `elastic` as a first-class data source kind.
--
-- Elasticsearch and OpenSearch share the same HTTP query DSL and Basic-Auth
-- credential shape, so the runtime simply reuses the existing OpenSearch
-- code path (see internal/router/data_sources.go and internal/ai/eligibility.go,
-- where every OpenSearch case is extended to handle Elastic in parallel).
--
-- Splitting the kind keeps the UI honest (operators wire up an "Elastic"
-- data source to an Elasticsearch cluster) and leaves room for a dedicated
-- Elastic client in the future without a second schema migration.

ALTER TABLE data_sources DROP CONSTRAINT IF EXISTS data_sources_kind_check;
ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_kind_check
    CHECK (kind IN ('prometheus', 'k8s', 'opensearch', 'kafka', 'elastic'));
