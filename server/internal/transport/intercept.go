package transport

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Interception tuning (calibration — tune freely). radius: an enemy sentry watching
// a hex within this many hexes of a caravan's current position seizes it. interval:
// ticks between scans.
const (
	interceptRadius            = 2
	interceptScanIntervalTicks = 1
)

// Notifier pushes a notification to a player. *notify.Hub satisfies it.
type Notifier interface {
	NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error
}

// InterceptScanHandler is the recurring sweep that seizes trade/transfer caravans
// passing within reach of an enemy sentry (movement-motor Slice C / War & Diplomacy
// Fas X). It reads ONLY the transports table — messengers are sacred and never
// scanned, so they can never be intercepted (only the gods may touch them).
type InterceptScanHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	notifier   Notifier
	clk        clock.Clock
}

// NewInterceptScanHandler creates an InterceptScanHandler.
func NewInterceptScanHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, notifier Notifier, clk clock.Clock) *InterceptScanHandler {
	return &InterceptScanHandler{pool: pool, scheduler: sched, eventStore: store, notifier: notifier, clk: clk}
}

type inFlightTransport struct {
	id       uuid.UUID
	owner    uuid.UUID
	originQ  int
	originR  int
	destQ    int
	destR    int
	category string
	departs  time.Time
	arrives  time.Time
}

// Handle scans every in-transit interceptable caravan once, seizing any caught by
// an enemy sentry, then re-enqueues itself.
func (h *InterceptScanHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	now := h.clk.Now()

	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, origin_q, origin_r, dest_q, dest_r, category, departs_at, arrives_at
		 FROM transports
		 WHERE world_id = $1 AND status = 'in_transit' AND interceptable = true`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("intercept scan: load transports: %w", err)
	}
	var fleet []inFlightTransport
	for rows.Next() {
		var t inFlightTransport
		if scanErr := rows.Scan(&t.id, &t.owner, &t.originQ, &t.originR, &t.destQ, &t.destR,
			&t.category, &t.departs, &t.arrives); scanErr != nil {
			rows.Close()
			return fmt.Errorf("intercept scan: scan transport: %w", scanErr)
		}
		fleet = append(fleet, t)
	}
	rows.Close()

	for _, t := range fleet {
		pos, ok, posErr := province.InterpolatePosition(ctx, h.pool, e.WorldID,
			province.MapPosition{Q: t.originQ, R: t.originR},
			province.MapPosition{Q: t.destQ, R: t.destR},
			t.category, t.departs, t.arrives, now)
		if posErr != nil || !ok {
			// Route no longer resolvable (e.g. a sea leg under the land-only category
			// default) — cannot place the caravan, so it cannot be intercepted yet.
			continue
		}

		// An enemy sentry watching within reach of the caravan's current hex.
		var sentryID, interceptor uuid.UUID
		if qErr := h.pool.QueryRow(ctx,
			`SELECT id, owner_id FROM units
			 WHERE world_id = $1 AND owner_id <> $2 AND status = 'positioned' AND stance = 'sentry'
			   AND sentry_q IS NOT NULL AND sentry_r IS NOT NULL
			   AND (ABS(sentry_q - $3) + ABS(sentry_r - $4) + ABS((sentry_q + sentry_r) - ($3 + $4))) / 2 <= $5
			 ORDER BY size DESC
			 LIMIT 1`,
			e.WorldID, t.owner, pos.Q, pos.R, interceptRadius,
		).Scan(&sentryID, &interceptor); qErr != nil {
			continue // no sentry in reach
		}

		if err := h.seize(ctx, e.WorldID, t, sentryID, interceptor, pos); err != nil {
			slog.Error("intercept scan: seize failed", "transport", t.id, "err", err)
		}
	}

	// Re-enqueue the next sweep.
	return h.scheduler.EnqueueTick(ctx, e.WorldID, events.ScheduledInterceptScan,
		struct{}{}, e.DueTick+interceptScanIntervalTicks)
}

// seize marks the caravan intercepted and moves its manifest to the interceptor's
// capital. Guarded so a caravan is seized at most once even under concurrent scans.
func (h *InterceptScanHandler) seize(ctx context.Context, worldID uuid.UUID, t inFlightTransport, sentryID, interceptor uuid.UUID, pos province.MapPosition) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`UPDATE transports SET status = 'intercepted', updated_at = now()
		 WHERE id = $1 AND status = 'in_transit'`, t.id)
	if err != nil {
		return fmt.Errorf("flip intercepted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil // already delivered/intercepted by a racing sweep
	}

	// The raider hauls the cargo home to their capital. No capital (rare) → the goods
	// are lost to the raid rather than credited.
	var capital *uuid.UUID
	_ = tx.QueryRow(ctx,
		`SELECT id FROM settlements WHERE owner_id = $1 AND world_id = $2 AND is_capital = true LIMIT 1`,
		interceptor, worldID,
	).Scan(&capital)
	if capital != nil {
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 SELECT $1, tg.good_key, tg.quantity, 0, 1000000, current_world_tick()
			 FROM transport_goods tg WHERE tg.transport_id = $2
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount = LEAST(
			         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick)
			             + EXCLUDED.amount,
			         settlement_goods.cap),
			     calc_tick = current_world_tick()`,
			*capital, t.id,
		); err != nil {
			return fmt.Errorf("credit loot: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Audit + notify both Wanax: the raider (seized) and the victim (raided). Async
	// play → the victim is likely offline when the raid lands, so the notice matters.
	if h.eventStore != nil {
		_, _ = h.eventStore.Append(ctx, t.id, events.StreamProvince, "CaravanIntercepted",
			map[string]any{"transport_id": t.id, "sentry_unit_id": sentryID, "interceptor": interceptor, "q": pos.Q, "r": pos.R},
			worldID, nil)
	}
	if h.notifier != nil {
		_ = h.notifier.NotifyPlayer(ctx, worldID, interceptor, "CaravanSeized", 3,
			map[string]any{"transport_id": t.id, "q": pos.Q, "r": pos.R})
		_ = h.notifier.NotifyPlayer(ctx, worldID, t.owner, "CaravanRaided", 3,
			map[string]any{"transport_id": t.id, "q": pos.Q, "r": pos.R})
	}
	slog.Info("caravan intercepted", "transport", t.id, "by", interceptor, "sentry", sentryID, "q", pos.Q, "r", pos.R)
	return nil
}
