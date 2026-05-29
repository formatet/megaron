package loyalty

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/settlement"
)

// ColonyPenaltyHandler applies daily loyalty drift based on how many colonies
// a Wanax controls. More colonies → higher penalty per colony per day.
type ColonyPenaltyHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
}

// NewColonyPenaltyHandler creates a ColonyPenaltyHandler.
func NewColonyPenaltyHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store) *ColonyPenaltyHandler {
	return &ColonyPenaltyHandler{pool: pool, scheduler: sched, eventStore: store}
}

// Handle processes a ColonyPenaltyTick scheduled event.
func (h *ColonyPenaltyHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// Find all players with at least one colony in this world.
	rows, err := h.pool.Query(ctx,
		`SELECT owner_id, COUNT(*) FILTER (WHERE is_capital = false) AS colony_count
		 FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL
		 GROUP BY owner_id
		 HAVING COUNT(*) FILTER (WHERE is_capital = false) > 0`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("query colony owners: %w", err)
	}
	defer rows.Close()

	type ownerColonies struct {
		ownerID     uuid.UUID
		colonyCount int
	}
	var owners []ownerColonies
	for rows.Next() {
		var oc ownerColonies
		if err := rows.Scan(&oc.ownerID, &oc.colonyCount); err == nil {
			owners = append(owners, oc)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, oc := range owners {
		delta := settlement.ColonyPenalty(oc.colonyCount)
		if delta == 0 {
			continue
		}
		if err := h.applyColonyPenalty(ctx, oc.ownerID, e.WorldID, delta); err != nil {
			slog.Error("colony penalty failed", "owner", oc.ownerID, "err", err)
		}
	}

	return h.scheduler.Enqueue(ctx, e.WorldID, events.ScheduledColonyPenaltyTick,
		DailyTickPayload{}, time.Now().Add(24*time.Hour))
}

// applyColonyPenalty writes a loyalty_events row for each colony belonging to owner.
func (h *ColonyPenaltyHandler) applyColonyPenalty(ctx context.Context, ownerID, worldID uuid.UUID, delta int) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND is_capital = false AND loyalty > 1`,
		worldID, ownerID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		if err := AppendLoyaltyEvent(ctx, h.pool, h.eventStore, id, worldID,
			"colony_penalty", delta, "overextension",
		); err != nil {
			slog.Error("colony penalty event", "settlement", id, "err", err)
		}
	}
	return nil
}
