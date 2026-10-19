package economy

import (
	"context"
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
// about. A fully fed population stays quiet.
const EventFoodShortfall = "FoodShortfall"

// EventHerdLost is an audit-only outcome (no notification kind): animals the
// pen could not hold were lost after the day's meal (order 28b).
const EventHerdLost = "HerdLost"

// HerdLostPayload: Lost animals written off, Cap the herd cap that applied.
type HerdLostPayload struct {
	Lost int    `json:"lost"`
	Cap  int    `json:"cap"`
	Name string `json:"name"`
}

// EventHerdBirths is an audit-only outcome (no notification kind): the calves
// born this tick (order 33). Fed of them were paid for with GrainUsed grain.
const EventHerdBirths = "HerdBirths"

// HerdBirthsPayload: Born calves, Fed of them paid with grain, GrainUsed grain.
type HerdBirthsPayload struct {
	Born      int     `json:"born"`
	Fed       int     `json:"fed"`
	GrainUsed float64 `json:"grain_used"`
	Name      string  `json:"name"`
}

// FoodShortfallPayload is the outcome of one day's food debit (events store
// outcomes, not intentions — CLAUDE.md "Events"): Unmet is what was left
// uncovered after grain, fish and livestock were all exhausted; the three
// *Used fields say how the covered part was actually paid, for audit.
type FoodShortfallPayload struct {
	Unmet         float64            `json:"unmet"`
	GrainUsed     float64            `json:"grain_used"`
	FishUsed      float64            `json:"fish_used"`
	LivestockUsed int                `json:"livestock_used"`
	FoodUsed      map[string]float64 `json:"food_used,omitempty"`
	// Name is the settlement's name (megaron_plan_tre_tysta_notiserna.md):
	// without it a Wanax with several cities only learns that ONE of them
	// starved, never which. New field, old ones unchanged (CLAUDE.md §Events).
	Name string `json:"name"`
}

// FoodTickHandler is Föda, priority 55 (between Plikt/UpkeepTick at 50 and
// Tillväxt/KharisTick at 60 — internal/events/priority.go,
// megaron_tickordning.md) — the self-rescheduling per-world pass that debits
// each settlement's population's daily food need from its STOCK, in kanon
// proportional four-food order, followed by unchanged civilian emergency herd.
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
		if err := h.tickSettlement(ctx, id, e.WorldID, e.ID, e.DueTick); err != nil {
			slog.Error("food tick: settlement failed", "settlement", id, "err", err)
		}
	}

	// Reschedule next tick (cadence +1, matching upkeep and kharis).
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
// KharisTick/colony use for their own per-settlement claims. Handle
// fans ONE ScheduledFoodTick event across every settlement, each committing
// its own transaction here, so the claim must be scoped per settlement, not
// per event: a Handle-level claim would either falsely skip settlements never
// reached before a crash, or falsely mark them done before their writes commit.
func (h *FoodTickHandler) tickSettlement(ctx context.Context, settlementID, worldID uuid.UUID, eventID int64, tick int) error {
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

	// Births come BEFORE the meal, in the same transaction: calves over the pen's
	// cap are then the first thing eaten today (order 33).
	born, fed, err := herdBirths(ctx, tx, settlementID, tick)
	if err != nil {
		return fmt.Errorf("herd births: %w", err)
	}

	meal, err := TakeCivilianFood(ctx, tx, settlementID, GrainConsumptionPerTick(population))
	if err != nil {
		return fmt.Errorf("debit civilian meal: %w", err)
	}
	unmet := meal.Unmet
	grainUsed, fishUsed, livestockConsumed := meal.Used[GoodGrain], meal.Used[GoodFish], int(meal.Used[GoodLivestock])
	if _, err := tx.Exec(ctx, `UPDATE settlements SET food_unmet_amount=$2,food_consumed=$3,food_consumed_tick=current_world_tick() WHERE id=$1`, settlementID, unmet, meal.Used); err != nil {
		return fmt.Errorf("write food outcome: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if born > 0 {
		var name string
		_ = h.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, settlementID).Scan(&name)
		_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, EventHerdBirths,
			HerdBirthsPayload{Born: born, Fed: fed, GrainUsed: float64(fed), Name: name}, worldID, nil)
	}

	if meal.Lost > 0 {
		var name string
		_ = h.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, settlementID).Scan(&name)
		_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, EventHerdLost,
			HerdLostPayload{Lost: int(meal.Lost), Cap: int(HerdCap(PenLevel(ctx, h.pool, settlementID))), Name: name}, worldID, nil)
	}

	// The routine case (unmet == 0) stays silent; only actual hunger is
	// worth a daily audit event. Civil hunger notifications belong to the shared
	// starvation transition observer in KharisTick, after this food pass.
	if unmet > 0 {
		var settlementName string
		_ = h.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, settlementID).Scan(&settlementName)

		payload := FoodShortfallPayload{
			Unmet: unmet, GrainUsed: grainUsed, FishUsed: fishUsed, LivestockUsed: livestockConsumed,
			Name: settlementName, FoodUsed: meal.Used,
		}
		_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, EventFoodShortfall, payload, worldID, nil)
	}

	return nil
}

// herdBirths adds this tick's calves to the herd and takes their grain, inside
// the caller's transaction (rows locked in LoadFood's stable order). The rule
// itself is HerdBirths.
func herdBirths(ctx context.Context, tx pgx.Tx, sid uuid.UUID, tick int) (born, fed int, err error) {
	if tick%HerdFedBirthEvery != 0 {
		return 0, 0, nil
	}
	stock, err := LoadFood(ctx, tx, sid, true)
	if err != nil {
		return 0, 0, err
	}
	born, fed = HerdBirths(stock[GoodLivestock], stock[GoodGrain], tick)
	if born == 0 {
		return 0, 0, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=GREATEST(0,settled(amount,rate,calc_tick)+$3),calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key=$2`, sid, GoodLivestock, float64(born)); err != nil {
		return 0, 0, err
	}
	if fed > 0 {
		if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=GREATEST(0,settled(amount,rate,calc_tick)-$3),calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key=$2`, sid, GoodGrain, float64(fed)); err != nil {
			return 0, 0, err
		}
	}
	return born, fed, nil
}
