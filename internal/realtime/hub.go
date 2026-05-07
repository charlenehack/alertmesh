// Package realtime provides a topic-based WebSocket pub/sub channel that
// replaces every HTTP polling timer the frontend used to keep itself in
// sync with the backend (incidents list, dashboard counters, incident
// detail).  The data flow is:
//
//	incident.Service mutation
//	  -> pg_notify('incident_event', json)        (cross-replica fan-out)
//	  -> realtime.Listener decodes payload
//	  -> realtime.Hub.Broadcast(topic, payload)
//	  -> writePump on each subscribed *Subscriber
//	  -> browser onmessage -> react-query invalidate -> single REST refetch
//
// We intentionally keep this hub independent from the existing ai.WSHub:
// the AI hub speaks a rich `analysis_*`/`tool_*` schema keyed on incident
// id only, while this one speaks a tiny `{type,incident_id}` envelope and
// supports arbitrary topic names so a single connection can multiplex
// `incidents` + `incident:<uuid>` subscriptions.
package realtime

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// sendBufferSize is the per-subscriber outbound channel depth.  When a
// websocket peer is too slow to drain, the channel fills and we drop the
// connection rather than block the broadcaster (see Broadcast below).
//
// 32 is comfortably above the natural rate of incident lifecycle events
// (a busy production system might emit a few per second) but small enough
// that a stuck client is detected within ~milliseconds of the next event.
const sendBufferSize = 32

// writeWait is the deadline for a single WebSocket write.  Keep it short
// — if the kernel can't drain a 100-byte frame in 10s the peer is gone.
const writeWait = 10 * time.Second

// pingPeriod is how often we send WebSocket pings to detect half-open
// connections (e.g. mobile network NAT timeouts).  Must be < pongWait.
const pingPeriod = 30 * time.Second

// pongWait is how long we'll wait for a pong before considering the
// connection dead.  Doubles as the read deadline (refreshed on each pong).
const pongWait = 60 * time.Second

// Subscriber owns a single WebSocket connection plus the set of topics it
// is subscribed to.  All writes happen on the writePump goroutine, so the
// underlying *websocket.Conn is only ever written to from one place,
// which is the gorilla docs' explicit safety requirement.
type Subscriber struct {
	conn   *websocket.Conn
	send   chan []byte
	topics []string
	hub    *Hub
}

// Hub is the topic-to-subscribers index.  RWMutex because broadcasts
// (RLock) vastly outnumber subscribe/unsubscribe (Lock) in steady state.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]map[*Subscriber]struct{}
}

// NewHub returns an empty hub ready to accept subscribers.
func NewHub() *Hub {
	return &Hub{
		topics: make(map[string]map[*Subscriber]struct{}),
	}
}

// Subscribe registers a connection against the given topics and returns a
// Subscriber whose Run loop the caller MUST invoke (synchronously) — that
// loop owns the conn for both the read pump (handles pings/pongs and
// detects close) and the write pump.
func (h *Hub) Subscribe(conn *websocket.Conn, topics []string) *Subscriber {
	sub := &Subscriber{
		conn:   conn,
		send:   make(chan []byte, sendBufferSize),
		topics: append([]string(nil), topics...),
		hub:    h,
	}

	h.mu.Lock()
	for _, t := range sub.topics {
		set, ok := h.topics[t]
		if !ok {
			set = make(map[*Subscriber]struct{})
			h.topics[t] = set
		}
		set[sub] = struct{}{}
	}
	h.mu.Unlock()

	return sub
}

// unsubscribe drops the subscriber from every topic it was registered for
// and closes its send channel so the writePump exits cleanly.  Called
// once from Subscriber.Run on either side closing.
func (h *Hub) unsubscribe(sub *Subscriber) {
	h.mu.Lock()
	for _, t := range sub.topics {
		if set, ok := h.topics[t]; ok {
			delete(set, sub)
			if len(set) == 0 {
				delete(h.topics, t)
			}
		}
	}
	h.mu.Unlock()

	// Closing send signals the writePump to exit.  Safe to do here
	// because we just removed sub from every topic set so no further
	// Broadcast can target it.
	close(sub.send)
}

// Broadcast fans out payload to every subscriber on the given topic.  It
// does NOT block: if a subscriber's send buffer is full we treat it as a
// stuck client and let the next pingPeriod cycle close it.  This is the
// crucial property that lets a single misbehaving browser tab not stall
// the entire pg_notify -> hub fan-out path.
func (h *Hub) Broadcast(topic string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	subs, ok := h.topics[topic]
	if !ok {
		return
	}
	for sub := range subs {
		select {
		case sub.send <- payload:
		default:
			log.Warn().
				Str("component", "realtime").
				Str("topic", topic).
				Msg("subscriber send buffer full, dropping event")
		}
	}
}

// SubscriberCount returns the number of distinct subscribers on a topic;
// exposed for tests and /metrics, never on the hot path.
func (h *Hub) SubscriberCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.topics[topic])
}

// Run takes ownership of the underlying connection until either side
// closes.  It launches the writePump in a child goroutine and runs the
// read pump on the calling goroutine — this matches the gorilla example
// pattern and lets the HTTP handler `defer conn.Close()` reliably.
func (s *Subscriber) Run() {
	defer s.hub.unsubscribe(s)

	go s.writePump()
	s.readPump()
}

// writePump consumes s.send and writes one frame per message.  Also
// emits periodic pings so a NAT'd connection that goes silent doesn't
// linger forever.
func (s *Subscriber) writePump() {
	pingTicker := time.NewTicker(pingPeriod)
	defer func() {
		pingTicker.Stop()
		_ = s.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-s.send:
			if !ok {
				// hub.unsubscribe was called — gracefully tell
				// the peer before the deferred Close fires.
				_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = s.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Debug().Err(err).Msg("realtime: write error, closing")
				return
			}
		case <-pingTicker.C:
			_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Debug().Err(err).Msg("realtime: ping error, closing")
				return
			}
		}
	}
}

// readPump exists almost entirely to (a) honour pongs so the read
// deadline keeps getting bumped and (b) detect peer-side close.  We
// don't expect the client to ever send application frames; any payload
// they push is silently discarded.
func (s *Subscriber) readPump() {
	_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		if _, _, err := s.conn.ReadMessage(); err != nil {
			return
		}
	}
}
