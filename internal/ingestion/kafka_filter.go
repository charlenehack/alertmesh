package ingestion

// Compiled filter / mapping pipeline used by the Kafka consumer (and the
// `/data-sources/{id}/test-message` dry-run endpoint).
//
// Two orthogonal stages:
//
//  1. Filter — an `expr-lang/expr` boolean expression evaluated against the
//     parsed JSON map.  Empty string == "let everything through".  The
//     expression is compiled once at Reload() time so the hot path only
//     pays an `expr.Run` (interpreted bytecode, no reflection).
//
//  2. Mapping — a struct of gjson paths describing how to project the
//     payload onto a `RawAlert`.  Required: `alertname` and `severity`
//     paths must resolve to non-empty strings.  Everything else is
//     optional; an empty path disables the field.  `labels` and
//     `annotations` are open string→path maps so users can attach arbitrary
//     custom dimensions without us hard-coding a closed set.
//
// Why two libs instead of "one DSL"?  expr is great at boolean logic but a
// pain for "fish out a deep nested key" because every traversal allocates a
// `map[string]any`.  gjson stays in the byte buffer and is ~10× faster at
// path lookup but has no logical operators.  Splitting the responsibility
// keeps the hot path zero-allocation in the common "let through + simple
// path map" case.
//
// Filter semantics — IMPORTANT for "exclusion" expressions:
//
// expr is compiled with `expr.AllowUndefinedVariables()` so the operator
// can write `kubernetes.namespace == "prod"` without us forcing them to
// declare a schema.  The flip side: missing fields evaluate to `nil`,
// and `nil != "DEBUG"` is `true` — so the seemingly innocent
// `level != "DEBUG"` filter SILENTLY KEEPS every message that doesn't
// have a `level` field at all.  Operators migrating from path-based DSLs
// (`$.level == 'DEBUG'` style) often hit this and report "alertmesh
// passes 100% of traffic".
//
// The fix is a small set of payload-bound helpers (has / eq / neq /
// gt / gte / lt / lte / oneof / regex_match / not_empty / get) injected
// into the expr env per Apply call by `pathHelpersFor`.  Every bool
// helper returns `false` when the path doesn't exist, restoring the
// strict "missing field → drop" semantics of the legacy DSL.  Operators
// should use `neq("level", "DEBUG")` instead of `level != "DEBUG"` for
// any exclusion-style filter; see docs/data-sources.md "安全 filter
// helper" for the full migration table.
//
// Helpers ride in via env injection (not `expr.Function`) because expr
// resolves Function bindings at compile time and gives the bound Go
// closure no way to see the per-payload `parsed gjson.Result`.  Env
// entries are looked up at run time, so the closure can capture
// `parsed` cleanly via `pathHelpersFor`.  Trade-off: typos like
// `oneoff(...)` aren't caught at Compile time — they manifest as
// runtime drops, which the test endpoint surfaces in the dry-run UI.

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/tidwall/gjson"
)

// exprPrefix marks a mapping value as an expr-lang expression instead of a
// gjson path.  Choice rationale: we deliberately stayed on gjson for the
// 99% "fish a deep nested key" case (zero alloc, sub-µs).  But once a user
// needs string surgery — strip query string from `path`, normalise REST id
// segments, build a composite fingerprint like `route_name + "|" + path`
// — gjson runs out of road.  The `expr:` prefix gives them the full
// expr-lang DSL with our domain helpers (strip_query / normalize_path /
// regex_replace / coalesce), without forcing a schema migration: the
// stored type stays `string`, the wire format stays a `Record<string,
// string>`, and every legacy row keeps working unchanged.
const exprPrefix = "expr:"

// autoFilterExprKey is the annotation we inject on every Kafka alert that
// passed a non-empty filter, so downstream notifications carry a record of
// which expression admitted the message.  Stable name → safe to grep /
// template against in IM cards or runbooks.  Operators can shadow the auto
// value by declaring the same key in their mapping.annotations list.
const autoFilterExprKey = "kafka_filter_expr"

// hasExprPrefix is the single point where we decide "is this string a path
// or an expression?" so future syntactic sugar (e.g. `{{ … }}`) only needs
// to be added here.
func hasExprPrefix(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), exprPrefix)
}

// stripExprPrefix extracts the body after `expr:` and trims whitespace so
// `expr:foo` and `expr:  foo  ` compile to the same program.
func stripExprPrefix(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), exprPrefix))
}

// KafkaFilterConfig is the JSON-tagged shape that maps directly onto the
// per-row `data_sources.config` jsonb subset relevant to the consumer.  The
// router layer copies these keys verbatim into the column; the manager
// passes the same struct into Compile().
type KafkaFilterConfig struct {
	Filter  string       `json:"filter"`
	Mapping KafkaMapping `json:"mapping"`
}

// KafkaMapping carries gjson paths for the well-known RawAlert fields plus
// open-ended `Labels` / `Annotations` projections.  Path syntax is gjson:
// dot-separated keys, `arr.0` for indices, `[?(@.tag=="x")]` style filters
// are NOT supported (we deliberately stayed on the simple subset for
// auditability).  An empty string disables the field; `Compile` doesn't
// touch the gjson result for empty paths.
type KafkaMapping struct {
	Alertname    string            `json:"alertname"`
	Severity     string            `json:"severity"`
	Fingerprint  string            `json:"fingerprint"`
	StartsAt     string            `json:"starts_at"`
	EndsAt       string            `json:"ends_at"`
	Summary      string            `json:"summary"`
	Description  string            `json:"description"`
	StatusPath   string            `json:"status_path"`
	ResolvedWhen string            `json:"resolved_when"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
}

// compiledField is a "string-valued mapping cell" that may resolve via
// either gjson or an expr program.  We keep both representations on the
// same struct (rather than two parallel maps) so the hot-path dispatcher
// in evalField can do one switch per cell instead of consulting two
// lookup tables.  `raw` is preserved verbatim for diagnostics — the
// test-message endpoint surfaces it back to the UI as the MappingHits
// key (`expr:<raw>` for expr cells).
type compiledField struct {
	raw     string
	isExpr  bool
	program *vm.Program // valid iff isExpr
	path    string      // valid iff !isExpr
}

// hasValue is true when the operator actually configured the cell.  Empty
// strings stay no-ops in evalField so callers don't need to guard at every
// site (mirrors the existing pathString contract).
func (f compiledField) hasValue() bool {
	if f.isExpr {
		return f.program != nil
	}
	return f.path != ""
}

// KafkaProgram is the compiled, ready-to-run combination of a filter VM
// program plus the validated mapping struct.  Treated as immutable after
// Compile returns — Reload() builds a new program and atomically swaps
// pointers in the manager rather than mutating in place.
type KafkaProgram struct {
	cfg          KafkaFilterConfig
	filter       *vm.Program
	resolvedWhen *vm.Program
	hasFilter    bool
	hasResolved  bool

	// Compiled mapping cells.  Every string-valued mapping cell goes
	// through the same compileStringField pipeline so any of them can
	// be either a gjson path or an `expr: …` program — the operator
	// shouldn't have to remember which fields support which syntax.
	alertname   compiledField
	severity    compiledField
	fingerprint compiledField
	startsAt    compiledField
	endsAt      compiledField
	statusPath  compiledField
	summary     compiledField
	description compiledField
	labels      []labelField
	annotations []labelField
}

// labelField pairs a stable output key (the column under
// RawAlert.Labels / .Annotations) with its compiled cell.  We stash both
// here rather than reusing map[string]compiledField because Go map
// iteration order is randomised, and we want the test endpoint to render
// rows in the same order the operator typed them — Form.List preserves
// order via slice, so we mirror that.
type labelField struct {
	key   string
	field compiledField
}

// CompileKafkaProgram validates and compiles the per-row config.  Errors
// are wrapped with friendly Chinese hints so the router can surface them
// straight to the operator without further rewriting.
func CompileKafkaProgram(cfg KafkaFilterConfig) (*KafkaProgram, error) {
	mapping := cfg.Mapping
	if strings.TrimSpace(mapping.Alertname) == "" {
		return nil, errors.New("kafka mapping: alertname 路径必填（例如 \"alertname\" 或 \"alert.name\"）")
	}
	if strings.TrimSpace(mapping.Severity) == "" {
		return nil, errors.New("kafka mapping: severity 路径必填（例如 \"severity\" 或 \"alert.level\"）")
	}

	prog := &KafkaProgram{cfg: cfg}

	if filterSrc := strings.TrimSpace(cfg.Filter); filterSrc != "" {
		p, err := compileBoolExpr(filterSrc)
		if err != nil {
			return nil, fmt.Errorf("kafka filter 编译失败：%w", err)
		}
		prog.filter = p
		prog.hasFilter = true
	}
	if rwSrc := strings.TrimSpace(mapping.ResolvedWhen); rwSrc != "" {
		p, err := compileBoolExpr(rwSrc)
		if err != nil {
			return nil, fmt.Errorf("kafka mapping.resolved_when 编译失败：%w", err)
		}
		prog.resolvedWhen = p
		prog.hasResolved = true
	}

	for _, cell := range []struct {
		name string
		raw  string
		dst  *compiledField
	}{
		{"alertname", mapping.Alertname, &prog.alertname},
		{"severity", mapping.Severity, &prog.severity},
		{"fingerprint", mapping.Fingerprint, &prog.fingerprint},
		{"starts_at", mapping.StartsAt, &prog.startsAt},
		{"ends_at", mapping.EndsAt, &prog.endsAt},
		{"status_path", mapping.StatusPath, &prog.statusPath},
		{"summary", mapping.Summary, &prog.summary},
		{"description", mapping.Description, &prog.description},
	} {
		f, err := compileStringField(cell.name, cell.raw)
		if err != nil {
			return nil, err
		}
		*cell.dst = f
	}

	prog.labels = make([]labelField, 0, len(mapping.Labels))
	for k, raw := range mapping.Labels {
		if k == "" || strings.TrimSpace(raw) == "" {
			continue
		}
		f, err := compileStringField("labels."+k, raw)
		if err != nil {
			return nil, err
		}
		prog.labels = append(prog.labels, labelField{key: k, field: f})
	}

	prog.annotations = make([]labelField, 0, len(mapping.Annotations))
	for k, raw := range mapping.Annotations {
		if k == "" || strings.TrimSpace(raw) == "" {
			continue
		}
		f, err := compileStringField("annotations."+k, raw)
		if err != nil {
			return nil, err
		}
		prog.annotations = append(prog.annotations, labelField{key: k, field: f})
	}

	return prog, nil
}

// compileStringField is the per-cell entry point shared by fingerprint,
// labels and annotations.  Empty values produce a zero-value compiledField
// (hasValue == false) so the caller can keep the "absent path is a
// no-op" semantics that the v1 codebase relied on.
func compileStringField(name, raw string) (compiledField, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return compiledField{raw: raw}, nil
	}
	if !hasExprPrefix(trimmed) {
		return compiledField{raw: raw, path: trimmed}, nil
	}
	body := stripExprPrefix(trimmed)
	if body == "" {
		return compiledField{}, fmt.Errorf("kafka mapping.%s 编译失败：expr 表达式为空", name)
	}
	prog, err := compileStringExpr(body)
	if err != nil {
		return compiledField{}, fmt.Errorf("kafka mapping.%s 编译失败：%w%s", name, err, exprFieldNameHint(err))
	}
	return compiledField{raw: raw, isExpr: true, program: prog}, nil
}

// exprFieldNameHint returns a short Chinese remediation tip when the expr
// compile error looks like the operator pasted a key with `@` or `-` (e.g.
// `@timestamp` / `x-trace-id`) directly into expr mode, where those bytes
// are not valid identifier characters.  The hint nudges them to either
// switch the cell to gjson mode (which treats the whole string as a path)
// or use bracket notation in expr (`this["@timestamp"]`).
//
// We keep this best-effort and string-based — the expr-lang error already
// carries the offending rune (e.g. `unrecognized character: U+0040 '@'`),
// so we just pattern-match on those well-known markers.
func exprFieldNameHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "U+0040") || strings.Contains(msg, "U+002D") || strings.Contains(msg, "unexpected token") {
		return ` （提示：含 @ / - 的字段名（如 @timestamp、x-trace-id）请改用 gjson 模式直接填字段名，或在 expr 中写 this["@timestamp"]）`
	}
	return ""
}

// compileBoolExpr is a shared helper so the filter and the resolved_when
// expression go through identical compile-time validation.  `AsBool()`
// makes the runtime contract explicit — the program is rejected at compile
// time if the expression cannot return a bool.  Domain helper functions
// are also exposed to the boolean expressions so e.g.
// `strip_query(path) != "/healthz"` works as a filter.
func compileBoolExpr(src string) (*vm.Program, error) {
	opts := append([]expr.Option{
		expr.AllowUndefinedVariables(),
		expr.AsBool(),
	}, envOptions()...)
	return expr.Compile(src, opts...)
}

// compileStringExpr enforces a string-typed return (we coerce non-strings
// in evalField but rejecting at compile time gives the operator faster
// feedback for the common “severity == "P0" ? "critical" : "warn"“-style
// mistakes that compile cleanly otherwise).
func compileStringExpr(src string) (*vm.Program, error) {
	opts := append([]expr.Option{
		expr.AllowUndefinedVariables(),
		expr.AsKind(reflect.String),
	}, envOptions()...)
	return expr.Compile(src, opts...)
}

// KafkaApplyResult bundles every observable side-effect of a single
// Apply() call so both the consumer and the dry-run endpoint can render
// the same diagnostic detail.
type KafkaApplyResult struct {
	Keep        bool
	Reason      string // populated when Keep == false; empty otherwise
	Alert       RawAlert
	FilterEval  *bool             // nil when filter == ""
	Resolved    bool              // true when status mapped to "resolved"
	MappingHits map[string]string // every path → resolved value (debug only)
}

// Apply evaluates filter + mapping against a single Kafka payload, and
// allocates a MappingHits debug map for the test-message endpoint to
// surface back to the UI.  Hot-path consumers should call
// ApplyForConsumer instead — the debug map is the largest per-message
// allocation in the pipeline and is wasted work outside the test
// endpoint.
func (p *KafkaProgram) Apply(payload []byte, source string, dataSourceID string) (KafkaApplyResult, error) {
	return p.apply(payload, source, dataSourceID, true)
}

// ApplyForConsumer is the per-message hot path used by KafkaManager.
// Identical semantics to Apply except MappingHits is left nil — the
// consumer never reads it, and skipping the allocation/recording shaves
// off one map-per-message plus N map writes (one per mapping field) in
// the steady state.  On a 5k msg/s topic that's ~50k map ops/s the
// engine no longer has to do.
func (p *KafkaProgram) ApplyForConsumer(payload []byte, source string, dataSourceID string) (KafkaApplyResult, error) {
	return p.apply(payload, source, dataSourceID, false)
}

// apply is the shared implementation.  recordHits=false replaces every
// MappingHits write with a no-op via the nil-map check in pathString /
// evalField; this keeps the debug-rendered output bit-identical for the
// test endpoint while removing it entirely from the consumer path.
//
// labels → annotations → fingerprint, each with its own miss/error
// branch.  Splitting harms readability without removing complexity.
//
//nolint:gocyclo // intentional state machine: filter → standard fields →
func (p *KafkaProgram) apply(payload []byte, source string, dataSourceID string, recordHits bool) (KafkaApplyResult, error) {
	if !gjson.ValidBytes(payload) {
		return KafkaApplyResult{Reason: "bad_json"}, nil
	}

	parsed := gjson.ParseBytes(payload)

	res := KafkaApplyResult{}
	if recordHits {
		res.MappingHits = map[string]string{}
	}

	// envCache is built once per Apply, lazily on first expr evaluation.
	// The filter / fingerprint / labels / annotations / resolved_when
	// blocks all share it via &envCache so we pay the
	// jsonValueToInterface allocation at most once per message even when
	// the whole pipeline is expr-heavy.
	var envCache map[string]any

	// 1. Filter.  We only build the env map when there's something to
	// run — JSON->map allocation is the most expensive thing on the hot
	// path so the no-filter case stays cheap.
	if p.hasFilter {
		env := buildEnv(parsed, &envCache)
		out, err := expr.Run(p.filter, env)
		if err != nil {
			return KafkaApplyResult{Reason: "filter_error"}, fmt.Errorf("filter runtime: %w", err)
		}
		ok, _ := out.(bool)
		res.FilterEval = &ok
		if !ok {
			res.Reason = "filter_false"
			return res, nil
		}
	}

	// 2. Mapping — pull every path; absentees are empty strings.  Both
	// alertname and severity must resolve non-empty for the alert to
	// progress; everything else is best-effort.
	alertname, err := p.evalField(parsed, &envCache, p.alertname, res.MappingHits)
	if err != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.alertname runtime: %w", err)
	}
	severity, err := p.evalField(parsed, &envCache, p.severity, res.MappingHits)
	if err != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.severity runtime: %w", err)
	}
	if alertname == "" {
		res.Reason = "missing_alertname"
		return res, nil
	}
	if severity == "" {
		res.Reason = "missing_severity"
		return res, nil
	}

	labels := map[string]string{
		"alertname": alertname,
		"severity":  severity,
		"source":    source,
	}
	for _, lf := range p.labels {
		v, err := p.evalField(parsed, &envCache, lf.field, res.MappingHits)
		if err != nil {
			res.Reason = "mapping_error"
			return res, fmt.Errorf("mapping.labels.%s runtime: %w", lf.key, err)
		}
		if v != "" {
			labels[lf.key] = v
		}
	}

	annotations := map[string]string{}
	if v, err := p.evalField(parsed, &envCache, p.summary, res.MappingHits); err != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.summary runtime: %w", err)
	} else if v != "" {
		annotations["summary"] = v
	}
	if v, err := p.evalField(parsed, &envCache, p.description, res.MappingHits); err != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.description runtime: %w", err)
	} else if v != "" {
		annotations["description"] = v
	}
	for _, af := range p.annotations {
		v, err := p.evalField(parsed, &envCache, af.field, res.MappingHits)
		if err != nil {
			res.Reason = "mapping_error"
			return res, fmt.Errorf("mapping.annotations.%s runtime: %w", af.key, err)
		}
		if v != "" {
			annotations[af.key] = v
		}
	}
	// Auto-injected: which filter let this message through.  Only set when
	// there's actually a filter to attribute the alert to (an empty filter
	// means "match all" — there's no distinguishing condition worth
	// recording).  We respect a user-provided shadow value: if their
	// mapping.annotations already defined the same key (possibly empty was
	// stripped above, hence the existence check), we leave it alone.
	if filterSrc := strings.TrimSpace(p.cfg.Filter); filterSrc != "" {
		if _, exists := annotations[autoFilterExprKey]; !exists {
			annotations[autoFilterExprKey] = filterSrc
		}
	}

	startsAt := time.Now().UTC()
	if v, err := p.evalField(parsed, &envCache, p.startsAt, res.MappingHits); err != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.starts_at runtime: %w", err)
	} else if v != "" {
		if t, ok := parseFlexibleTime(v); ok {
			startsAt = t
		}
	}

	var endsAt *time.Time
	if v, err := p.evalField(parsed, &envCache, p.endsAt, res.MappingHits); err != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.ends_at runtime: %w", err)
	} else if v != "" {
		if t, ok := parseFlexibleTime(v); ok {
			endsAt = &t
		}
	}

	// 3. Status / resolved gating.  Two independent signals that fold
	// into a single boolean: an explicit status field AND/OR an expr
	// that returns true.  Either being satisfied flips the alert to
	// resolved so the v2 lifecycle's onResolved fast-path fires.
	status := "firing"
	resolved := false
	if v, err := p.evalField(parsed, &envCache, p.statusPath, res.MappingHits); err != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.status_path runtime: %w", err)
	} else if v != "" {
		if isResolvedKeyword(v) {
			resolved = true
		}
	}
	if !resolved && p.hasResolved {
		env := buildEnv(parsed, &envCache)
		out, err := expr.Run(p.resolvedWhen, env)
		if err == nil {
			if ok, _ := out.(bool); ok {
				resolved = true
			}
		}
	}
	if resolved {
		status = "resolved"
		if endsAt == nil {
			t := time.Now().UTC()
			endsAt = &t
		}
	}

	fp, fpErr := p.evalField(parsed, &envCache, p.fingerprint, res.MappingHits)
	if fpErr != nil {
		res.Reason = "mapping_error"
		return res, fmt.Errorf("mapping.fingerprint runtime: %w", fpErr)
	}
	if fp == "" {
		fp = ComputeFingerprint(labels)
	}

	res.Keep = true
	res.Resolved = resolved
	res.Alert = RawAlert{
		Source:       source,
		Fingerprint:  fp,
		Labels:       labels,
		Annotations:  annotations,
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		Status:       status,
		RawPayload:   append([]byte(nil), payload...),
		DataSourceID: dataSourceID,
	}
	return res, nil
}

// evalField is the single dispatcher for "string-valued mapping cell".
// gjson cells reuse the existing pathString helper (so the legacy
// MappingHits debug map stays keyed by raw path); expr cells share the
// per-message env cache and surface their resolved value under
// "expr:<raw>" so the test-message endpoint can render side-by-side rows
// with the original expression and what it evaluated to.
//
// Non-string return values are coerced to their fmt.Sprint form for
// resilience — the compiler already enforces AsKind(String) on direct
// return, but a `coalesce(int_field, "")` could legally produce a non-
// string at runtime, and we'd rather stringify than crash.
func (p *KafkaProgram) evalField(parsed gjson.Result, envCache *map[string]any, f compiledField, hits map[string]string) (string, error) {
	if !f.hasValue() {
		return "", nil
	}
	if !f.isExpr {
		return pathString(parsed, f.path, hits), nil
	}
	env := buildEnv(parsed, envCache)
	out, err := expr.Run(f.program, env)
	if err != nil {
		return "", err
	}
	var s string
	switch v := out.(type) {
	case string:
		s = v
	case nil:
		s = ""
	default:
		s = fmt.Sprint(v)
	}
	if hits != nil {
		hits[exprPrefix+f.raw] = s
	}
	return s, nil
}

// buildEnv lazily constructs the env map shared by every expr.Run in a
// single Apply call.  We deliberately stash the cache on the caller's
// stack (via *map) instead of a struct field so KafkaProgram stays
// goroutine-safe — the dispatcher fans out across worker goroutines and
// each gets its own envCache.
//
// Two layers of helpers are mixed in here:
//
//  1. Stateless transforms (`builtinFunctions`: strip_query, normalize_path,
//     regex_replace, coalesce) — same Go closure for every payload, just
//     attached so operators don't have to import them.
//  2. Payload-bound predicates (`pathHelpersFor`: has, get, eq, neq,
//     gt/gte/lt/lte, oneof, matches, not_empty) — closures freshly
//     bound to the current `parsed` so they can do gjson lookups under
//     the operator's chosen path.  These are the strict-semantics
//     filter helpers: they treat "field missing" as false rather than
//     letting expr's nil-comparison silently pass everything through.
func buildEnv(parsed gjson.Result, cache *map[string]any) map[string]any {
	if cache != nil && *cache != nil {
		return *cache
	}
	v := jsonValueToInterface(parsed)
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		m = map[string]any{}
	}
	for k, fn := range builtinFunctions {
		// Stateless helpers.  Operators can still shadow them with
		// payload fields of the same name — that's intentional:
		// payload data wins over helpers, matching the standard
		// expr precedence rule.
		if _, exists := m[k]; !exists {
			m[k] = fn
		}
	}
	for k, fn := range pathHelpersFor(parsed) {
		// Payload-bound helpers.  Same shadow-by-payload rule applies
		// — if the JSON has a top-level "eq" key the operator clearly
		// meant the payload field, not our helper.
		if _, exists := m[k]; !exists {
			m[k] = fn
		}
	}
	if cache != nil {
		*cache = m
	}
	return m
}

// pathHelpersFor returns the per-payload helper closures injected into
// the expr env.  Each closure captures `parsed` and does a gjson lookup
// under the path argument the operator passed.
//
// Strict-semantics contract: every bool-returning helper returns false
// when the path doesn't exist or the value can't be coerced to the
// expected type.  This is the intentional difference from raw expr
// dot-access (`level != "DEBUG"` is true when `level` is missing because
// expr's nil compares unequal to everything); the helpers let
// "exclusion" filters round-trip with the legacy DSL's `if !exists
// { return false }` semantics.
//
// Allocation note: this builds 11 closures per Apply call.  Each
// closure is ~16 bytes; we trade the allocation for the simplicity of
// having `parsed` as a pure free variable so KafkaProgram stays
// goroutine-safe across the M-processor fan-out.
func pathHelpersFor(parsed gjson.Result) map[string]any {
	getStr := func(path string) (string, bool) {
		v := parsed.Get(path)
		if !v.Exists() {
			return "", false
		}
		return v.String(), true
	}
	getNum := func(path string) (float64, bool) {
		v := parsed.Get(path)
		if !v.Exists() {
			return 0, false
		}
		if v.Type == gjson.Number {
			return v.Float(), true
		}
		// String-encoded numbers are common in real-world payloads
		// (e.g. Higress logs status_code as the string "500").  We
		// accept them so operators don't have to remember which
		// upstream serialises numbers as strings.
		f, err := strconv.ParseFloat(strings.TrimSpace(v.String()), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}

	return map[string]any{
		"has": func(path string) bool {
			return parsed.Get(path).Exists()
		},
		"get": func(path string) string {
			s, _ := getStr(path)
			return s
		},
		"eq": func(path, val string) bool {
			s, ok := getStr(path)
			return ok && s == val
		},
		"neq": func(path, val string) bool {
			// Strict: missing path → false (NOT true).  This is the
			// whole reason this helper exists.
			s, ok := getStr(path)
			return ok && s != val
		},
		// Numeric comparisons take `any` for the threshold rather
		// than float64 so expr's int literals (`gte("code", 500)`)
		// don't trip on env-side reflection coercion (`int → float64`
		// isn't an automatic conversion under reflect).  toFloat
		// handles int / float / string-encoded numbers symmetrically.
		"gt": func(path string, n any) bool {
			f, ok := getNum(path)
			if !ok {
				return false
			}
			rhs, rok := toFloat(n)
			return rok && f > rhs
		},
		"gte": func(path string, n any) bool {
			f, ok := getNum(path)
			if !ok {
				return false
			}
			rhs, rok := toFloat(n)
			return rok && f >= rhs
		},
		"lt": func(path string, n any) bool {
			f, ok := getNum(path)
			if !ok {
				return false
			}
			rhs, rok := toFloat(n)
			return rok && f < rhs
		},
		"lte": func(path string, n any) bool {
			f, ok := getNum(path)
			if !ok {
				return false
			}
			rhs, rok := toFloat(n)
			return rok && f <= rhs
		},
		"oneof": func(path string, vs ...string) bool {
			s, ok := getStr(path)
			if !ok {
				return false
			}
			for _, v := range vs {
				if v == s {
					return true
				}
			}
			return false
		},
		// `matches` is a reserved infix operator in expr-lang
		// (`s matches "p"`), so the function form has to live under
		// a different name.  `regex_match` keeps the snake_case
		// convention the rest of the helpers use.
		"regex_match": func(path, pattern string) bool {
			s, ok := getStr(path)
			if !ok {
				return false
			}
			re, err := getCachedRegex(pattern)
			if err != nil {
				return false
			}
			return re.MatchString(s)
		},
		"not_empty": func(path string) bool {
			// Same placeholder rules as coalesce: treat "", "-",
			// "null" as empty so operators don't have to special-case
			// access-log style "missing" markers.
			s, ok := getStr(path)
			if !ok {
				return false
			}
			t := strings.TrimSpace(s)
			return t != "" && t != "-" && t != "null"
		},
	}
}

// pathString reads a gjson path and records both the path itself and the
// extracted value into the debug map (used only by the test endpoint).
// An empty path is a deliberate no-op so callers don't have to guard.
// hits == nil means "skip diagnostic recording" — that's the consumer
// hot path; the gjson lookup itself is unchanged so behaviour stays
// identical apart from the missing debug map entry.
func pathString(parsed gjson.Result, path string, hits map[string]string) string {
	if path == "" {
		return ""
	}
	v := parsed.Get(path)
	if !v.Exists() {
		if hits != nil {
			hits[path] = ""
		}
		return ""
	}
	s := v.String()
	if hits != nil {
		hits[path] = s
	}
	return s
}

// isResolvedKeyword normalises the various "we're better now" spellings
// that real-world JSON payloads ship.  Anything else (including "ok",
// which means different things in different schemas) requires the
// resolved_when expression instead so we don't accidentally close
// incidents on heartbeat-style messages.
func isResolvedKeyword(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "resolved", "ok", "recovered", "recover", "cleared", "info":
		return true
	}
	return false
}

// parseFlexibleTime tries the formats real users actually emit:
// RFC3339 nanosecond, RFC3339, plain seconds-since-epoch, milliseconds.
// Returns ok=false if none match — caller falls back to time.Now().
func parseFlexibleTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), true
		}
	}
	// Numeric epoch (string form, since gjson's String() always
	// returns string).  10 digits == seconds, 13 == milliseconds.
	if len(v) >= 10 && len(v) <= 19 {
		n, err := parseInt(v)
		if err == nil {
			switch {
			case n > 1e18: // ns
				return time.Unix(0, n).UTC(), true
			case n > 1e15: // µs
				return time.Unix(0, n*1e3).UTC(), true
			case n > 1e12: // ms
				return time.Unix(0, n*1e6).UTC(), true
			case n > 1e9: // s
				return time.Unix(n, 0).UTC(), true
			}
		}
	}
	return time.Time{}, false
}

// parseInt is strconv.ParseInt without the import for this single use.
func parseInt(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// builtinFunctions are the domain helpers we expose to every expr-lang
// program (filter, resolved_when, mapping cells).  Kept as plain Go
// closures rather than expr.Function() options so they can also be
// injected as env entries — that gives both `expr.Compile` static type
// checks (via envOptions) and `expr.Run` runtime resolution (via
// buildEnv) without duplicating definitions.
//
// Ordering rationale: small set, stable names, snake_case to match the
// JSON convention the rest of alertmesh uses for config knobs.  We keep
// the surface deliberately tight — operators that need more should reach
// for `regex_replace`, not have us add `path_lower`/`path_collapse`/etc.
// every time a use case shows up.
var builtinFunctions = map[string]any{
	"strip_query":    stripQuery,
	"normalize_path": normalizePath,
	"regex_replace":  regexReplace,
	"coalesce":       coalesce,
}

// envOptions returns the expr.Compile options that mirror
// builtinFunctions.  We register them as expr.Function so the compiler
// can resolve call sites at compile time (catching typos like
// `strip_querty`); the runtime fallback via buildEnv is for symmetry
// only.
//
// Path-bound filter helpers (has / eq / neq / gt / gte / lt / lte /
// oneof / regex_match / not_empty / get) intentionally do NOT live in
// this list: expr resolves expr.Function bindings at compile time and
// uses the registered Go closure as the call target, leaving no way
// for the per-payload `parsed gjson.Result` to reach the body.
// Instead, those helpers ride in via env injection
// (buildEnv → pathHelpersFor(parsed)); under
// `expr.AllowUndefinedVariables()` the call site `eq("level", "x")`
// compiles into a "call undefined identifier" node that resolves to
// whatever the env has under that name at run time, which is exactly
// the payload-bound closure.
//
// Trade-off: typos like `oneoff(...)` or `eq("level")` (wrong arity)
// compile clean and only manifest as a runtime drop / nil call.  We
// accept this in exchange for letting helpers see the parsed JSON;
// the test endpoint surfaces the runtime error in the dry-run UI so
// the loss is bounded.
func envOptions() []expr.Option {
	return []expr.Option{
		expr.Function("strip_query",
			func(params ...any) (any, error) {
				if len(params) == 0 {
					return "", nil
				}
				return stripQuery(toString(params[0])), nil
			},
			new(func(string) string),
		),
		expr.Function("normalize_path",
			func(params ...any) (any, error) {
				if len(params) == 0 {
					return "", nil
				}
				return normalizePath(toString(params[0])), nil
			},
			new(func(string) string),
		),
		expr.Function("regex_replace",
			func(params ...any) (any, error) {
				if len(params) < 3 {
					return "", errors.New("regex_replace(s, pattern, repl) 需 3 个参数")
				}
				out, err := regexReplace(toString(params[0]), toString(params[1]), toString(params[2]))
				if err != nil {
					return "", err
				}
				return out, nil
			},
			new(func(string, string, string) (string, error)),
		),
		expr.Function("coalesce",
			func(params ...any) (any, error) {
				return coalesce(params...), nil
			},
		),
	}
}

// stripQuery returns the substring of s before the first `?`.  No
// allocation when the string is unchanged, so the common
// "path-already-clean" case (Higress access logs) costs one IndexByte.
func stripQuery(s string) string {
	if i := strings.IndexByte(s, '?'); i >= 0 {
		return s[:i]
	}
	return s
}

// pathIDPatterns is the stock regex set we treat as "this path segment is
// an id, replace it with {id} for fingerprint stability".  Order matters:
// most-specific patterns come first so e.g. an ETH address doesn't get
// matched by the long-hex rule and lose its `0x` prefix.  Operators that
// need extra patterns can fall back to `regex_replace` per call site —
// we keep the global set deliberately small + audit-friendly.
var pathIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`), // UUID
	regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`),                                                           // ETH address
	regexp.MustCompile(`^0x[0-9a-fA-F]{16,}$`),                                                          // long hex w/ 0x prefix (tx hash etc.)
	regexp.MustCompile(`^[0-9a-fA-F]{32,}$`),                                                            // bare long hex (md5/sha)
	regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`),                                                 // base58 (Solana addr / BTC)
	regexp.MustCompile(`^[A-Za-z0-9_-]{20,}$`),                                                          // base64url-ish opaque token (caught last)
	regexp.MustCompile(`^\d+$`),                                                                         // plain digits id
}

// normalizePath splits s on `/`, replaces every segment that matches
// pathIDPatterns with the literal `{id}`, and rejoins.  Leading / trailing
// slashes are preserved.  Empty / nil input is returned as-is so the
// helper composes safely with strip_query on missing fields.
//
// Performance note: the implementation builds a new string via
// strings.Join only when at least one substitution happens, so the
// "nothing to normalise" path stays allocation-free in the steady state.
func normalizePath(s string) string {
	if s == "" {
		return s
	}
	parts := strings.Split(s, "/")
	changed := false
	for i, seg := range parts {
		if seg == "" {
			continue
		}
		for _, re := range pathIDPatterns {
			if re.MatchString(seg) {
				parts[i] = "{id}"
				changed = true
				break
			}
		}
	}
	if !changed {
		return s
	}
	return strings.Join(parts, "/")
}

// regexCache memoises compiled regexes used by regex_replace AND the
// matches() filter helper so a hot expression that runs
// `regex_replace(path, "/users/\\d+", "/users/{id}")` or
// `matches("path", "^/api/v1/")` per message doesn't pay for
// `regexp.Compile` every call.  Bounded implicitly by the number of
// distinct user-supplied patterns, which in practice is one or two per
// data source.
var (
	regexCache   = map[string]*regexp.Regexp{}
	regexCacheMu sync.RWMutex
)

// getCachedRegex returns a compiled regex for pattern, compiling on
// first use and caching the result.  Shared by every site that needs
// user-supplied regex (regex_replace, matches helper) so callers don't
// each grow their own cache.  The returned *regexp.Regexp is safe for
// concurrent use per the regexp package docs.
func getCachedRegex(pattern string) (*regexp.Regexp, error) {
	regexCacheMu.RLock()
	re, ok := regexCache[pattern]
	regexCacheMu.RUnlock()
	if ok {
		return re, nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCacheMu.Lock()
	// Re-check under the write lock in case a concurrent caller won
	// the compile race; either compiled program is functionally
	// identical so we keep the first to land.
	if existing, ok := regexCache[pattern]; ok {
		regexCacheMu.Unlock()
		return existing, nil
	}
	regexCache[pattern] = compiled
	regexCacheMu.Unlock()
	return compiled, nil
}

func regexReplace(s, pattern, repl string) (string, error) {
	re, err := getCachedRegex(pattern)
	if err != nil {
		return "", fmt.Errorf("regex_replace 编译失败：%w", err)
	}
	return re.ReplaceAllString(s, repl), nil
}

// coalesce returns the first non-empty / non-placeholder argument, or
// "" if none qualifies.  The "placeholder" heuristic matches Higress /
// Envoy access-log defaults ("-") and the common Logback null-string
// ("null") so an operator can write
// `coalesce(x_real_ip, downstream_remote_address)` and have the dash
// from the upstream log automatically skipped.
func coalesce(values ...any) any {
	for _, v := range values {
		if v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return v
		}
		t := strings.TrimSpace(s)
		if t == "" || t == "-" || t == "null" {
			continue
		}
		return s
	}
	return ""
}

// toString is the cheap normaliser shared by the expr.Function wrappers.
// gjson.String() already returns "" for missing keys so the most common
// "field not present" path stays well-behaved; we only need to handle
// non-string scalars (numbers from `gjson.Number` → float64 in env).
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

// toFloat normalises numeric arguments arriving from expr.  expr emits
// integer literals as `int` and float literals as `float64`, plus
// occasionally an `int64` after arithmetic; reflect-based env calls
// don't auto-coerce between them, so the numeric helpers (gt/gte/lt/lte)
// take `any` and route through here.  String-encoded numbers are
// accepted for symmetry with getNum on the LHS.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// jsonValueToInterface converts a gjson.Result tree into the plain
// `map[string]any` / `[]any` / scalar shape expr expects in its env.  We
// keep the conversion confined to this file so the rest of the codebase
// never accidentally depends on gjson types.
func jsonValueToInterface(v gjson.Result) any {
	switch v.Type {
	case gjson.Null:
		return nil
	case gjson.False:
		return false
	case gjson.True:
		return true
	case gjson.Number:
		return v.Float()
	case gjson.String:
		return v.String()
	}
	if v.IsArray() {
		out := make([]any, 0)
		v.ForEach(func(_, item gjson.Result) bool {
			out = append(out, jsonValueToInterface(item))
			return true
		})
		return out
	}
	if v.IsObject() {
		out := map[string]any{}
		v.ForEach(func(k, item gjson.Result) bool {
			out[k.String()] = jsonValueToInterface(item)
			return true
		})
		return out
	}
	return v.Raw
}
