package label

// Data source identities (Prometheus / Kafka / OpenSearch / K8s registry).
//
// All endpoints are admin-only — these rows wire alertmesh up to upstream
// systems with credentials, so handing them to a regular role would let
// any operator pivot into the cluster's metrics / logs / Kafka.  Mirrors
// the LLMProvider* identity bucket (admin-only by convention).
//
// `DataSourceQuery` is split out from the CRUD identities because the
// PromQL Explore page (read-only graph rendering for any logged-in user
// who already has dashboard access) wants to grant the proxy without
// granting CRUD on the registry itself.

const (
	DataSourceModuleName = "数据源"

	DataSourceList    = "dataSourceList"
	DataSourceCreate  = "dataSourceCreate"
	DataSourceUpdate  = "dataSourceUpdate"
	DataSourceDelete  = "dataSourceDelete"
	DataSourceTest    = "dataSourceTest"
	DataSourceDefault = "dataSourceDefault"

	// Prometheus query proxy — read-only data plane, separate identity so
	// it can be granted to non-admin viewers later without re-architecting.
	DataSourceQuery = "dataSourceQuery"
)
