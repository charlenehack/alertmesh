package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// pgChannel is the PostgreSQL NOTIFY channel the listener subscribes to.
// `incident.Service` writes to the same name via plain SQL `pg_notify` —
// see notifyIncidentEvent there.  Kept as a constant in this package so
// both ends share the canonical name.
const pgChannel = "incident_event"

// reconnectBackoff is how long we wait before re-establishing the LISTEN
// connection after an error.  Matches the AI orchestrator's setup so an
// unstable PG link behaves the same way across both subsystems.
const reconnectBackoff = 5 * time.Second

// notifyWait is the per-iteration WaitForNotification deadline.  Returning
// every 5 s gives ctx-cancellation a chance to fire even when the channel
// is silent.
const notifyWait = 5 * time.Second

// Start launches the LISTEN goroutine.  The function returns immediately;
// the goroutine runs until ctx is cancelled.  Each notification payload
// is decoded as a realtime.Event and broadcast to two topics:
//   - TopicAll                 — the firehose for list/dashboard pages
//   - TopicIncident(IncidentID) — the per-incident channel for detail page
//
// Failures (decode error, lost LISTEN socket) are logged at warn level and
// retried with a fixed backoff.  Crucially we DO NOT crash the process on
// listener loss — incidents are still written to the DB; the worst case
// is the UI falling back to its on-page user-driven Refresh button.
func Start(ctx context.Context, db *gorm.DB, hub *Hub) {
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := listen(ctx, db, hub); err != nil && !errors.Is(err, context.Canceled) {
				log.Warn().Err(err).Str("component", "realtime").Msg("LISTEN loop ended, will retry")
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectBackoff):
			}
		}
	}()
	log.Info().Str("component", "realtime").Str("channel", pgChannel).Msg("LISTEN goroutine started")
}

// listen runs one iteration of the LISTEN loop on a dedicated DB
// connection.  Returns when the connection drops or ctx is cancelled.
func listen(ctx context.Context, db *gorm.DB, hub *Hub) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "LISTEN "+pgChannel); err != nil {
		return err
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var payload string
		err := conn.Raw(func(driverConn any) error {
			// pgx is the only driver in the project (go.mod has
			// jackc/pgx/v5).  We type-assert to its stdlib
			// adapter so we can reach the rich pgx.Conn API and
			// get the notification payload — the bare pgconn
			// WaitForNotification doesn't return one.
			sc, ok := driverConn.(*stdlib.Conn)
			if !ok {
				// Defensive: wait briefly so we don't busy-spin
				// if the driver is ever swapped out.
				time.Sleep(2 * time.Second)
				return nil
			}

			waitCtx, cancel := context.WithTimeout(ctx, notifyWait)
			defer cancel()

			n, err := sc.Conn().WaitForNotification(waitCtx)
			if err != nil {
				return err
			}
			if n != nil {
				payload = n.Payload
			}
			return nil
		})

		if ctx.Err() != nil {
			return ctx.Err()
		}
		// notifyWait expiring is the steady-state path; everything
		// else (broken socket, EOF) bubbles up so listen()'s caller
		// can reconnect.
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return err
		}

		if payload == "" {
			continue
		}

		var ev Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			log.Warn().
				Err(err).
				Str("component", "realtime").
				Str("payload", payload).
				Msg("invalid pg_notify payload, dropping")
			continue
		}

		// Re-encode after unmarshal to normalise the JSON shape the
		// browser sees (omitempty stripping etc.) — slightly
		// wasteful but keeps the wire format fully under our control.
		out, err := ev.Marshal()
		if err != nil {
			continue
		}
		hub.Broadcast(TopicAll, out)
		if ev.IncidentID != "" {
			hub.Broadcast(TopicIncident(ev.IncidentID), out)
		}
	}
}

// isTimeout returns true for context.DeadlineExceeded and equivalent net
// timeout errors so the caller can distinguish "nothing happened in 5s"
// (continue) from "socket dead" (reconnect).
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
