package realtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ListenLoop is a small generic helper that owns a long-lived PostgreSQL
// connection LISTENing on a single channel and invokes onPayload for each
// notification.  The function blocks until ctx is cancelled and
// transparently reconnects on error with a fixed 5s backoff.
//
// Two callers today:
//
//   - realtime.Start  — fans incident_event into the WS hub.
//   - ingestion.KafkaManager — coalesces data_source_event into a
//     debounced Reload().
//
// Both share the same connection-loss / reconnect / context-cancel
// semantics so we don't reinvent the wait-for-notification dance per
// subsystem.  Errors inside onPayload are NOT fatal — they're just
// logged at warn level so a single malformed notification can't kill
// the listener.
//
// Implementation note: the project pins pgx v5 via gorm's postgres
// driver, so we can safely type-assert the driver conn to pgx's stdlib
// wrapper.  An unsupported driver returns a hard error rather than
// silently degrading to a polling loop (see internal/engine/pipeline.go's
// listenReload comment for why that matters).
func ListenLoop(
	ctx context.Context,
	db *gorm.DB,
	channel string,
	onPayload func(payload string),
) {
	logger := log.With().Str("component", "pglisten").Str("channel", channel).Logger()
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := listenOnce(ctx, db, channel, onPayload); err != nil && !errors.Is(err, context.Canceled) {
				logger.Warn().Err(err).Msg("LISTEN loop ended, will retry")
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectBackoff):
			}
		}
	}()
	logger.Info().Msg("LISTEN goroutine started")
}

func listenOnce(
	ctx context.Context,
	db *gorm.DB,
	channel string,
	onPayload func(payload string),
) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "LISTEN "+channel); err != nil {
		return err
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var notif *pgconn.Notification
		err := conn.Raw(func(driverConn any) error {
			type pgxConner interface{ Conn() *pgx.Conn }
			pc, ok := driverConn.(pgxConner)
			if !ok {
				return fmt.Errorf("unsupported db driver conn %T: pglisten requires pgx", driverConn)
			}
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			n, werr := pc.Conn().WaitForNotification(waitCtx)
			if werr != nil {
				return werr
			}
			notif = n
			return nil
		})

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			continue
		}
		if err != nil {
			return err
		}

		payload := ""
		if notif != nil {
			payload = notif.Payload
		}
		onPayload(payload)
	}
}
