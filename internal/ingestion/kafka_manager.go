package ingestion

// KafkaManager owns one segmentio/kafka-go Reader per enabled
// `data_sources` row of kind=kafka.  Lifecycle:
//
//   Reload() reads the table → diffs against the running reader set →
//   stops removed/disabled rows, spawns new ones, and replaces rows whose
//   broker / topic / sasl / filter / mapping changed.  Reload() is invoked
//   on three triggers (in order of importance):
//
//     1. PG NOTIFY data_source_event   — sub-second hot-reload, fired by
//        every CRUD in router/data_sources.go.  This is the steady-state
//        path; "polling" is explicitly out per the user mandate.
//     2. 5-minute floor                — last-resort safety net for the
//        case where a NOTIFY is lost (e.g. listener was reconnecting
//        when CRUD happened).  This is NOT polling business data; we
//        only re-read the same registry table the listener already
//        watches.
//     3. process startup                — initial population.
//
// Per-reader goroutine: drains FetchMessage in a loop, hands the payload
// to the per-row KafkaProgram, rate-limits with golang.org/x/time/rate
// before forwarding to the engine pipeline, and CommitMessages so a
// restart resumes from the same offset.  A bad message (filter dropped,
// mapping rejected) is committed regardless — not committing would
// poison the consumer group with the same bad message forever.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	kafka "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/model"
	"github.com/kuzane/alertmesh/internal/realtime"
	"github.com/kuzane/alertmesh/pkg/metrics"
)

// DataSourceEventChannel is the PG NOTIFY channel router/data_sources.go
// publishes to after every CRUD.  Constants live here so adding a third
// listener (e.g. an OpenSearch poller in Phase 4) can subscribe by
// importing this package rather than re-discovering the string.
const DataSourceEventChannel = "data_source_event"

// reloadDebounce coalesces a burst of CRUDs (e.g. operator clicking
// "save" five times in a row) into a single Reload() call.  Picked to be
// short enough that humans don't notice the lag and long enough to
// absorb typical UI re-saves.
const reloadDebounce = 500 * time.Millisecond

// reloadFloor is the 5-minute safety net described in the file header.
const reloadFloor = 5 * time.Minute

// statsTickInterval is how often each reader's Lag gauge is refreshed.
// kafka-go computes Stats on every call (deltas + reset) so we keep the
// cadence loose to avoid masking the lag spike with our own sampling.
const statsTickInterval = 30 * time.Second

// maxConsumerConcurrency caps the per-row Reader fan-out.  The router
// validation layer rejects values above this, but we keep the constant
// here as defence-in-depth for stale jsonb rows that bypassed the API.
// 32 covers the largest partition counts we see on prod Higress / log
// gateway topics; going above is purely waste because broker-side
// partition assignment will leave the surplus workers idle.
const maxConsumerConcurrency = 32

// processConcurrency is how many filter/pipeline goroutines run *behind*
// each Reader.  Decoupled from consumer_concurrency because it doesn't
// share the broker-side partition cap: filter+mapping+engine.Process are
// pure CPU/lock-bound work that the Reader's single FetchMessage stream
// can comfortably feed.  8 is a sweet spot for typical Higress access-log
// payloads (few-KB JSON, mapping is pure gjson paths) without saturating
// the engine's sharded aggregator.
const processConcurrency = 8

// fetchChannelDepth is the bounded fetcher → processors hand-off depth
// per Reader.  Keep it modest so a stuck pipeline back-pressures the
// Reader (which in turn back-pressures the broker via the fetch session)
// instead of accumulating an unbounded in-memory queue.  Memory ceiling
// per Reader = depth × max kafka message size — at 32 × 10MiB worst case
// it's 320MiB, but real Higress payloads are kilobytes so the realistic
// ceiling is in the low MiB.
const fetchChannelDepth = processConcurrency * 4

// commitInterval is how often the per-Reader committer flushes acked
// offsets to the broker.  250ms is the same trade-off Sarama uses by
// default — short enough that a graceful shutdown loses at most one tick
// of in-flight progress, long enough to coalesce hundreds of acks into
// one CommitMessages round-trip on busy topics.
const commitInterval = 250 * time.Millisecond

// KafkaManager is the long-lived owner of all per-row Reader groups.
// Methods are safe to call concurrently — internal state is mu-protected.
type KafkaManager struct {
	db       *gorm.DB
	cfg      *config.Config
	pipeline func(RawAlert)

	mu     sync.Mutex
	groups map[string]*readerGroup // keyed by data_sources.id

	// reloadCh is the debouncer's request side.  ListenLoop fires
	// whenever a NOTIFY arrives; the debouncer collapses bursts and
	// then calls Reload().
	reloadCh chan struct{}
}

// readerGroup is the manager's bookkeeping for a single data_sources row:
// configHash + the slice of running workers.  When N is raised/lowered the
// whole group is torn down and respawned (configHash includes N), keeping
// Reload() diff logic identical to the single-reader era.
type readerGroup struct {
	dsID       string
	dsName     string
	configHash string
	workers    []*kafkaWorker
}

// kafkaWorker is one Reader plus its three-stage pipeline:
//
//	fetcher (1 goroutine)        — pulls messages off the wire and
//	                                hands them to the processor pool
//	                                via a bounded channel.
//	processors (M goroutines)    — run filter/mapping/engine.Process,
//	                                then hand the (partition,offset)
//	                                pair to the per-partition tracker.
//	committer (1 goroutine)      — drains the tracker on a ticker and
//	                                calls CommitMessages with the highest
//	                                contiguous offset per partition.
//
// Decoupling fetch from process lets us actually use the M goroutines
// when the topic has fewer partitions than consumer_concurrency, which
// is the common case on alertmesh's high-volume access-log topics.
//
// Multiple workers per readerGroup still share the same GroupID so the
// broker auto-distributes partitions across the workers; each worker
// owns its own FetchMessage / processor pool / CommitMessages cycle, so
// per-partition offset advancement remains linear (the partitionTracker
// enforces this even when processors finish out of order — see
// partition_tracker.go).
type kafkaWorker struct {
	index   int // 0..N-1, mainly for log grep / leak diagnosis
	cancel  context.CancelFunc
	done    chan struct{}
	reader  *kafka.Reader
	tracker *partitionTracker
}

// NewKafkaManager wires the dependencies but does NOT start any
// goroutines — call Start() on the returned manager so main.go controls
// the root context lifetime.
func NewKafkaManager(db *gorm.DB, cfg *config.Config, pipeline func(RawAlert)) *KafkaManager {
	return &KafkaManager{
		db:       db,
		cfg:      cfg,
		pipeline: pipeline,
		groups:   map[string]*readerGroup{},
		reloadCh: make(chan struct{}, 1),
	}
}

// Start kicks off the manager's background goroutines:
//   - debouncer + Reload loop          (one)
//   - PG LISTEN goroutine               (one)
//   - per-row Reader workers            (R rows × N consumer_concurrency,
//                                        each running a fetcher + M
//                                        processor goroutines + a
//                                        committer + a stats sampler;
//                                        total goroutines per Reader
//                                        is M+3, see runWorker).
//
// Returns immediately.  All goroutines drain on ctx cancellation.
func (m *KafkaManager) Start(ctx context.Context) {
	go m.runDebouncer(ctx)
	realtime.ListenLoop(ctx, m.db, DataSourceEventChannel, func(payload string) {
		log.Debug().Str("component", "kafka").Str("payload", payload).Msg("data_source_event received")
		m.requestReload()
	})
	// Initial population — no need to debounce; just block long enough
	// for the first read to land before main returns.
	m.requestReload()
}

func (m *KafkaManager) requestReload() {
	select {
	case m.reloadCh <- struct{}{}:
	default:
		// A reload is already pending; one debounced run will pick up
		// any state we'd have requested anyway.
	}
}

func (m *KafkaManager) runDebouncer(ctx context.Context) {
	floor := time.NewTicker(reloadFloor)
	defer floor.Stop()
	for {
		select {
		case <-ctx.Done():
			m.shutdownAll()
			return
		case <-floor.C:
			m.Reload(ctx)
		case <-m.reloadCh:
			// Wait out the debounce window in case more requests
			// land in quick succession.
			t := time.NewTimer(reloadDebounce)
			drained := false
			for !drained {
				select {
				case <-t.C:
					drained = true
				case <-m.reloadCh:
					if !t.Stop() {
						<-t.C
					}
					t.Reset(reloadDebounce)
				case <-ctx.Done():
					if !t.Stop() {
						<-t.C
					}
					m.shutdownAll()
					return
				}
			}
			m.Reload(ctx)
		}
	}
}

// Reload performs one full diff between the DB rows and the running set.
// Exposed (rather than kept package-private) so tests / debug endpoints
// can force an immediate reconcile.
func (m *KafkaManager) Reload(ctx context.Context) {
	rows, err := m.loadRows(ctx)
	if err != nil {
		log.Warn().Err(err).Str("component", "kafka").Msg("kafka manager: failed to load data_sources, keeping previous state")
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	want := map[string]struct{}{}

	for _, row := range rows {
		want[row.ID] = struct{}{}
		hash := configHash(row)
		existing, ok := m.groups[row.ID]
		if ok && existing.configHash == hash {
			continue
		}
		if ok {
			log.Info().Str("component", "kafka").Str("ds", row.Name).Msg("kafka reader config changed, restarting group")
			m.stopGroupLocked(existing)
		}
		// Spawn fresh group of N workers.
		grp, err := m.spawnReaderGroup(ctx, row, hash)
		if err != nil {
			log.Warn().Err(err).Str("component", "kafka").Str("ds", row.Name).Msg("kafka reader group spawn failed; will retry on next reload")
			continue
		}
		m.groups[row.ID] = grp
	}

	for id, grp := range m.groups {
		if _, keep := want[id]; keep {
			continue
		}
		log.Info().Str("component", "kafka").Str("ds", grp.dsName).Int("workers", len(grp.workers)).Msg("kafka reader group stopped (row removed or disabled)")
		m.stopGroupLocked(grp)
		delete(m.groups, id)
	}
}

func (m *KafkaManager) shutdownAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, grp := range m.groups {
		m.stopGroupLocked(grp)
		delete(m.groups, id)
	}
}

// stopGroupLocked tears the whole readerGroup down in parallel.
//
// Order matters here: cancelling the worker context unblocks the
// fetcher's FetchMessage call, which cascades through the
// fetcher/processors/committer pipeline (see runWorker for the drain
// sequence).  We wait on `done` BEFORE closing the underlying
// kafka.Reader so the final flushCommits pass in runWorker can still
// reach the broker — closing the reader first would leave that last
// batch of acks uncommitted, forcing the consumer-group to re-deliver
// them after restart (deduper tolerates this, but it's wasted work
// every redeploy).
//
// The 5s timeout is the upstream cap: if a hung TCP keeps a worker
// from draining, we log + leak rather than block other workers'
// shutdown indefinitely.  Bumped from 2s because the new pipeline
// includes a final commit RTT in its drain path.
func (m *KafkaManager) stopGroupLocked(grp *readerGroup) {
	var wg sync.WaitGroup
	for _, w := range grp.workers {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.cancel()
			select {
			case <-w.done:
			case <-time.After(5 * time.Second):
				log.Warn().Str("component", "kafka").Str("ds", grp.dsName).Int("worker_index", w.index).Msg("kafka reader did not shut down within 5s, leaking")
			}
			if w.reader != nil {
				_ = w.reader.Close()
			}
		}()
	}
	wg.Wait()
}

func (m *KafkaManager) loadRows(ctx context.Context) ([]model.DataSource, error) {
	var rows []model.DataSource
	err := m.db.WithContext(ctx).
		Where("kind = ? AND is_enabled = ?", model.DataSourceKindKafka, true).
		Find(&rows).Error
	return rows, err
}

// configHash is the cheap "did anything that affects the consumer
// change?" signature.  Includes everything FetchMessage / filter /
// mapping / consumer_concurrency needs; deliberately excludes display
// fields like description so cosmetic edits don't bounce a healthy
// reader.  consumer_concurrency lives inside row.Config so it is
// already covered by the json marshal — we keep it explicitly noted in
// the struct shape so future readers don't accidentally strip Config.
func configHash(row model.DataSource) string {
	type signature struct {
		Endpoint string
		Config   string
	}
	sig := signature{Endpoint: row.Endpoint, Config: string(row.Config)}
	b, _ := json.Marshal(sig)
	return string(b)
}

// resolveConsumerConcurrency reads `consumer_concurrency` from the per-row
// jsonb config.  Defaults to 1 (legacy single-Reader behaviour) when absent
// or out of range; clamps to the [1, maxConsumerConcurrency] band so
// stale rows that bypassed validation can't blow up the goroutine pool.
func resolveConsumerConcurrency(cfg map[string]any) int {
	n := int(asFloat(cfg["consumer_concurrency"]))
	if n < 1 {
		return 1
	}
	if n > maxConsumerConcurrency {
		return maxConsumerConcurrency
	}
	return n
}

// spawnReaderGroup compiles the per-row filter/mapping once, then spawns N
// independent kafka.Reader instances all sharing the same GroupID.  The
// broker's consumer-group protocol distributes partitions across the N
// workers automatically — same model the Kafka docs recommend over
// "1 Reader + N FetchMessage goroutines" because it preserves per-partition
// commit order.  N comes from `consumer_concurrency` (default 1, max 32).
//
// Partial-success contract: if the i-th worker fails to construct, every
// already-spawned worker in this batch is rolled back so the data source
// either has N healthy workers or none at all.  Reload() will retry on the
// next NOTIFY / 5-minute floor.
func (m *KafkaManager) spawnReaderGroup(parent context.Context, row model.DataSource, hash string) (*readerGroup, error) {
	cfgMap := jsonToMapBytes(row.Config)

	prog, err := CompileKafkaProgram(KafkaFilterConfig{
		Filter:  asStringMap(cfgMap, "filter"),
		Mapping: extractMapping(cfgMap),
	})
	if err != nil {
		return nil, fmt.Errorf("compile filter/mapping: %w", err)
	}

	brokers := splitBrokers(row.Endpoint)
	if len(brokers) == 0 {
		return nil, errors.New("endpoint has no brokers")
	}
	topic := asStringMap(cfgMap, "topic")
	if topic == "" {
		return nil, errors.New("topic is required")
	}
	groupID := asStringMap(cfgMap, "group_id")
	if groupID == "" {
		return nil, errors.New("group_id is required")
	}

	dialer, err := m.buildDialer(row, cfgMap)
	if err != nil {
		return nil, fmt.Errorf("build dialer: %w", err)
	}

	concurrency := resolveConsumerConcurrency(cfgMap)
	maxPerSec := asFloat(cfgMap["max_per_second"])

	grp := &readerGroup{
		dsID:       row.ID,
		dsName:     row.Name,
		configHash: hash,
		workers:    make([]*kafkaWorker, 0, concurrency),
	}

	for i := 0; i < concurrency; i++ {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			GroupID: groupID,
			Topic:   topic,
			Dialer:  dialer,
			// MinBytes/MaxWait control fetch batching: wait up to
			// MaxWait for at least MinBytes to accumulate, then ship.
			// MinBytes:1 (the old default) caused one fetch per
			// message; bumping to 10 KiB lets the broker batch
			// hundreds of access-log lines per round-trip.
			MinBytes:    10 << 10, // 10 KiB
			MaxBytes:    10 << 20, // 10 MiB
			MaxWait:     200 * time.Millisecond,
			StartOffset: kafka.LastOffset,
			// CommitInterval=0 means manual commit only — our
			// committer goroutine batches per commitInterval tick;
			// letting kafka-go also auto-commit would race with the
			// tracker's contiguous-offset bookkeeping.
			CommitInterval: 0,
		})

		ctx, cancel := context.WithCancel(parent)
		w := &kafkaWorker{
			index:   i,
			cancel:  cancel,
			done:    make(chan struct{}),
			reader:  reader,
			tracker: newPartitionTracker(),
		}

		// Each worker owns its own limiter — sharing one limiter across
		// N workers would serialise the throughput we're trying to
		// expand.  Operators set max_per_second as a per-worker budget;
		// within a worker the M processor goroutines also share the
		// same limiter so the per-Reader budget stays correct.
		limiter := buildLimiter(maxPerSec)

		go m.runWorker(ctx, grp, w, prog, limiter)
		go m.statsLoop(ctx, grp, w)

		grp.workers = append(grp.workers, w)

		log.Info().
			Str("component", "kafka").
			Str("ds", row.Name).
			Str("topic", topic).
			Str("group_id", groupID).
			Strs("brokers", brokers).
			Int("worker_index", i).
			Int("concurrency", concurrency).
			Int("processors_per_reader", processConcurrency).
			Int("total_processor_goroutines", concurrency*processConcurrency).
			Msg("kafka reader spawned")
	}

	return grp, nil
}

// runWorker is the per-Reader supervisor.  It launches the three-stage
// pipeline (fetcher / M processors / committer), waits for them to drain
// in the correct order on shutdown, and then closes w.done so
// stopGroupLocked can move on.
//
// Drain order on shutdown matters for at-least-once semantics:
//
//  1. ctx cancellation interrupts FetchMessage in the fetcher.
//  2. Fetcher closes fetchCh as it exits — telling processors that no
//     more work is coming.
//  3. Processors finish whatever's already in fetchCh, then exit; each
//     ack lands in the tracker.
//  4. Once all processors have returned, we make ONE final synchronous
//     commit pass via the tracker.  This is critical: messages
//     processed in the tail of the run that were not yet swept by the
//     250ms committer tick would otherwise be re-delivered after
//     restart.  Dedup tolerates that, but we'd rather not pay the
//     duplicate-processing cost on every redeploy.
//  5. Committer goroutine returns (it was racing with the final pass
//     but the tracker uses a mutex so the race is safe).
//
// Concurrency note: prog (KafkaProgram) is built from compiled expr
// programs that segmentio/expr-lang explicitly documents as goroutine-safe
// for evaluation; pipeline (engine.Process) is goroutine-safe via the
// upstream Pipeline.mu RWMutex.  Sharing them across processCount × N
// goroutines is fine.
func (m *KafkaManager) runWorker(ctx context.Context, grp *readerGroup, w *kafkaWorker, prog *KafkaProgram, limiter *rate.Limiter) {
	defer close(w.done)

	fetchCh := make(chan kafka.Message, fetchChannelDepth)

	var fetcherWG sync.WaitGroup
	fetcherWG.Add(1)
	go func() {
		defer fetcherWG.Done()
		defer close(fetchCh)
		m.fetcherLoop(ctx, grp, w, fetchCh)
	}()

	var processorsWG sync.WaitGroup
	for p := 0; p < processConcurrency; p++ {
		processorsWG.Add(1)
		go func(processorIndex int) {
			defer processorsWG.Done()
			m.processorLoop(ctx, grp, w, prog, limiter, fetchCh, processorIndex)
		}(p)
	}

	committerStop := make(chan struct{})
	var committerWG sync.WaitGroup
	committerWG.Add(1)
	go func() {
		defer committerWG.Done()
		m.committerLoop(grp, w, committerStop)
	}()

	// Wait for fetcher first — its exit closes fetchCh which is the
	// processors' termination signal.
	fetcherWG.Wait()
	processorsWG.Wait()

	// Final synchronous commit pass before the committer exits, so any
	// acks that landed in the tracker after the committer's last tick
	// still get persisted.  Use a fresh background context so we can
	// commit even when the parent ctx is already cancelled (the broker
	// connection is still alive — Reader.Close() comes later).
	m.flushCommits(grp, w)

	close(committerStop)
	committerWG.Wait()
}

// fetcherLoop is the only goroutine that calls FetchMessage on this
// Reader.  Centralising fetch here means we can rely on kafka-go's
// internal cursor without inventing our own concurrency story for it,
// and it means a hung pipeline simply blocks on `fetchCh <-` instead of
// triggering rebalance churn.
func (m *KafkaManager) fetcherLoop(ctx context.Context, grp *readerGroup, w *kafkaWorker, fetchCh chan<- kafka.Message) {
	dsLabel := grp.dsName
	for {
		if ctx.Err() != nil {
			return
		}
		msg, err := w.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Warn().Err(err).Str("component", "kafka").Str("ds", dsLabel).Int("worker_index", w.index).Msg("FetchMessage failed")
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		metrics.KafkaMessagesReceived.WithLabelValues(dsLabel).Inc()
		select {
		case fetchCh <- msg:
		case <-ctx.Done():
			return
		}
	}
}

// processorLoop is the per-processor hot path.  M of these run behind
// each Reader, all draining the same fetchCh.  Each call to tracker.Ack
// is what eventually unblocks the committer's per-partition contiguous
// offset advance.
//
// Crucially: every code path acks (filter drop, mapping error, real
// process) — failing to ack would stall the partition's HWM forever and
// the consumer would appear to be progressing while never committing.
func (m *KafkaManager) processorLoop(ctx context.Context, grp *readerGroup, w *kafkaWorker, prog *KafkaProgram, limiter *rate.Limiter, fetchCh <-chan kafka.Message, processorIndex int) {
	dsLabel := grp.dsName
	for msg := range fetchCh {
		started := time.Now()
		res, applyErr := prog.ApplyForConsumer(msg.Value, "kafka", grp.dsID)
		if applyErr != nil {
			log.Warn().Err(applyErr).Str("component", "kafka").Str("ds", dsLabel).Int("worker_index", w.index).Int("processor_index", processorIndex).Msg("filter/mapping runtime error, dropping message")
			metrics.KafkaMessagesDropped.WithLabelValues(dsLabel, "filter_error").Inc()
			w.tracker.Ack(msg)
			metrics.KafkaProcessLatency.WithLabelValues(dsLabel, "filter_error").Observe(time.Since(started).Seconds())
			continue
		}
		if !res.Keep {
			reason := dropReason(res.Reason)
			metrics.KafkaMessagesDropped.WithLabelValues(dsLabel, reason).Inc()
			w.tracker.Ack(msg)
			metrics.KafkaProcessLatency.WithLabelValues(dsLabel, reason).Observe(time.Since(started).Seconds())
			continue
		}

		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				// Limiter.Wait only errs on ctx cancellation.
				// During shutdown we deliberately bypass the rate
				// limit and still process the message: the
				// pipeline cost is bounded, the tracker.Ack +
				// final flushCommits will commit what we did, and
				// the processor will exit naturally when fetchCh
				// closes.  The alternative — return early without
				// processing — would force re-delivery of every
				// in-flight message on next start, which dedup
				// tolerates but is wasted CPU we can avoid.
				_ = err
			}
		}

		if m.pipeline != nil {
			m.pipeline(res.Alert)
		}
		w.tracker.Ack(msg)
		metrics.KafkaProcessLatency.WithLabelValues(dsLabel, "ok").Observe(time.Since(started).Seconds())
	}
}

// committerLoop ticks every commitInterval and pushes the tracker's
// contiguous high-water marks to the broker.  Exits when stop is
// closed; the runWorker drain path closes stop only after the final
// flushCommits pass, so this loop never observes stop while there's
// still pending work.
func (m *KafkaManager) committerLoop(grp *readerGroup, w *kafkaWorker, stop <-chan struct{}) {
	t := time.NewTicker(commitInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			m.flushCommits(grp, w)
		}
	}
}

// flushCommits pulls every per-partition committable offset out of the
// tracker and ships it in a single CommitMessages call.  Used both by
// the periodic committer tick and by the runWorker shutdown drain.
//
// Uses a short-deadline context derived from context.Background() rather
// than the worker context: we want to commit even when shutdown is in
// progress (the Reader's TCP connection is still alive — stopGroupLocked
// waits for runWorker to return before closing it), but we also don't
// want a hung broker to keep the process from exiting.  A real network
// failure here is retried by kafka-go internally; a hard EOF
// (Reader.Close already called by us during teardown) returns
// immediately and is logged.
func (m *KafkaManager) flushCommits(grp *readerGroup, w *kafkaWorker) {
	msgs := w.tracker.DrainCommittable()
	if len(msgs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.reader.CommitMessages(ctx, msgs...); err != nil && !errors.Is(err, context.Canceled) {
		log.Warn().Err(err).Str("component", "kafka").Str("ds", grp.dsName).Int("worker_index", w.index).Int("commit_count", len(msgs)).Msg("CommitMessages failed")
	}
}

// statsLoop publishes per-(datasource, partition) lag.  Within a single
// readerGroup the broker assigns each partition to exactly one worker, so
// the {datasource, partition} label tuple stays unique across workers and
// no scraper-side merge is needed.
func (m *KafkaManager) statsLoop(ctx context.Context, grp *readerGroup, w *kafkaWorker) {
	t := time.NewTicker(statsTickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s := w.reader.Stats()
			metrics.KafkaConsumerLag.WithLabelValues(grp.dsName, s.Partition).Set(float64(s.Lag))
		}
	}
}

// buildDialer assembles the kafka-go Dialer with the SASL + TLS knobs
// the registry row stores.  Decryption of the SASL password lives here so
// secrets never escape the manager package — the rest of the codebase
// only sees the resolved sasl.Mechanism.
func (m *KafkaManager) buildDialer(row model.DataSource, cfg map[string]any) (*kafka.Dialer, error) {
	d := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
	}

	if asBool(cfg["tls_enabled"]) {
		d.TLS = &tls.Config{
			InsecureSkipVerify: asBool(cfg["tls_insecure_skip_verify"]), //nolint:gosec
		}
	}

	mech := strings.TrimSpace(asStringMap(cfg, "sasl_mechanism"))
	if mech == "" {
		return d, nil
	}

	user := strings.TrimSpace(asStringMap(cfg, "sasl_user"))
	password, err := m.decryptSASLPassword(row.SecretEnc)
	if err != nil {
		return nil, err
	}
	if user == "" || password == "" {
		return nil, fmt.Errorf("sasl_mechanism=%s requires sasl_user and sasl_password", mech)
	}

	var saslMech sasl.Mechanism
	switch strings.ToUpper(mech) {
	case "PLAIN":
		saslMech = plain.Mechanism{Username: user, Password: password}
	case "SCRAM-SHA-256":
		saslMech, err = scram.Mechanism(scram.SHA256, user, password)
		if err != nil {
			return nil, fmt.Errorf("scram-sha-256: %w", err)
		}
	case "SCRAM-SHA-512":
		saslMech, err = scram.Mechanism(scram.SHA512, user, password)
		if err != nil {
			return nil, fmt.Errorf("scram-sha-512: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported sasl_mechanism %q", mech)
	}
	d.SASLMechanism = saslMech
	return d, nil
}

// decryptSASLPassword mirrors router/data_sources.go::decryptSecrets so
// the manager doesn't have to reach into the router package.  Same
// "encrypted ↔ plaintext fallback" semantics keep dev (no encryption
// key) and prod (AES-GCM ciphertext) round-tripping cleanly.
func (m *KafkaManager) decryptSASLPassword(stored string) (string, error) { //nolint:unparam // error reserved for future strict-decrypt mode
	if stored == "" {
		return "", nil
	}
	plaintext := stored
	if m.cfg != nil && m.cfg.EncryptionKey != "" {
		if dec, err := config.Decrypt(stored, m.cfg.EncryptionKey); err == nil {
			plaintext = dec
		}
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(plaintext), &out); err != nil {
		// Plain-string fallback — older rows that stored the password
		// directly without the JSON envelope.
		return strings.TrimSpace(plaintext), nil //nolint:nilerr
	}
	return strings.TrimSpace(out["sasl_password"]), nil
}

// dropReason maps the soft-drop strings returned by KafkaProgram.Apply
// onto a constrained metric label set so a typo in the engine doesn't
// quietly explode the cardinality.
func dropReason(s string) string {
	switch s {
	case "filter_false", "missing_alertname", "missing_severity", "bad_json", "filter_error":
		return s
	default:
		return "other"
	}
}

func splitBrokers(endpoint string) []string {
	parts := strings.Split(endpoint, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func buildLimiter(perSecond float64) *rate.Limiter {
	if perSecond <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(perSecond), int(perSecond)+1)
}

func extractMapping(cfg map[string]any) KafkaMapping {
	raw, _ := cfg["mapping"].(map[string]any)
	if raw == nil {
		return KafkaMapping{}
	}
	out := KafkaMapping{
		Alertname:    asStringMap(raw, "alertname"),
		Severity:     asStringMap(raw, "severity"),
		Fingerprint:  asStringMap(raw, "fingerprint"),
		StartsAt:     asStringMap(raw, "starts_at"),
		EndsAt:       asStringMap(raw, "ends_at"),
		Summary:      asStringMap(raw, "summary"),
		Description:  asStringMap(raw, "description"),
		StatusPath:   asStringMap(raw, "status_path"),
		ResolvedWhen: asStringMap(raw, "resolved_when"),
		Labels:       asStringStringMap(raw["labels"]),
		Annotations:  asStringStringMap(raw["annotations"]),
	}
	return out
}

func asStringMap(raw map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	s, _ := raw[key].(string)
	return strings.TrimSpace(s)
}

func asStringStringMap(v any) map[string]string {
	out := map[string]string{}
	m, ok := v.(map[string]any)
	if !ok {
		return out
	}
	for k, val := range m {
		if s, ok := val.(string); ok && s != "" {
			out[k] = s
		}
	}
	return out
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	}
	return 0
}

func jsonToMapBytes(b []byte) map[string]any {
	if len(b) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// Static net.Dialer reference kept here so go vet doesn't complain about
// the unused import once the build inevitably reorders things during
// future edits.  Cheap, no behaviour.
var _ = (&net.Dialer{Timeout: time.Second}).DialContext
