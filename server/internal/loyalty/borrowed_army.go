package loyalty

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

// BorrowedArmyPenaltyHandler applies daily penalties for armies that have been
// borrowed by the king for too long.
//
// Day 1–7:  no penalty.
// Day 8+:   king's kharis -5/day.
// Day 15+:  lender's loyalty -1/day in addition.
type BorrowedArmyPenaltyHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	clk        clock.Clock
}

// NewBorrowedArmyPenaltyHandler creates a BorrowedArmyPenaltyHandler.
func NewBorrowedArmyPenaltyHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, clk clock.Clock) *BorrowedArmyPenaltyHandler {
	return &BorrowedArmyPenaltyHandler{pool: pool, scheduler: sched, eventStore: store, clk: clk}
}

// Handle processes a BorrowedArmyTick scheduled event.
func (h *BorrowedArmyPenaltyHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	type borrowRow struct {
		id         uuid.UUID
		kingdomID  uuid.UUID
		lenderID   uuid.UUID
		borrowedAt time.Time
	}

	rows, err := h.pool.Query(ctx,
		`SELECT ba.id, ba.kingdom_id, ba.lender_id, ba.borrowed_at
		 FROM borrowed_armies ba
		 JOIN kingdoms k ON k.id = ba.kingdom_id
		 WHERE k.world_id = $1 AND ba.returned_at IS NULL`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("query borrowed armies: %w", err)
	}
	defer rows.Close()

	var borrows []borrowRow
	for rows.Next() {
		var b borrowRow
		if err := rows.Scan(&b.id, &b.kingdomID, &b.lenderID, &b.borrowedAt); err == nil {
			borrows = append(borrows, b)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := h.clk.Now()
	for _, b := range borrows {
		daysHeld := int(now.Sub(b.borrowedAt).Hours() / 24)

		if daysHeld >= 7 {
			if err := h.penaliseKingKharis(ctx, b.kingdomID, e.WorldID); err != nil {
				slog.Error("king kharis penalty", "kingdom", b.kingdomID, "err", err)
			}
		}
		if daysHeld >= 14 {
			if err := h.penaliseLenderLoyalty(ctx, b.lenderID, e.WorldID); err != nil {
				slog.Error("lender loyalty penalty", "lender", b.lenderID, "err", err)
			}
		}
	}

	return h.scheduler.EnqueueTick(ctx, e.WorldID, events.ScheduledBorrowedArmyTick,
		DailyTickPayload{}, e.DueTick+1)
}

// penaliseKingKharis drains 5 kharis from the king's capital settlement.
func (h *BorrowedArmyPenaltyHandler) penaliseKingKharis(ctx context.Context, kingdomID, worldID uuid.UUID) error {
	// Find king's capital settlement.
	var kingSettlementID uuid.UUID
	err := h.pool.QueryRow(ctx,
		`SELECT s.id
		 FROM settlements s
		 JOIN kingdom_members km ON km.player_id = s.owner_id
		 WHERE km.kingdom_id = $1 AND km.role = 'king'
		   AND s.world_id = $2 AND s.is_capital = true`,
		kingdomID, worldID,
	).Scan(&kingSettlementID)
	if err != nil {
		return fmt.Errorf("find king settlement: %w", err)
	}

	_, err = h.pool.Exec(ctx,
		`UPDATE settlements SET
		   kharis_amount = GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick) - 5),
		   kharis_calc_tick = current_world_tick()
		 WHERE id = $1`,
		kingSettlementID,
	)
	if err != nil {
		return fmt.Errorf("drain king kharis: %w", err)
	}

	_, _ = h.eventStore.Append(ctx, kingSettlementID, events.StreamProvince, "KharisLost",
		map[string]any{"amount": 5, "reason": "borrowed_army_too_long"}, worldID, nil)
	return nil
}

// penaliseLenderLoyalty applies -1 loyalty to the lender's capital.
func (h *BorrowedArmyPenaltyHandler) penaliseLenderLoyalty(ctx context.Context, lenderID, worldID uuid.UUID) error {
	var settlementID uuid.UUID
	err := h.pool.QueryRow(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, lenderID,
	).Scan(&settlementID)
	if err != nil {
		return fmt.Errorf("find lender settlement: %w", err)
	}

	return AppendLoyaltyEvent(ctx, h.pool, h.eventStore, settlementID, worldID,
		"borrowed_army_penalty", -1, "army_not_returned")
}
