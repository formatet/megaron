// Package loyalty implements loyalty tick handlers for scheduled daily events.
package loyalty

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
)

// DailyTickPayload is the payload for recurring daily world tick events.
type DailyTickPayload struct{}

// DecayHandler applies loyalty decay for neglected colonies.
// A colony (is_capital=false) that received no gift in the last 48 hours
// loses 1 loyalty point (minimum 1).
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
		         AND le.created_at > now() - interval '48 hours'
		   )`,
		e.WorldID,
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

	return h.scheduler.EnqueueTick(ctx, e.WorldID, events.ScheduledLoyaltyDecayTick,
		DailyTickPayload{}, e.DueTick+1)
}

func (h *DecayHandler) applyDecay(ctx context.Context, settlementID, worldID uuid.UUID) error {
	tag, err := h.pool.Exec(ctx,
		`UPDATE settlements SET loyalty = GREATEST(1, loyalty - 1)
		 WHERE id = $1 AND loyalty > 1`,
		settlementID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	_, err = h.eventStore.Append(ctx, settlementID, events.StreamProvince, "LoyaltyDecay",
		map[string]any{"delta": -1, "reason": "long_silence"}, worldID, nil)
	if err != nil {
		slog.Error("record decay event", "err", err)
	}
	return nil
}

// AppendLoyaltyEvent writes a loyalty_events row and updates settlement.loyalty.
// Delta is clamped so loyalty stays in [1, 4].
func AppendLoyaltyEvent(ctx context.Context, pool *pgxpool.Pool, store *events.Store, settlementID, worldID uuid.UUID, eventType string, delta int, reason string) error {
	// Clamp to [1,4].
	_, err := pool.Exec(ctx,
		`UPDATE settlements
		 SET loyalty = GREATEST(1, LEAST(4, loyalty + $1)),
		     loyalty_trend = CASE
		         WHEN $1 > 0 THEN 'rising'
		         WHEN $1 < 0 THEN 'falling'
		         ELSE loyalty_trend
		     END
		 WHERE id = $2`,
		delta, settlementID,
	)
	if err != nil {
		return fmt.Errorf("update loyalty: %w", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO loyalty_events (settlement_id, world_id, event_type, loyalty_delta, reason)
		 VALUES ($1, $2, $3, $4, $5)`,
		settlementID, worldID, eventType, delta, reason,
	)
	if err != nil {
		return fmt.Errorf("insert loyalty_event: %w", err)
	}

	if _, serr := store.Append(ctx, settlementID, events.StreamProvince, "LoyaltyChanged",
		map[string]any{"event_type": eventType, "delta": delta, "reason": reason},
		worldID, nil,
	); serr != nil {
		slog.Error("record loyalty event", "err", serr)
	}

	// Check revolt conditions after any loyalty change.
	go checkRevolt(context.Background(), pool, settlementID)
	return nil
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
