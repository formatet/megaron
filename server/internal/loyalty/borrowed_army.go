package loyalty

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
			if err := h.penaliseKingKharis(ctx, b.id, b.kingdomID, e.WorldID, e.ID); err != nil {
				slog.Error("king kharis penalty", "kingdom", b.kingdomID, "err", err)
			}
		}
		if daysHeld >= 14 {
			if err := h.penaliseLenderLoyalty(ctx, b.id, b.lenderID, e.WorldID, e.ID); err != nil {
				slog.Error("lender loyalty penalty", "lender", b.lenderID, "err", err)
			}
		}
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledBorrowedArmyTick,
		MacroTickPayload{}, e.DueTick, events.MacroTickInterval)
}

// penaliseKingKharis drains 5 kharis from the king's capital settlement.
// Claimed per (event_id, scope) — migration 098's processed_tick_claims, the
// same guard colony.go's applyColonyPenalty uses for this exact class of bug
// (Handle fans one scheduled event across every overdue borrow and mutates
// directly; a G2 handler timeout or crash mid-fan-out leaves the event
// unprocessed and events.Worker re-runs it from the top). scope is derived
// from the borrowed_armies row (borrowID), not the king's settlement id
// directly: one king can hold several overdue borrows in the same event
// pass, all resolving to the SAME capital settlement, and a claim keyed on
// settlement_id alone would let the second borrow's claim collide with the
// first's and silently skip a penalty that should fire.
func (h *BorrowedArmyPenaltyHandler) penaliseKingKharis(ctx context.Context, borrowID, kingdomID, worldID uuid.UUID, eventID int64) error {
	scope := uuid.NewSHA1(borrowID, []byte("king_kharis"))
	claim, err := h.pool.Exec(ctx,
		`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, eventID, scope)
	if err != nil {
		return fmt.Errorf("claim king kharis penalty: %w", err)
	}
	if claim.RowsAffected() == 0 {
		return nil // this event already penalised this borrow's king
	}

	// Find king's capital settlement.
	var kingSettlementID uuid.UUID
	err = h.pool.QueryRow(ctx,
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
// Claimed per (event_id, scope) for the same reason as penaliseKingKharis:
// scope is derived from the borrowed_armies row rather than the lender's
// settlement id directly, because the same lender can have multiple overdue
// loans outstanding (nothing in BorrowArmy prevents lending more than once),
// all resolving to the SAME lender capital settlement within one event pass.
func (h *BorrowedArmyPenaltyHandler) penaliseLenderLoyalty(ctx context.Context, borrowID, lenderID, worldID uuid.UUID, eventID int64) error {
	scope := uuid.NewSHA1(borrowID, []byte("lender_loyalty"))
	claim, err := h.pool.Exec(ctx,
		`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, eventID, scope)
	if err != nil {
		return fmt.Errorf("claim lender loyalty penalty: %w", err)
	}
	if claim.RowsAffected() == 0 {
		return nil // this event already penalised this borrow's lender
	}

	var settlementID uuid.UUID
	err = h.pool.QueryRow(ctx,
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
