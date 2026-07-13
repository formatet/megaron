// Package loyalty implements loyalty tick handlers for scheduled daily events.
package loyalty

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/tick"
)

// loyaltyExecutor is the subset of pgx used to write a loyalty change. Both
// *pgxpool.Pool and pgx.Tx satisfy it, so AppendLoyaltyEvent (pool, non-tx
// daily handlers) and AppendLoyaltyEventTx (callers already inside a tx, e.g.
// combat resolution) can share the same two writes.
type loyaltyExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// DailyTickPayload is the payload for recurring daily world tick events.
type DailyTickPayload struct{}

// loyaltyDecayGraceDays is how many game-days a colony may go without a
// loyalty-raising event (gift/governor_visit/victory_nearby) before decay
// applies. Expressed in game-days — NOT wall-clock — because the decay tick
// itself is tick-scheduled (fires once per game-day). The window is converted
// to real time via tick.TickMinutes at query time, the same conversion the
// messenger travel durations use (internal/messenger/recall.go). A hard-coded
// "48 hours" silently became ~120 game-days at TICK_MINUTES=1, disabling decay
// in sped-up worlds (the "loyalty 2 uniformly" soak finding).
const loyaltyDecayGraceDays = 2

// decayGraceMinutes converts the grace window to real minutes at the current
// tick cadence: graceDays × ticks/day × minutes/tick. At the default 60
// min/tick this is 2880 (48 h, preserving the old behaviour); at
// TICK_MINUTES=1 it is 48 (a true 2 game-days).
func decayGraceMinutes() int {
	return loyaltyDecayGraceDays * events.TicksPerDay * tick.TickMinutes
}

// DecayHandler applies loyalty decay for neglected colonies.
// A colony (is_capital=false) that received no gift in the last
// loyaltyDecayGraceDays game-days loses 1 loyalty point (minimum 1).
type DecayHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
}

// NewDecayHandler creates a DecayHandler.
func NewDecayHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store) *DecayHandler {
	return &DecayHandler{pool: pool, scheduler: sched, eventStore: store}
}

// Handle processes a LoyaltyDecayTick scheduled event.
func (h *DecayHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1
		   AND is_capital = false
		   AND owner_id IS NOT NULL
		   AND loyalty > 1
		   AND NOT EXISTS (
		       SELECT 1 FROM loyalty_events le
		       WHERE le.settlement_id = settlements.id
		         AND le.event_type IN ('gift', 'governor_visit', 'victory_nearby')
		         AND le.created_at > now() - ($2 * interval '1 minute')
		   )`,
		e.WorldID, decayGraceMinutes(),
	)
	if err != nil {
		return fmt.Errorf("query neglected colonies: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if err := h.applyDecay(ctx, id, e.WorldID); err != nil {
			slog.Error("loyalty decay failed", "settlement", id, "err", err)
		}
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledLoyaltyDecayTick,
		DailyTickPayload{}, e.DueTick, events.TicksPerDay)
}

func (h *DecayHandler) applyDecay(ctx context.Context, settlementID, worldID uuid.UUID) error {
	// Neglect erodes loyalty_points (a -1 intent, scaled by the loss factor); the
	// integer loyalty is re-derived. The query already filtered to loyalty > 1, so
	// this only bites colonies with headroom above near-revolt.
	rows, err := applyLoyaltyPointsDelta(ctx, h.pool, settlementID, scaleDeltaToPoints(-1))
	if err != nil {
		return err
	}
	if rows == 0 {
		return nil
	}

	_, err = h.eventStore.Append(ctx, settlementID, events.StreamProvince, "LoyaltyDecay",
		map[string]any{"delta": -1, "reason": "long_silence"}, worldID, nil)
	if err != nil {
		slog.Error("record decay event", "err", err)
	}
	return nil
}

// appendLoyaltyEventOn writes the loyalty projection UPDATE + loyalty_events row
// + audit event on the given executor (pool or tx). Delta is clamped so loyalty
// stays in [1, 4]. It does NOT evaluate revolt — that is the pool-based wrapper's
// job (checkRevolt must run against committed rows, not an open tx).
func appendLoyaltyEventOn(ctx context.Context, db loyaltyExecutor, store *events.Store, settlementID, worldID uuid.UUID, eventType string, delta int, reason string) error {
	// Accumulator model: the integer delta is an INTENT — it moves the hidden
	// loyalty_points (scaled + asymmetric), and the integer loyalty (1–4) is
	// re-derived from the bands. loyalty_events still logs the integer intent for
	// audit legibility.
	if _, err := applyLoyaltyPointsDelta(ctx, db, settlementID, scaleDeltaToPoints(delta)); err != nil {
		return fmt.Errorf("update loyalty points: %w", err)
	}

	if _, err := db.Exec(ctx,
		`INSERT INTO loyalty_events (settlement_id, world_id, event_type, loyalty_delta, reason)
		 VALUES ($1, $2, $3, $4, $5)`,
		settlementID, worldID, eventType, delta, reason,
	); err != nil {
		return fmt.Errorf("insert loyalty_event: %w", err)
	}

	if _, serr := store.Append(ctx, settlementID, events.StreamProvince, "LoyaltyChanged",
		map[string]any{"event_type": eventType, "delta": delta, "reason": reason},
		worldID, nil,
	); serr != nil {
		slog.Error("record loyalty event", "err", serr)
	}
	return nil
}

// AppendLoyaltyEvent writes a loyalty_events row and updates settlement.loyalty.
// Delta is clamped so loyalty stays in [1, 4]. For callers on the pool (daily
// tick handlers); it also evaluates revolt conditions after the change.
func AppendLoyaltyEvent(ctx context.Context, pool *pgxpool.Pool, store *events.Store, settlementID, worldID uuid.UUID, eventType string, delta int, reason string) error {
	if err := appendLoyaltyEventOn(ctx, pool, store, settlementID, worldID, eventType, delta, reason); err != nil {
		return err
	}
	// Check revolt conditions after any loyalty change.
	go checkRevolt(context.Background(), pool, settlementID)
	return nil
}

// AppendLoyaltyEventTx writes the same loyalty change on an existing transaction,
// so the loyalty projection is atomic with the caller's surrounding writes
// (combat resolution updates settlements/units/goods in one tx — a cross-
// connection pool write would block on that tx's own locks). Revolt is not
// evaluated here; a battle that drops a settlement to loyalty 1 is picked up by
// the next pool-based loyalty change or the daily decay handler's checkRevolt.
func AppendLoyaltyEventTx(ctx context.Context, tx pgx.Tx, store *events.Store, settlementID, worldID uuid.UUID, eventType string, delta int, reason string) error {
	return appendLoyaltyEventOn(ctx, tx, store, settlementID, worldID, eventType, delta, reason)
}

// checkRevolt runs asynchronously after loyalty changes and flips settlement
// state to 'revolting' if all three conditions are met.
func checkRevolt(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID) {
	var loyalty int
	var ownerID *uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT loyalty, owner_id FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&loyalty, &ownerID)
	if err != nil || loyalty > 1 {
		return
	}

	// Count garrison units from the units table (discrete unit model, mig 047).
	var garrisonCount int
	_ = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM units WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	).Scan(&garrisonCount)

	ownerFraction := 1.0
	// For now: assume all garrison troops belong to owner; in future,
	// borrowed army complicates this.
	if ownerID == nil {
		ownerFraction = 0
	}

	// Trigger event: loyalty just hit 1 and garrison is thin (fewer than 5 units).
	triggerOccurred := garrisonCount < 5

	if loyalty == 1 && ownerFraction < 0.5 && triggerOccurred {
		_, _ = pool.Exec(ctx,
			`UPDATE settlements SET state = 'revolting' WHERE id = $1 AND loyalty = 1`,
			settlementID,
		)
		slog.Info("revolt triggered", "settlement", settlementID)
	}
}
