package economy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventFoodShortfall is the outcome event fired when a settlement's daily
// food need was NOT fully covered even after the grain→fish→livestock
// fallback chain (FoodConsumptionSplit) — the moment a Wanax needs to hear
// about (mirrors SitosTickHandler's own "storing is routine and silent,
// release is rare and notified" posture). A day where the population was
// fully fed emits nothing — the routine case stays quiet, same as Sitos.
const EventFoodShortfall = "FoodShortfall"

// FoodShortfallPayload is the outcome of one day's food debit (events store
// outcomes, not intentions — CLAUDE.md "Events"): Unmet is what was left
// uncovered after grain, fish and livestock were all exhausted; the three
// *Used fields say how the covered part was actually paid, for audit.
type FoodShortfallPayload struct {
	Unmet         float64 `json:"unmet"`
	GrainUsed     float64 `json:"grain_used"`
	FishUsed      float64 `json:"fish_used"`
	LivestockUsed int     `json:"livestock_used"`
}

// FoodTickHandler is Föda, priority 55 (between Plikt/UpkeepTick at 50 and
// Tillväxt/KharisTick at 60 — internal/events/priority.go,
// megaron_tickordning.md) — the self-rescheduling per-world pass that debits
// each settlement's population's daily food need from its STOCK, in kanon
// fallback order grain → fish → livestock (Timothy 2026-08-07, unchanged by
// this slice — megaron_plan_utfodringsordningen.md D3).
//
// Before this handler existed, the population's consumption was folded
// CONTINUOUSLY into grain's rate by RecomputeProduction, ahead of every
// discrete daily step including the army's own upkeep — the population ate
// first and the army took what was left, backwards from Timothy's kanon
// ("ALLT SOM STADEN FÖRSÖRJER ÄTER FÖRE BEFOLKNINGEN", 2026-08-25). Running
// this AFTER UpkeepTick (50) and BEFORE KharisTick (60) makes the ordering
// literal: the army has already eaten by the time this runs, and growth (60)
// only ever sees what is left once the population itself has too.
type FoodTickHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
	hub       Broadcaster
}

// NewFoodTickHandler creates a FoodTickHandler. hub may be nil (tests).
func NewFoodTickHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster) *FoodTickHandler {
	return &FoodTickHandler{pool: pool, scheduler: sched, store: store, hub: hub}
}

// Handle processes one ScheduledFoodTick event for a world.
func (h *FoodTickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state NOT IN ('sunk', 'collapsed')`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("food tick: query settlements: %w", err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if err := h.tickSettlement(ctx, id, e.WorldID, e.ID); err != nil {
			slog.Error("food tick: settlement failed", "settlement", id, "err", err)
		}
	}

	// Reschedule next tick (cadence +1 — same daily cadence as Sitos/Upkeep/Kharis).
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledFoodTick,
		struct{}{}, e.DueTick, events.MacroTickInterval)
}

// tickSettlement debits one settlement's daily food need from its stock, in a
// single TX, and writes the day's unmet outcome to settlements.food_unmet_amount
// (D4 — KharisTick's growth/starvation branch and the siege starvation clock
// both read this column instead of grain's rate/grain_now, migration 134).
//
// G2 idempotency (CLAUDE.md "Events"): claims (event_id, settlement_id) in
// the shared processed_tick_claims table (migration 098) — same convention
// SitosTickHandler uses (processed_sitos_ticks, a settlement-scoped sibling
// table) and KharisTick/colony use for their own per-settlement claims. Handle
// fans ONE ScheduledFoodTick event across every settlement, each committing
// its own transaction here, so the claim must be scoped per settlement, not
// per event: a Handle-level claim would either falsely skip settlements never
// reached before a crash, or falsely mark them done before their writes commit.
func (h *FoodTickHandler) tickSettlement(ctx context.Context, settlementID, worldID uuid.UUID, eventID int64) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	claim, err := tx.Exec(ctx,
		`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		eventID, settlementID)
	if err != nil {
		return fmt.Errorf("claim food tick: %w", err)
	}
	if claim.RowsAffected() == 0 {
		return nil // already processed this event for this settlement
	}

	var population int
	if err := tx.QueryRow(ctx,
		`SELECT population FROM settlements WHERE id = $1 FOR UPDATE`,
		settlementID,
	).Scan(&population); err != nil {
		return fmt.Errorf("load settlement: %w", err)
	}

	var grainStock, fishStock, livestockStock float64
	if err := tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick))
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, GoodGrain,
	).Scan(&grainStock); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load grain stock: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick))
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, GoodFish,
	).Scan(&fishStock); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load fish stock: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick))
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, GoodLivestock,
	).Scan(&livestockStock); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load livestock stock: %w", err)
	}

	demand := GrainConsumptionPerTick(population)

	// FoodConsumptionSplit called against STOCK, not rate (D1/D3): grainNet/
	// fishNet here ARE the settlement's new stock levels after today's meal,
	// not a rate. If demand still isn't covered once livestock is exhausted
	// too, grainNet goes negative — the function's own "encode the shortfall"
	// convention (recompute.go) — which is exactly the unmet this handler
	// exists to surface, not to hide by flooring early.
	grainNet, fishNet, livestockConsumed := FoodConsumptionSplit(demand, grainStock, fishStock, livestockStock)

	unmet := 0.0
	if grainNet < 0 {
		unmet = -grainNet
		grainNet = 0
	}

	grainUsed := grainStock - grainNet
	fishUsed := fishStock - fishNet

	if grainUsed > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, grainUsed, GoodGrain,
		); err != nil {
			return fmt.Errorf("debit grain: %w", err)
		}
	}
	if fishUsed > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, fishUsed, GoodFish,
		); err != nil {
			return fmt.Errorf("debit fish: %w", err)
		}
	}
	if livestockConsumed > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, float64(livestockConsumed), GoodLivestock,
		); err != nil {
			return fmt.Errorf("debit livestock: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET food_unmet_amount = $2 WHERE id = $1`,
		settlementID, unmet,
	); err != nil {
		return fmt.Errorf("write food_unmet_amount: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// The routine case (unmet == 0) stays silent, same posture as Sitos's
	// "storing" leg — only the day the population actually went hungry is
	// worth an event and a notification.
	if unmet > 0 {
		payload := FoodShortfallPayload{
			Unmet: unmet, GrainUsed: grainUsed, FishUsed: fishUsed, LivestockUsed: livestockConsumed,
		}
		_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, EventFoodShortfall, payload, worldID, nil)
		if h.hub != nil {
			var ownerID uuid.UUID
			if err := h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, settlementID).Scan(&ownerID); err == nil {
				_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, EventFoodShortfall, 2, map[string]any{
					"settlement_id": settlementID,
					"unmet":         unmet,
				})
			}
		}
	}

	return nil
}
