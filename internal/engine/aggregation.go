package engine

import (
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

const defaultGroupWait = 30 * time.Second

// aggregatorShards is the fan-out factor for the per-group_key mutex.
// 16 is enough to make worst-case contention between K consumer
// goroutines roughly K/16 per shard while keeping the per-Aggregator
// memory footprint trivial (16 maps + 16 mutexes ≈ a few KiB).  Power
// of two so the modulo collapses to a mask in the compiler.
const aggregatorShards = 16

// AggPolicyDef is the engine-level representation of an AggregationPolicy row.
// The first policy whose Matchers all match the alert wins; otherwise routing
// or built-in defaults are used.
type AggPolicyDef struct {
	Name      string
	Matchers  []LabelMatcher
	GroupBy   []string
	GroupWait time.Duration
}

type pendingGroup struct {
	group AlertGroup
	timer *time.Timer
}

// aggregatorShard is one bucket of the sharded mutex layout.  Each
// shard owns a disjoint set of group_keys (assigned by FNV-1a hash on
// the key string); contention only matters between goroutines hitting
// the same shard.
type aggregatorShard struct {
	mu     sync.Mutex
	groups map[string]*pendingGroup
}

// Aggregator collects alerts into groups based on groupBy labels and
// fires after group_wait.
//
// Concurrency model: the previous single-Mutex layout serialised every
// Add() call across all consumer goroutines, which capped the Kafka
// pipeline throughput at ~1/lockHoldTime per machine no matter how many
// consumer workers were configured.  Sharding by group_key (the only
// field Add uses for placement) lets N goroutines targeting different
// keys proceed in parallel while preserving per-key ordering (the same
// shard always serialises the same group_key).
type Aggregator struct {
	shards     [aggregatorShards]aggregatorShard
	groupWait  time.Duration
	policiesMu sync.RWMutex
	policies   []AggPolicyDef
}

func NewAggregator() *Aggregator {
	a := &Aggregator{groupWait: defaultGroupWait}
	for i := range a.shards {
		a.shards[i].groups = make(map[string]*pendingGroup)
	}
	return a
}

// shardFor maps a group_key to its owning shard.  computeGroupKey
// returns sha256 hex (uniformly distributed) so an FNV-1a hash on the
// hex string gives an even split across the 16 buckets.
func (a *Aggregator) shardFor(key string) *aggregatorShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &a.shards[h.Sum32()%uint32(aggregatorShards)]
}

// SetPolicies replaces the aggregation policies (called during init and hot-reload).
// In-flight pending groups are not affected; the new set applies to subsequent alerts.
func (a *Aggregator) SetPolicies(policies []AggPolicyDef) {
	a.policiesMu.Lock()
	a.policies = policies
	a.policiesMu.Unlock()
}

// resolve selects the (groupBy, groupWait) pair for an alert.
// Order: first matching aggregation policy → route group_by → built-in default.
func (a *Aggregator) resolve(alert ingestion.RawAlert, route *RouteDef) ([]string, time.Duration) {
	a.policiesMu.RLock()
	defer a.policiesMu.RUnlock()

	for _, p := range a.policies {
		if matchesAll(p.Matchers, alert.Labels) {
			gb := p.GroupBy
			if len(gb) == 0 {
				if route != nil && len(route.GroupBy) > 0 {
					gb = route.GroupBy
				} else {
					gb = []string{"alertname"}
				}
			}
			gw := p.GroupWait
			if gw <= 0 {
				gw = a.groupWait
			}
			return gb, gw
		}
	}

	if route != nil && len(route.GroupBy) > 0 {
		return route.GroupBy, a.groupWait
	}
	return []string{"alertname"}, a.groupWait
}

// Add inserts an alert into the appropriate group, creating a new group if needed.
func (a *Aggregator) Add(alert ingestion.RawAlert, route *RouteDef, callback func(AlertGroup)) {
	groupBy, groupWait := a.resolve(alert, route)
	key := computeGroupKey(alert.Labels, groupBy)
	shard := a.shardFor(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if pg, ok := shard.groups[key]; ok {
		pg.group.Alerts = append(pg.group.Alerts, alert)
		pg.group.Severity = maxSeverity(pg.group.Severity, alert.Labels["severity"])
		return
	}

	sev := mapSeverity(alert.Labels["severity"])
	group := AlertGroup{
		GroupKey:     key,
		Labels:       alert.Labels,
		Alerts:       []ingestion.RawAlert{alert},
		Severity:     sev,
		DataSourceID: alert.DataSourceID,
	}

	timer := time.AfterFunc(groupWait, func() {
		shard.mu.Lock()
		pg, ok := shard.groups[key]
		if ok {
			delete(shard.groups, key)
		}
		shard.mu.Unlock()
		if ok && callback != nil {
			callback(pg.group)
		}
	})

	shard.groups[key] = &pendingGroup{group: group, timer: timer}
}

func computeGroupKey(labels map[string]string, groupBy []string) string {
	parts := make([]string, 0, len(groupBy))
	sort.Strings(groupBy)
	for _, k := range groupBy {
		parts = append(parts, k+"="+labels[k])
	}
	h := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return fmt.Sprintf("%x", h[:8])
}

var severityRank = map[string]int{
	"P0": 4, "critical": 4,
	"P1": 3,
	"P2": 2, "warning": 2,
	"P3": 1, "info": 1,
}

func mapSeverity(s string) string {
	switch s {
	case "critical":
		return "P1"
	case "warning":
		return "P2"
	case "info":
		return "P3"
	default:
		if _, ok := severityRank[s]; ok {
			return s
		}
		return "P3"
	}
}

func maxSeverity(a, b string) string {
	ra := severityRank[a]
	rb := severityRank[mapSeverity(b)]
	if rb > ra {
		return mapSeverity(b)
	}
	return a
}
