package router

// CRUD + management + Prometheus query proxy for /api/v1/data-sources.
//
// Scope:
//
//   - This handler manages the registry rows that tell alertmesh how to
//     connect to upstream systems.  The actual consumer/connector loops
//     for Kafka / OpenSearch / K8s land in Phase 3 (per the README §4.1.4
//     roadmap) and will use the official Go SDKs (`segmentio/kafka-go`,
//     `opensearch-project/opensearch-go`, `client-go`).
//   - The Prometheus integration is fully functional today: the registry
//     row stores the URL + optional auth, and this handler exposes a
//     server-side proxy so the operator-facing PromQL Explore page can
//     render graphs without leaking credentials to the browser, and the
//     AI agent can query metrics through the same code path.
//
// Secret handling mirrors `llm_providers.go`:
//
//   - Wire transport: the browser RSA-encrypts each secret with the
//     system public key and sends it prefixed `ENC:`; the server peels
//     the prefix via `auth.DecodeClientCipher`.
//   - At rest: secrets are bundled into a small JSON object and
//     AES-256-GCM encrypted with the platform's master key
//     (`config.EncryptionKey`).  The plaintext NEVER leaves the server.
//   - List/get responses always return an empty `secret` map plus a
//     `secret_keys` array telling the UI which keys are populated, so
//     "leave blank to keep" works exactly like the LLM provider page.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/auth"
	"github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/httputil"
	"github.com/kuzane/alertmesh/internal/ingestion"
	"github.com/kuzane/alertmesh/internal/label"
	"github.com/kuzane/alertmesh/internal/model"
)

// allowedDataSourceKinds is the closed enum mirrored by migration 34's
// CHECK constraint.  Adding a new kind = adding it here AND in the
// `data_sources_kind_check` constraint.
var allowedDataSourceKinds = map[string]struct{}{
	model.DataSourceKindPrometheus: {},
	model.DataSourceKindK8s:        {},
	model.DataSourceKindOpenSearch: {},
	model.DataSourceKindKafka:      {},
	// Elasticsearch shares the OpenSearch HTTP query DSL + Basic-Auth
	// shape; every kind switch below is extended with a parallel case
	// so the runtime treats the two identically.  Surfaced as a separate
	// kind so the UI doesn't lie about which cluster operators wired up.
	model.DataSourceKindElastic: {},
}

// allowedK8sEvents is the closed enum of selectable K8s detectors.  These
// strings end up in `Config.events[]` and are what the future k8s
// connector (Phase 3) reads to decide which informers to wire up.
var allowedK8sEvents = map[string]struct{}{
	model.K8sEventPodRestart:       {},
	model.K8sEventPodPending:       {},
	model.K8sEventHPAScale:         {},
	model.K8sEventNodeNotReady:     {},
	model.K8sEventFailedScheduling: {},
}

type dataSourceHandler struct {
	db  *gorm.DB
	cfg *config.Config

	// httpc is reused across all upstream HTTP calls (Prometheus proxy +
	// connection tests).  The aggressive timeouts are intentional — these
	// are user-facing buttons, not background scrapes.
	httpc *http.Client
}

func newDataSourceHandler(db *gorm.DB, cfg *config.Config) *dataSourceHandler {
	return &dataSourceHandler{
		db:  db,
		cfg: cfg,
		httpc: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				// We honour `tls_insecure_skip_verify` from the per-row config
				// (set at request time via a request-scoped client clone).
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
				DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				MaxIdleConns:        16,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		},
	}
}

// notifyDataSourceEvent broadcasts a debounce-friendly event to every
// background subscriber of `data_source_event` (today: the Kafka
// manager).  Failures are logged at warn level; we do NOT bubble them up
// because the row was committed successfully and the subscriber's
// 5-minute floor will eventually catch up.
//
// Payload is intentionally tiny — the subscriber only needs "something
// changed for kind X" to decide whether to do a full reload; the actual
// new config is read from the DB inside Reload() so we never have to
// keep two truths in sync.
func (h *dataSourceHandler) notifyDataSourceEvent(ctx context.Context, kind, action, id string) {
	payload := map[string]string{"kind": kind, "action": action, "id": id}
	raw, _ := json.Marshal(payload)
	if err := h.db.WithContext(ctx).
		Exec("SELECT pg_notify(?, ?)", ingestion.DataSourceEventChannel, string(raw)).Error; err != nil {
		log.Warn().Err(err).
			Str("kind", kind).Str("action", action).Str("id", id).
			Msg("failed to notify data_source_event")
	}
}

func (h *dataSourceHandler) registerRoutes(ws *restful.WebService) {
	ws.Route(ws.GET("/data-sources").To(h.list).
		Doc("List data sources (secrets always masked, optional ?kind= filter)").
		Param(ws.QueryParameter("kind", "filter by kind").DataType("string")).
		Metadata(label.MetaIdentity, label.DataSourceList).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/data-sources").To(h.create).
		Doc("Create data source").
		Metadata(label.MetaIdentity, label.DataSourceCreate).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/data-sources/{id}").To(h.update).
		Doc("Update data source (blank secret keeps existing ciphertext)").
		Metadata(label.MetaIdentity, label.DataSourceUpdate).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/data-sources/{id}").To(h.delete).
		Doc("Delete data source").
		Metadata(label.MetaIdentity, label.DataSourceDelete).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/data-sources/{id}/test").To(h.test).
		Doc("Test connection (uses inline body fields if provided, else stored row)").
		Metadata(label.MetaIdentity, label.DataSourceTest).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// Dry-run a single message through this row's filter + mapping
	// pipeline.  Powers the "测试" button on the Kafka data-source
	// drawer so operators can iterate on the expression / paths without
	// having to actually publish to Kafka.  Reuses the
	// label.DataSourceTest identity so anyone allowed to run "测试连接"
	// can also validate transforms.
	ws.Route(ws.POST("/data-sources/{id}/test-message").To(h.testMessage).
		Doc("Dry-run a sample JSON payload through this row's filter/mapping").
		Metadata(label.MetaIdentity, label.DataSourceTest).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/data-sources/{id}/set-default").To(h.setDefault).
		Doc("Mark this row as the default for its kind (clears is_default on siblings)").
		Metadata(label.MetaIdentity, label.DataSourceDefault).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// Prometheus query proxy.  These three endpoints stream upstream
	// responses verbatim so the frontend can use the same JSON shape as
	// Prometheus's native API (Grafana / promxy / graph page also speak it).
	ws.Route(ws.GET("/data-sources/{id}/prom/query").To(h.promQuery).
		Doc("Proxy Prometheus instant query").
		Param(ws.QueryParameter("query", "PromQL").DataType("string")).
		Param(ws.QueryParameter("time", "RFC3339 or unix").DataType("string")).
		Metadata(label.MetaIdentity, label.DataSourceQuery).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.GET("/data-sources/{id}/prom/query_range").To(h.promQueryRange).
		Doc("Proxy Prometheus range query").
		Param(ws.QueryParameter("query", "PromQL").DataType("string")).
		Param(ws.QueryParameter("start", "unix or RFC3339").DataType("string")).
		Param(ws.QueryParameter("end", "unix or RFC3339").DataType("string")).
		Param(ws.QueryParameter("step", "duration or seconds, e.g. 15s").DataType("string")).
		Metadata(label.MetaIdentity, label.DataSourceQuery).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.GET("/data-sources/{id}/prom/labels").To(h.promLabels).
		Doc("Proxy Prometheus /api/v1/labels (autocomplete)").
		Metadata(label.MetaIdentity, label.DataSourceQuery).
		Metadata(label.MetaModule, label.DataSourceModuleName).
		Metadata(label.MetaKind, "DataSource").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))
}

// ─── Views & DTOs ────────────────────────────────────────────────────────────

// dataSourceView is the API-facing shape.  We deliberately strip the raw
// ciphertext blob and replace it with a `secret_keys` slice listing which
// keys the UI should show as "(unchanged)" placeholders.
type dataSourceView struct {
	model.DataSource
	SecretKeys []string `json:"secret_keys"`
}

func (h *dataSourceHandler) toView(row model.DataSource) dataSourceView {
	keys, _ := h.populatedSecretKeys(row.SecretEnc)
	return dataSourceView{DataSource: row, SecretKeys: keys}
}

// dataSourceInput is the inbound DTO for create/update/test.  Secrets ride
// in `Secrets map[string]string` rather than as separate fields so adding a
// new credential type later (e.g. mtls_client_cert) doesn't require touching
// every consumer.  Empty / "******" values mean "keep existing".
type dataSourceInput struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	IsEnabled   bool   `json:"is_enabled"`
	IsDefault   bool   `json:"is_default"`
	// AIEnabled gates manual + optional auto AI for this source (kind whitelist).
	AIEnabled bool `json:"ai_enabled"`
	// AIAutoTrigger: on new incident create, enqueue ai_tasks without operator click.
	AIAutoTrigger bool `json:"ai_auto_trigger"`
	Endpoint  string         `json:"endpoint"`
	Config    map[string]any `json:"config"`

	// Map of secret name → either:
	//   "ENC:<base64-rsa-cipher>"  – preferred, RSA-decoded server-side
	//   ""  / "******"             – update: keep existing ciphertext
	//   "<plaintext>"              – legacy / curl, accepted as-is
	//
	// Known keys per kind (validated below):
	//   prometheus: password (basic), token (bearer)
	//   k8s:        token (bearer)
	//   opensearch: password
	//   kafka:      sasl_password
	Secrets map[string]string `json:"secrets"`
}

const secretMask = "******"

// ─── List / Create / Update / Delete ─────────────────────────────────────────

func (h *dataSourceHandler) list(req *restful.Request, resp *restful.Response) {
	q := h.db.WithContext(req.Request.Context()).Model(&model.DataSource{})
	if k := strings.TrimSpace(req.QueryParameter("kind")); k != "" {
		q = q.Where("kind = ?", k)
	}
	var rows []model.DataSource
	if err := q.Order("kind asc, is_default desc, created_at desc").Find(&rows).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	out := make([]dataSourceView, 0, len(rows))
	for _, r := range rows {
		out = append(out, h.toView(r))
	}
	httputil.Success(resp, out)
}

func (h *dataSourceHandler) create(req *restful.Request, resp *restful.Response) {
	var body dataSourceInput
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}

	row, err := h.materialise(body, model.DataSource{}, true)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	err = h.db.WithContext(req.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if row.IsDefault {
			if err := tx.Model(&model.DataSource{}).
				Where("kind = ? AND is_default = ?", row.Kind, true).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		row.ID = ""
		return tx.Create(&row).Error
	})
	if err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyDataSourceEvent(req.Request.Context(), row.Kind, "create", row.ID)
	httputil.Created(resp, h.toView(row))
}

func (h *dataSourceHandler) update(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")

	var existing model.DataSource
	if err := h.db.WithContext(req.Request.Context()).First(&existing, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(resp)
			return
		}
		httputil.InternalError(resp, err.Error())
		return
	}

	var body dataSourceInput
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}

	row, err := h.materialise(body, existing, false)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt

	wantDefault := row.IsDefault

	err = h.db.WithContext(req.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if wantDefault && !existing.IsDefault {
			if err := tx.Model(&model.DataSource{}).
				Where("kind = ? AND is_default = ? AND id <> ?", row.Kind, true, id).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(&row).Error
	})
	if err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyDataSourceEvent(req.Request.Context(), row.Kind, "update", row.ID)
	httputil.Success(resp, h.toView(row))
}

func (h *dataSourceHandler) delete(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	// Read the kind first so the notify payload tells the manager which
	// subsystem cares — without this it would have to do a full reload
	// even for non-Kafka deletes.
	var existing model.DataSource
	_ = h.db.WithContext(req.Request.Context()).First(&existing, "id = ?", id).Error
	if err := h.db.WithContext(req.Request.Context()).
		Delete(&model.DataSource{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	if existing.Kind != "" {
		h.notifyDataSourceEvent(req.Request.Context(), existing.Kind, "delete", id)
	}
	httputil.Success(resp, nil)
}

func (h *dataSourceHandler) setDefault(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")

	var existing model.DataSource
	if err := h.db.WithContext(req.Request.Context()).First(&existing, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(resp)
			return
		}
		httputil.InternalError(resp, err.Error())
		return
	}

	err := h.db.WithContext(req.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.DataSource{}).
			Where("kind = ? AND is_default = ? AND id <> ?", existing.Kind, true, id).
			Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&model.DataSource{}).
			Where("id = ?", id).
			Updates(map[string]any{"is_default": true, "is_enabled": true}).Error
	})
	if err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyDataSourceEvent(req.Request.Context(), existing.Kind, "set_default", id)
	httputil.Success(resp, map[string]string{"id": id, "status": "default"})
}

// ─── Test connection ─────────────────────────────────────────────────────────

func (h *dataSourceHandler) test(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")

	var body dataSourceInput
	_ = req.ReadEntity(&body)

	// Start from the stored row when we have one, then overlay any inline
	// fields the form submitted.  This is the same pattern as
	// llm_providers.go::testLLMProvider so editing a row + clicking
	// "测试连接" before saving works exactly like the LLM page.
	base := model.DataSource{}
	if id != "" && id != "_" && id != "new" {
		_ = h.db.WithContext(req.Request.Context()).First(&base, "id = ?", id).Error
	}
	if body.Kind != "" {
		base.Kind = body.Kind
	}
	if body.Endpoint != "" {
		base.Endpoint = body.Endpoint
	}
	if body.Config != nil {
		raw, _ := json.Marshal(body.Config)
		base.Config = raw
	}

	// Build the effective secret map: stored ciphertext + any inline
	// overrides.  Inline values are wire-decrypted (ENC:) here just like
	// llm_providers.go does for api_key.
	secrets, err := h.decryptSecrets(base.SecretEnc)
	if err != nil {
		httputil.InternalError(resp, "decrypt stored secrets: "+err.Error())
		return
	}
	for k, v := range body.Secrets {
		decoded := auth.DecodeClientCipher(v)
		if decoded == "" || decoded == secretMask {
			continue
		}
		secrets[k] = decoded
	}

	if _, ok := allowedDataSourceKinds[base.Kind]; !ok {
		httputil.BadRequest(resp, "kind is required")
		return
	}

	ctx, cancel := context.WithTimeout(req.Request.Context(), 15*time.Second)
	defer cancel()

	cfgMap := jsonToMap(base.Config)
	var (
		ok      bool
		message string
		detail  any
	)
	switch base.Kind {
	case model.DataSourceKindPrometheus:
		ok, message, detail = h.testPrometheus(ctx, base.Endpoint, cfgMap, secrets)
	case model.DataSourceKindK8s:
		ok, message, detail = h.testK8s(ctx, base.Endpoint, cfgMap, secrets)
	case model.DataSourceKindOpenSearch, model.DataSourceKindElastic:
		ok, message, detail = h.testOpenSearch(ctx, base.Endpoint, cfgMap, secrets)
	case model.DataSourceKindKafka:
		ok, message, detail = h.testKafka(ctx, base.Endpoint, cfgMap, secrets)
	default:
		httputil.BadRequest(resp, "unsupported kind")
		return
	}

	// Persist last_test_* on the existing row when we have one — gives the
	// list page a "last successful at" timestamp without an extra round trip.
	if id != "" && id != "new" && base.ID != "" {
		now := time.Now().UTC()
		var errPtr *string
		if !ok {
			errPtr = strPtr(message)
		}
		_ = h.db.WithContext(req.Request.Context()).Model(&model.DataSource{}).
			Where("id = ?", base.ID).Updates(map[string]any{
			"last_test_at": now,
			"last_test_ok": ok,
			"last_error":   errPtr,
		}).Error
	}

	out := map[string]any{"ok": ok, "message": message}
	if detail != nil {
		out["detail"] = detail
	}
	httputil.Success(resp, out)
}

func (h *dataSourceHandler) testPrometheus(ctx context.Context, endpoint string, cfg map[string]any, secrets map[string]string) (bool, string, any) {
	if endpoint == "" {
		return false, "endpoint is required", nil
	}
	u, err := url.Parse(strings.TrimRight(endpoint, "/") + "/api/v1/query")
	if err != nil {
		return false, "invalid endpoint: " + err.Error(), nil
	}
	q := u.Query()
	q.Set("query", "1")
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	applyHTTPAuth(req, cfg, secrets)

	res, err := h.httpc.Do(req)
	if err != nil {
		return false, "dial failed: " + err.Error(), nil
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("HTTP %d: %s", res.StatusCode, string(body)), nil
	}
	return true, "ok", json.RawMessage(body)
}

func (h *dataSourceHandler) testK8s(ctx context.Context, endpoint string, cfg map[string]any, secrets map[string]string) (bool, string, any) {
	// In-cluster mode: trust the in-pod ServiceAccount file system (the
	// Phase-3 connector will read /var/run/secrets/...); for now the test
	// just confirms the operator picked a reachable mode.
	if asBool(cfg["in_cluster"]) {
		return true, "in-cluster: skipped (will use ServiceAccount at runtime)", nil
	}
	if endpoint == "" {
		return false, "endpoint is required when in_cluster=false", nil
	}
	token := strings.TrimSpace(secrets["token"])
	if token == "" {
		return false, "bearer token is required when in_cluster=false", nil
	}

	u, err := url.Parse(strings.TrimRight(endpoint, "/") + "/version")
	if err != nil {
		return false, "invalid endpoint: " + err.Error(), nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := h.httpClientForRow(cfg)
	res, err := client.Do(req)
	if err != nil {
		return false, "dial failed: " + err.Error(), nil
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("HTTP %d: %s", res.StatusCode, string(body)), nil
	}
	return true, "ok", json.RawMessage(body)
}

func (h *dataSourceHandler) testOpenSearch(ctx context.Context, endpoint string, cfg map[string]any, secrets map[string]string) (bool, string, any) {
	// Phase-3 connector will use opensearch-go.  For the credentials smoke
	// test a plain HTTP GET / with basic auth is enough — this is what
	// opensearch-go's Info() ends up calling anyway.
	if endpoint == "" {
		return false, "endpoint is required", nil
	}
	u, err := url.Parse(strings.TrimRight(endpoint, "/") + "/")
	if err != nil {
		return false, "invalid endpoint: " + err.Error(), nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if user, _ := cfg["username"].(string); user != "" {
		pwd := secrets["password"]
		req.SetBasicAuth(user, pwd)
	}

	client := h.httpClientForRow(cfg)
	res, err := client.Do(req)
	if err != nil {
		return false, "dial failed: " + err.Error(), nil
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode >= 400 {
		return false, fmt.Sprintf("HTTP %d: %s", res.StatusCode, string(body)), nil
	}
	return true, "ok", json.RawMessage(body)
}

// testMessage is the dry-run endpoint for the "测试" button on the Kafka
// data-source drawer.  Body shape:
//
//	{
//	  "sample": "<json string>" | <json object>,
//	  "config": {                     // optional, defaults to stored row
//	    "filter":  "...",
//	    "mapping": { ... }
//	  }
//	}
//
// Response:
//
//	{
//	  "kept":         bool,
//	  "drop_reason":  "filter_false" | "missing_alertname" | ...,
//	  "raw_alert":    { source, fingerprint, labels, ... } | null,
//	  "debug": {
//	    "filter_eval":      bool | null,
//	    "resolved":         bool,
//	    "mapping_resolved": { "<path>": "<value>" }
//	  }
//	}
//
// Compile errors come back as 400 with the same Chinese error text the
// router would have surfaced on save; this lets the UI block the form on
// invalid expressions before round-tripping the whole row.
func (h *dataSourceHandler) testMessage(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")

	var body struct {
		Sample any            `json:"sample"`
		Config map[string]any `json:"config"`
	}
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}

	// Resolve the effective config: stored row → overlay request body.
	row := model.DataSource{}
	if id != "" && id != "_" && id != "new" {
		_ = h.db.WithContext(req.Request.Context()).First(&row, "id = ?", id).Error
	}
	cfgMap := jsonToMap(row.Config)
	if body.Config != nil {
		// Allow the front-end to test edits without saving by overlaying
		// just the filter / mapping keys (everything else stays from
		// the stored row).
		for _, k := range []string{"filter", "mapping"} {
			if v, ok := body.Config[k]; ok {
				cfgMap[k] = v
			}
		}
	}

	prog, err := ingestion.CompileKafkaProgram(ingestion.KafkaFilterConfig{
		Filter:  asString(cfgMap["filter"]),
		Mapping: kafkaMappingFromCfg(cfgMap),
	})
	if err != nil {
		// Same translation layer as validateKafkaConfig — keeps the
		// dry-run "测试一条样例消息" panel consistent with what the
		// save button would surface.
		httputil.BadRequest(resp, translateKafkaFilterError(err))
		return
	}

	payload, err := normaliseSamplePayload(body.Sample)
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	dsID := row.ID
	res, runErr := prog.Apply(payload, "kafka", dsID)
	if runErr != nil {
		httputil.BadRequest(resp, runErr.Error())
		return
	}

	out := map[string]any{
		"kept":        res.Keep,
		"drop_reason": res.Reason,
		"debug": map[string]any{
			"filter_eval":      res.FilterEval,
			"resolved":         res.Resolved,
			"mapping_resolved": res.MappingHits,
		},
	}
	if res.Keep {
		out["raw_alert"] = res.Alert
	}
	httputil.Success(resp, out)
}

// normaliseSamplePayload accepts both a raw JSON string ("escaped" form
// that's easy to paste from kcat -P stdin) and a parsed JSON object (the
// form Antd's JSON Input emits).  Returns the canonical byte slice the
// filter engine expects.
func normaliseSamplePayload(sample any) ([]byte, error) {
	switch v := sample.(type) {
	case nil:
		return nil, errors.New("sample is required")
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil, errors.New("sample is empty")
		}
		return []byte(s), nil
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("sample is not serialisable: %w", err)
		}
		return raw, nil
	}
}

func (h *dataSourceHandler) testKafka(ctx context.Context, endpoint string, _ map[string]any, _ map[string]string) (bool, string, any) { //nolint:unparam // signature symmetry with testKafkaTopic / testHTTPEndpoint

	// Phase-3 connector will use segmentio/kafka-go (Reader / Dialer).  The
	// TCP smoke test below is intentionally cheap so the "测试连接" button
	// stays snappy and we don't pull kafka-go just for a port-open check.
	// SASL / TLS verification happens at the consumer-loop layer.
	if endpoint == "" {
		return false, "endpoint (brokers) is required", nil
	}
	d := &net.Dialer{Timeout: 5 * time.Second}
	for _, b := range strings.Split(endpoint, ",") {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		conn, err := d.DialContext(ctx, "tcp", b)
		if err != nil {
			return false, fmt.Sprintf("broker %s unreachable: %s", b, err.Error()), nil
		}
		_ = conn.Close()
	}
	return true, "ok (TCP reachable; SASL / TLS verification deferred to consumer)", nil
}

// ─── Prometheus query proxy ──────────────────────────────────────────────────

func (h *dataSourceHandler) promQuery(req *restful.Request, resp *restful.Response) {
	h.promProxy(req, resp, "query", "query", "time")
}

func (h *dataSourceHandler) promQueryRange(req *restful.Request, resp *restful.Response) {
	h.promProxy(req, resp, "query_range", "query", "start", "end", "step")
}

func (h *dataSourceHandler) promLabels(req *restful.Request, resp *restful.Response) {
	h.promProxy(req, resp, "labels")
}

// promProxy is the shared plumbing for the three Prometheus-flavoured
// endpoints.  We rebuild the upstream URL from scratch (rather than
// trusting whatever the browser sent) and forward only the whitelisted
// query parameters so the proxy can never be turned into a generic SSRF
// gadget against the Prometheus host.
func (h *dataSourceHandler) promProxy(req *restful.Request, resp *restful.Response, apiPath string, allowedParams ...string) {
	id := req.PathParameter("id")

	var row model.DataSource
	if err := h.db.WithContext(req.Request.Context()).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(resp)
			return
		}
		httputil.InternalError(resp, err.Error())
		return
	}
	if row.Kind != model.DataSourceKindPrometheus {
		httputil.BadRequest(resp, "data source is not prometheus")
		return
	}
	if row.Endpoint == "" {
		httputil.BadRequest(resp, "data source endpoint is empty")
		return
	}

	upstream, err := url.Parse(strings.TrimRight(row.Endpoint, "/") + "/api/v1/" + apiPath)
	if err != nil {
		httputil.InternalError(resp, "bad endpoint: "+err.Error())
		return
	}
	q := upstream.Query()
	for _, p := range allowedParams {
		if v := req.QueryParameter(p); v != "" {
			q.Set(p, v)
		}
	}
	upstream.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(req.Request.Context(), 30*time.Second)
	defer cancel()

	hreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.String(), nil)

	cfgMap := jsonToMap(row.Config)
	secrets, _ := h.decryptSecrets(row.SecretEnc)
	applyHTTPAuth(hreq, cfgMap, secrets)

	client := h.httpClientForRow(cfgMap)
	res, err := client.Do(hreq)
	if err != nil {
		httputil.Error(resp, http.StatusBadGateway, "upstream call failed: "+err.Error())
		return
	}
	defer func() { _ = res.Body.Close() }()

	// Pass the upstream response straight through.  The frontend already
	// speaks the Prometheus JSON envelope, no remapping needed.
	resp.AddHeader("Content-Type", res.Header.Get("Content-Type"))
	resp.WriteHeader(res.StatusCode)
	_, _ = io.Copy(resp.ResponseWriter, io.LimitReader(res.Body, 8<<20)) // 8 MiB hard cap
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// materialise validates the inbound DTO and produces a model.DataSource
// with `SecretEnc` already encrypted.  When `creating` is false, secrets
// that arrive blank / masked are taken from `existing` so editing
// non-secret fields doesn't require re-entering credentials.
func (h *dataSourceHandler) materialise(in dataSourceInput, existing model.DataSource, creating bool) (model.DataSource, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Kind = strings.TrimSpace(in.Kind)

	if in.Name == "" {
		return model.DataSource{}, errors.New("name is required")
	}
	if in.Kind == "" && existing.Kind == "" {
		return model.DataSource{}, errors.New("kind is required")
	}
	if in.Kind == "" {
		in.Kind = existing.Kind
	}
	if _, ok := allowedDataSourceKinds[in.Kind]; !ok {
		return model.DataSource{}, fmt.Errorf("unsupported kind %q", in.Kind)
	}

	// AI 分析白名单：仅日志类数据源（kafka / opensearch / elastic）支持开启 ai_enabled。
	// data_sources_ai_enabled_kind_chk CHECK 约束是兜底。
	if in.AIEnabled {
		switch in.Kind {
		case model.DataSourceKindKafka, model.DataSourceKindOpenSearch, model.DataSourceKindElastic:
		default:
			return model.DataSource{}, fmt.Errorf("ai_enabled 仅 Kafka / OpenSearch / Elastic 数据源支持，当前 kind=%q", in.Kind)
		}
	}

	aiAuto := in.AIAutoTrigger && in.AIEnabled
	if aiAuto {
		switch in.Kind {
		case model.DataSourceKindKafka, model.DataSourceKindOpenSearch, model.DataSourceKindElastic:
		default:
			return model.DataSource{}, fmt.Errorf("ai_auto_trigger 仅 Kafka / OpenSearch / Elastic 数据源支持，当前 kind=%q", in.Kind)
		}
	}

	cfg := normaliseConfig(in.Kind, in.Config)
	if err := validateKindConfig(in.Kind, in.Endpoint, cfg); err != nil {
		return model.DataSource{}, err
	}
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		return model.DataSource{}, err
	}

	// Merge secrets: start from existing plaintext, overlay inbound (after
	// peeling ENC:) for keys whose value is non-blank-and-non-mask.
	mergedSecrets := map[string]string{}
	if !creating {
		stored, err := h.decryptSecrets(existing.SecretEnc)
		if err != nil {
			return model.DataSource{}, fmt.Errorf("decrypt stored secrets: %w", err)
		}
		mergedSecrets = stored
	}
	for k, v := range in.Secrets {
		decoded := auth.DecodeClientCipher(v)
		if decoded == "" || decoded == secretMask {
			continue
		}
		mergedSecrets[k] = decoded
	}

	if creating {
		if err := requireSecretsForKind(in.Kind, in.Endpoint, cfg, mergedSecrets); err != nil {
			return model.DataSource{}, err
		}
	}

	encSecrets, err := h.encryptSecrets(mergedSecrets)
	if err != nil {
		return model.DataSource{}, fmt.Errorf("encrypt secrets: %w", err)
	}

	return model.DataSource{
		Name:          in.Name,
		Kind:          in.Kind,
		Description:   strings.TrimSpace(in.Description),
		IsEnabled:     in.IsEnabled,
		IsDefault:     in.IsDefault,
		AIEnabled:     in.AIEnabled,
		AIAutoTrigger: aiAuto,
		Endpoint:      strings.TrimSpace(in.Endpoint),
		Config:        datatypes.JSON(cfgRaw),
		SecretEnc:     encSecrets,
	}, nil
}

// normaliseConfig drops unknown keys and coerces obvious types so the
// validator below works against a clean shape.  Anything not in the
// kind-specific allow list is silently discarded — that way a stale
// browser cache can't smuggle a bogus key into the jsonb column.
func normaliseConfig(kind string, raw map[string]any) map[string]any {
	if raw == nil {
		raw = map[string]any{}
	}
	out := map[string]any{}
	switch kind {
	case model.DataSourceKindPrometheus:
		copyKeys(raw, out, "auth_type", "username", "scrape_timeout_seconds", "tls_insecure_skip_verify")
	case model.DataSourceKindK8s:
		copyKeys(raw, out,
			"in_cluster", "ca_cert_pem", "tls_insecure_skip_verify",
			"watched_namespaces", "ignored_namespaces", "ignored_pods_re",
			"events", "mute_seconds", "ignore_restart_count_above",
			"pending_threshold_seconds")
		// Closed enum check on `events`.
		if evs, ok := out["events"].([]any); ok {
			cleaned := make([]any, 0, len(evs))
			for _, e := range evs {
				if s, ok := e.(string); ok {
					if _, valid := allowedK8sEvents[s]; valid {
						cleaned = append(cleaned, s)
					}
				}
			}
			out["events"] = cleaned
		}
	case model.DataSourceKindOpenSearch, model.DataSourceKindElastic:
		copyKeys(raw, out,
			"username", "index", "query", "watermark_field",
			"poll_interval_seconds", "lookback_seconds", "tls_insecure_skip_verify",
			// Per-data-source consumer concurrency.  OpenSearch poller is
			// Phase 4; the value is persisted now so operators don't lose
			// the setting when the consumer ships.
			"consumer_concurrency")
	case model.DataSourceKindKafka:
		copyKeys(raw, out,
			"topic", "group_id", "sasl_mechanism", "sasl_user",
			"tls_enabled", "tls_insecure_skip_verify", "max_per_second",
			// New in incident-lifecycle-v2 follow-up: per-row JSON
			// filter + field mapping.  See model/data_source.go for the
			// shape; ingestion/kafka_filter.go is the consumer.
			"filter", "mapping",
			// Per-data-source consumer concurrency: kafka_manager spawns
			// N independent Reader workers (same GroupID) when present.
			"consumer_concurrency")
	}
	return out
}

func copyKeys(src, dst map[string]any, keys ...string) {
	for _, k := range keys {
		if v, ok := src[k]; ok {
			dst[k] = v
		}
	}
}

// translateKafkaFilterError maps the most common expr compile errors
// returned by ingestion.CompileKafkaProgram into actionable Chinese
// hints surfaced directly to the operator.  We keep the original
// `err.Error()` appended at the tail so power users still see the expr
// position info — the prefix is the part they're meant to act on.
//
// Why this lives in the router and not in ingestion: both
// `testKafkaMessage` (the dry-run UI button) and `validateKafkaConfig`
// (the save button) hit the same error class and want the same hint;
// putting the mapping here lets both call sites share one switch
// without dragging UI-flavoured strings into the engine package.
//
// The patterns matched here are observed against expr-lang v1.x; if
// expr changes its error message format on upgrade the `default`
// branch transparently falls back to the original message so we never
// hide information from the operator.
func translateKafkaFilterError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "expected bool, but got map"):
		// Triggered by pasting `{"filter": "..."}` into the filter
		// textarea — expr parses the leading `{` as a map literal.
		return "filter 编译失败：检测到表达式以 `{` 开头，疑似把 `{\"filter\": \"...\"}` 整段 JSON 粘到了表达式框。" +
			"这里只需要表达式本体，例如 `neq(\"response_body\", \"-\")`。原始错误：" + msg
	case strings.Contains(msg, `unexpected token Operator("matches")`):
		// `matches` is reserved as an infix operator in expr-lang;
		// the function form must use the `regex_match` helper.
		return "filter 编译失败：`matches` 是 expr 内置中缀操作符，不能作为函数调用。请改用 `regex_match(path, pattern)`，例如 `regex_match(\"path\", \"^/api/\")`。原始错误：" + msg
	case strings.Contains(msg, `unexpected token Operator("in")`):
		// Same flavour: `in` is an expr keyword for `x in [a,b]`.
		return "filter 编译失败：`in` 是 expr 关键字，不能作为函数名。请改用 `oneof(path, v1, v2, ...)`，例如 `oneof(\"severity\", \"P0\", \"P1\")`。原始错误：" + msg
	default:
		return msg
	}
}

// validateKindConfig enforces the bare minimum so a half-filled form gets
// a friendly 400 instead of a runtime KeyError when the connector spins up.
func validateKindConfig(kind, endpoint string, cfg map[string]any) error {
	switch kind {
	case model.DataSourceKindPrometheus:
		if endpoint == "" {
			return errors.New("prometheus: endpoint is required (e.g. http://prometheus:9090)")
		}
	case model.DataSourceKindK8s:
		if !asBool(cfg["in_cluster"]) && endpoint == "" {
			return errors.New("k8s: endpoint is required when in_cluster=false")
		}
		evs, _ := cfg["events"].([]any)
		if len(evs) == 0 {
			return errors.New("k8s: at least one event type must be selected")
		}
	case model.DataSourceKindOpenSearch, model.DataSourceKindElastic:
		// Friendly label so operators see the kind they configured rather
		// than always-OpenSearch in error messages.
		label := "opensearch"
		if kind == model.DataSourceKindElastic {
			label = "elastic"
		}
		if endpoint == "" {
			return fmt.Errorf("%s: endpoint is required (e.g. https://elastic.example.com:9200)", label)
		}
		if asString(cfg["index"]) == "" {
			return fmt.Errorf("%s: index is required (e.g. prod-app-logs-*)", label)
		}
		if err := validateConsumerConcurrency(label, cfg); err != nil {
			return err
		}
	case model.DataSourceKindKafka:
		if endpoint == "" {
			return errors.New("kafka: endpoint (brokers) is required (e.g. kafka-1:9092,kafka-2:9092)")
		}
		if asString(cfg["topic"]) == "" {
			return errors.New("kafka: topic is required")
		}
		if asString(cfg["group_id"]) == "" {
			return errors.New("kafka: group_id is required")
		}
		if err := validateConsumerConcurrency("kafka", cfg); err != nil {
			return err
		}
		// Compile-test the filter + mapping so the operator gets a
		// proper 400 with the expr error position before the row is
		// committed.  Without this they'd only learn about a typo when
		// the consumer goroutine spammed warn-level logs at runtime.
		// translateKafkaFilterError replaces opaque expr internals
		// (e.g. "expected bool, but got map") with operator-actionable
		// Chinese hints; falls through to the original error otherwise.
		if _, err := ingestion.CompileKafkaProgram(ingestion.KafkaFilterConfig{
			Filter:  asString(cfg["filter"]),
			Mapping: kafkaMappingFromCfg(cfg),
		}); err != nil {
			return fmt.Errorf("kafka: %s", translateKafkaFilterError(err))
		}
	}
	return nil
}

// validateConsumerConcurrency enforces the [1,8] band on the per-row
// `consumer_concurrency` jsonb key.  Absent / zero values are treated as
// "unset" (the manager defaults them to 1) so existing rows that never
// touched the field stay accepted.  We accept ints, floats (json.Unmarshal
// promotes to float64 by default) and numeric strings — anything else is a
// validation error.
func validateConsumerConcurrency(kind string, cfg map[string]any) error {
	v, ok := cfg["consumer_concurrency"]
	if !ok || v == nil {
		return nil
	}
	var n int
	switch x := v.(type) {
	case float64:
		n = int(x)
	case float32:
		n = int(x)
	case int:
		n = x
	case int64:
		n = int(x)
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return fmt.Errorf("%s: consumer_concurrency 必须是 1-32 之间的整数", kind)
		}
		n = int(i)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		var err error
		var i int64
		i, err = parseIntStrict(s)
		if err != nil {
			return fmt.Errorf("%s: consumer_concurrency 必须是 1-32 之间的整数", kind)
		}
		n = int(i)
	default:
		return fmt.Errorf("%s: consumer_concurrency 必须是 1-32 之间的整数", kind)
	}
	if n == 0 {
		// Treat 0 as "unset" — the manager applies the default of 1.
		return nil
	}
	if n < 1 || n > 32 {
		return fmt.Errorf("%s: consumer_concurrency 必须在 1-32 之间", kind)
	}
	cfg["consumer_concurrency"] = n
	return nil
}

// parseIntStrict accepts only an unsigned base-10 integer (no scientific
// notation, no leading sign, no spaces beyond the trim already done by the
// caller).  Strict on purpose so "1.5" doesn't sneak in via the string path.
func parseIntStrict(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("non-digit")
		}
	}
	var n int64
	for _, r := range s {
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

// kafkaMappingFromCfg coerces the loosely-typed map[string]any (from the
// jsonb round-trip) into the strongly-typed KafkaMapping the filter
// engine expects.  Any unexpected types silently become "" — matching
// normaliseConfig's "drop unknown" stance — so a stale browser cache
// can't smuggle a bool into a path field.
func kafkaMappingFromCfg(cfg map[string]any) ingestion.KafkaMapping {
	m, _ := cfg["mapping"].(map[string]any)
	if m == nil {
		return ingestion.KafkaMapping{}
	}
	out := ingestion.KafkaMapping{
		Alertname:    asString(m["alertname"]),
		Severity:     asString(m["severity"]),
		Fingerprint:  asString(m["fingerprint"]),
		StartsAt:     asString(m["starts_at"]),
		EndsAt:       asString(m["ends_at"]),
		Summary:      asString(m["summary"]),
		Description:  asString(m["description"]),
		StatusPath:   asString(m["status_path"]),
		ResolvedWhen: asString(m["resolved_when"]),
	}
	if labels, ok := m["labels"].(map[string]any); ok {
		out.Labels = map[string]string{}
		for k, v := range labels {
			if s, ok := v.(string); ok && s != "" {
				out.Labels[k] = s
			}
		}
	}
	if anns, ok := m["annotations"].(map[string]any); ok {
		out.Annotations = map[string]string{}
		for k, v := range anns {
			if s, ok := v.(string); ok && s != "" {
				out.Annotations[k] = s
			}
		}
	}
	return out
}

// requireSecretsForKind refuses creation when a kind that always needs a
// credential was created blank.  Updates skip this check since the
// existing ciphertext already covers the requirement.
func requireSecretsForKind(kind, endpoint string, cfg map[string]any, secrets map[string]string) error {
	switch kind {
	case model.DataSourceKindK8s:
		if !asBool(cfg["in_cluster"]) && strings.TrimSpace(secrets["token"]) == "" {
			return errors.New("k8s: bearer token is required when in_cluster=false")
		}
	case model.DataSourceKindOpenSearch, model.DataSourceKindElastic:
		label := "opensearch"
		if kind == model.DataSourceKindElastic {
			label = "elastic"
		}
		if asString(cfg["username"]) != "" && strings.TrimSpace(secrets["password"]) == "" {
			return fmt.Errorf("%s: password is required when username is set", label)
		}
	case model.DataSourceKindKafka:
		if asString(cfg["sasl_mechanism"]) != "" && strings.TrimSpace(secrets["sasl_password"]) == "" {
			return errors.New("kafka: sasl_password is required when sasl_mechanism is set")
		}
	}
	_ = endpoint
	return nil
}

func (h *dataSourceHandler) populatedSecretKeys(enc string) ([]string, error) {
	if enc == "" {
		return []string{}, nil
	}
	m, err := h.decryptSecrets(enc)
	if err != nil {
		return []string{}, err
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		if strings.TrimSpace(v) != "" {
			out = append(out, k)
		}
	}
	return out, nil
}

func (h *dataSourceHandler) encryptSecrets(plain map[string]string) (string, error) {
	if len(plain) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(plain)
	if err != nil {
		return "", err
	}
	if h.cfg == nil || h.cfg.EncryptionKey == "" {
		// Match the LLM provider fallback (development setups w/o
		// EncryptionKey) — round-trips as plaintext so `make run` works
		// without extra env vars.
		return string(raw), nil
	}
	return config.Encrypt(string(raw), h.cfg.EncryptionKey)
}

// decryptSecrets is the read-side counterpart to encryptSecrets.  The
// double-fallback (try AES-GCM, on failure parse as raw JSON) mirrors
// llm_providers.go::decryptAPIKey so rows seeded directly via SQL during
// development still round-trip correctly.
func (h *dataSourceHandler) decryptSecrets(stored string) (map[string]string, error) {
	if stored == "" {
		return map[string]string{}, nil
	}
	if h.cfg != nil && h.cfg.EncryptionKey != "" {
		if plain, err := config.Decrypt(stored, h.cfg.EncryptionKey); err == nil {
			return parseSecretJSON(plain)
		}
		// fall through to plaintext JSON parse
	}
	return parseSecretJSON(stored)
}

func parseSecretJSON(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}, fmt.Errorf("malformed secret blob: %w", err)
	}
	return out, nil
}

// applyHTTPAuth wires basic-auth or bearer-token onto an outgoing request
// according to `cfg.auth_type` (Prometheus) / heuristic (others).
func applyHTTPAuth(req *http.Request, cfg map[string]any, secrets map[string]string) {
	authType := strings.ToLower(asString(cfg["auth_type"]))
	switch authType {
	case "basic":
		if user := asString(cfg["username"]); user != "" {
			req.SetBasicAuth(user, secrets["password"])
		}
	case "bearer":
		if t := strings.TrimSpace(secrets["token"]); t != "" {
			req.Header.Set("Authorization", "Bearer "+t)
		}
	default:
		// Heuristic: token wins, then basic.  Lets a Prometheus row that
		// was stored without an explicit auth_type still authenticate when
		// only a token / username was provided.
		if t := strings.TrimSpace(secrets["token"]); t != "" {
			req.Header.Set("Authorization", "Bearer "+t)
		} else if user := asString(cfg["username"]); user != "" {
			req.SetBasicAuth(user, secrets["password"])
		}
	}
}

// httpClientForRow returns a per-request client honouring the row's
// `tls_insecure_skip_verify` bit.  Cheap to allocate (transport is shared).
func (h *dataSourceHandler) httpClientForRow(cfg map[string]any) *http.Client {
	if !asBool(cfg["tls_insecure_skip_verify"]) {
		return h.httpc
	}
	return &http.Client{
		Timeout: h.httpc.Timeout,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
}

func jsonToMap(j datatypes.JSON) map[string]any {
	if len(j) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	_ = json.Unmarshal(j, &out)
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func strPtr(s string) *string { return &s }
