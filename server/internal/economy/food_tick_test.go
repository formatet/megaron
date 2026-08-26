package economy

// DB integration tests for FoodTickHandler (Föda, priority 55), gated by
// DATABASE_URL — megaron_plan_utfodringsordningen.md §6 rött-före fall 1 and 3
// (fall 2, garrison-before-population, needs UpkeepTick too and lives in
// internal/combat/food_ordering_test.go).
//
// Rött-före (before this slice, per the plan §2): FoodConsumptionSplit was
// called against grain/fish's PRODUCTION RATE, not their STOCK. A settlement
// with zero grain production but a full grain stock got grainShare =
// min(demand, 0) = 0 — the whole daily need went to fish, and the grain
// stockpile sat untouched no matter how large it was. FoodTickHandler
// (food_tick.go, D3) fixes this by calling FoodConsumptionSplit against
// GREATEST(0, settled(amount, rate, calc_tick)) — the actual stock — instead.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// foodFixture is a single active world with one owned settlement, sized so
// tests can set population/goods precisely without another test's leftover
// active world interfering (current_world_tick() assumes a single active world).
type foodFixture struct {
	worldID, settlementID uuid.UUID
	tick                  int
}

func newFoodFixture(t *testing.T, pool *pgxpool.Pool, tag string, population int) foodFixture {
	t.Helper()
	ctx := context.Background()
	const tick = 4000

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	f := foodFixture{tick: tick}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"food-tick-"+tag+"-"+uuid.New().String(), tick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"food-tick-owner-"+tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var prov uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Foodtown', 'achaean', $3, 'capital', true, 'active', $4) RETURNING id`,
		f.worldID, prov, owner, population,
	).Scan(&f.settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	return f
}

// seedFoodStock inserts grain/fish/livestock rows at the given STOCK
// (rate=0, calc_tick=f.tick so settled() reads back exactly `amount`, no
// drift from elapsed ticks).
func seedFoodStock(t *testing.T, pool *pgxpool.Pool, f foodFixture, grain, fish, livestock float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, 1000000, $5),
		        ($1, $6, $4, 0, 1000000, $5)`,
		f.settlementID, GoodGrain, grain, fish, f.tick, GoodFish,
	); err != nil {
		t.Fatalf("seed grain/fish: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, 1000000, $4)`,
		f.settlementID, GoodLivestock, livestock, f.tick,
	); err != nil {
		t.Fatalf("seed livestock: %v", err)
	}
}

func foodStockOf(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, good string) float64 {
	t.Helper()
	var v float64
	if err := pool.QueryRow(context.Background(),
		`SELECT GREATEST(0, settled(amount, rate, calc_tick)) FROM settlement_goods
		  WHERE settlement_id = $1 AND good_key = $2`, settlementID, good,
	).Scan(&v); err != nil {
		t.Fatalf("read %s stock: %v", good, err)
	}
	return v
}

func foodUnmetOf(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) float64 {
	t.Helper()
	var v float64
	if err := pool.QueryRow(context.Background(),
		`SELECT food_unmet_amount FROM settlements WHERE id = $1`, settlementID,
	).Scan(&v); err != nil {
		t.Fatalf("read food_unmet_amount: %v", err)
	}
	return v
}

func newFoodTickHandler(pool *pgxpool.Pool) *FoodTickHandler {
	return NewFoodTickHandler(pool,
		events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		events.NewStore(pool), nil)
}

// Rött-före fall 1 (§6.1): a settlement with a fully-stocked granary and a
// fish stock that alone would cover the day's demand. Before this slice,
// FoodConsumptionSplit was fed grain's RATE (0 here, matching "grain_prod =
// 0" in the plan) instead of its STOCK: grainShare = min(demand, 0) = 0, the
// entire need fell through to fish, and the grain stockpile never moved no
// matter how large it was. Grönt: grain drops by exactly the day's demand;
// fish is untouched, because grain alone covers it.
func TestFoodTick_DrawsGrainStockBeforeFish(t *testing.T) {
	pool := testPool(t)
	const population = 1000 // demand = 1000 * 0.5 = 500/tick (GrainConsumptionPerCitizenPerTick)
	f := newFoodFixture(t, pool, "grain-before-fish", population)

	const grainStock = 10000.0 // far more than one day's demand
	const fishStock = 5000.0
	seedFoodStock(t, pool, f, grainStock, fishStock, 0)

	demand := GrainConsumptionPerTick(population)
	if demand <= 0 || demand >= grainStock {
		t.Fatalf("fixture demand %.1f does not sit strictly between 0 and grainStock %.1f — test would not discriminate", demand, grainStock)
	}

	h := newFoodTickHandler(pool)
	if err := h.Handle(context.Background(), events.ScheduledEvent{
		ID: 900001, WorldID: f.worldID, DueTick: f.tick,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	grainAfter := foodStockOf(t, pool, f.settlementID, GoodGrain)
	fishAfter := foodStockOf(t, pool, f.settlementID, GoodFish)
	unmet := foodUnmetOf(t, pool, f.settlementID)

	if want := grainStock - demand; grainAfter != want {
		t.Errorf("grain stock after FoodTick = %.1f, want %.1f (demand %.1f debited from STOCK, not from a zero rate)", grainAfter, want, demand)
	}
	if fishAfter != fishStock {
		t.Errorf("fish stock after FoodTick = %.1f, want unchanged %.1f — fish must stay untouched while grain alone covers demand", fishAfter, fishStock)
	}
	if unmet != 0 {
		t.Errorf("food_unmet_amount = %.1f, want 0 — grain stock covered the whole day's demand", unmet)
	}
}

// Rött-före fall 3 (§6.3), D5/G2: FoodTickHandler must tolerate its
// ScheduledFoodTick event being replayed (handler timeout, crash between
// commit and markDone — CLAUDE.md "Events"). Mirrors
// internal/combat/upkeep_idempotent_test.go's TestUpkeepHandler_ReplayIsIdempotent:
// drive the SAME scheduled event through Handle twice and assert the second
// run is a no-op.
func TestFoodTick_ReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	const population = 1000 // demand = 500/tick
	f := newFoodFixture(t, pool, "idempotent", population)

	const grainStock = 1000.0
	seedFoodStock(t, pool, f, grainStock, 0, 0)

	h := newFoodTickHandler(pool)
	const fixedEventID int64 = 900002
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: f.worldID, DueTick: f.tick}

	if err := h.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	grainAfterFirst := foodStockOf(t, pool, f.settlementID, GoodGrain)
	unmetAfterFirst := foodUnmetOf(t, pool, f.settlementID)

	demand := GrainConsumptionPerTick(population)
	if want := grainStock - demand; grainAfterFirst != want {
		t.Fatalf("grain after first run = %.1f, want %.1f — fixture does not exercise the handler", grainAfterFirst, want)
	}

	// Replay the SAME event.
	if err := h.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	grainAfterReplay := foodStockOf(t, pool, f.settlementID, GoodGrain)
	unmetAfterReplay := foodUnmetOf(t, pool, f.settlementID)

	if grainAfterReplay != grainAfterFirst {
		t.Errorf("grain after replay = %.1f, want unchanged %.1f (a non-idempotent handler would double-debit to %.1f)",
			grainAfterReplay, grainAfterFirst, grainAfterFirst-demand)
	}
	if unmetAfterReplay != unmetAfterFirst {
		t.Errorf("food_unmet_amount after replay = %.1f, want unchanged %.1f", unmetAfterReplay, unmetAfterFirst)
	}

	var claimCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM processed_tick_claims WHERE event_id = $1 AND scope_id = $2`,
		fixedEventID, f.settlementID,
	).Scan(&claimCount); err != nil {
		t.Fatalf("count processed_tick_claims rows: %v", err)
	}
	if claimCount != 1 {
		t.Errorf("processed_tick_claims rows for (event %d, settlement %s) = %d, want exactly 1", fixedEventID, f.settlementID, claimCount)
	}
}
