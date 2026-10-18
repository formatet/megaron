package economy

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"math"
	"os"
	"testing"
	"time"
)

func TestFoodStore_SingleInventory(t *testing.T) {
	pool := testPool(t)
	var exists bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass('public.settlement_granary') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("separate granary still exists: food must live in settlement_goods only")
	}
}

// Real FoodTick against a surplus: all food remains directly available in the
// city; retired scheduled work cannot store it elsewhere or emit new outcomes.
func TestFoodStore_SurplusAndRetiredTick(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "one-inventory", 1000)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',10000,100,100000,$2)`, f.settlementID, f.tick); err != nil {
		t.Fatal(err)
	}
	if err := events.RetiredSitosTick(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatal(err)
	}
	h := NewFoodTickHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))), events.NewStore(pool), nil)
	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 184001, WorldID: f.worldID, DueTick: f.tick}); err != nil {
			t.Fatal(err)
		}
	}
	var amount float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, f.settlementID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if math.Abs(amount-(10000-FoodNeedPerTick(1000))) > 1e-9 {
		t.Fatalf("single city stock or replay debit wrong: %.8f", amount)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type IN ('SitosGranaryStored','SitosGranaryReleased')`, f.worldID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("retired reserve still emits %d outcomes", n)
	}
}

// Apply the actual migration to an old-schema fixture inside a rollback TX.
// Covers lazy stock, over-cap credit and absent city-good rows without touching
// any live DB or leaving pre-migration tables in the current test schema.
func TestFoodStore_MigrationConservesLazyStockAndMissingGood(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "migration", 1000)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, q := range []string{
		`CREATE TABLE settlement_granary(settlement_id uuid,good_key text,amount float8)`,
		`CREATE TABLE processed_sitos_ticks(event_id bigint)`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',7,3,5,$2),($1,'silver',99,0,100,$3)`, f.settlementID, f.tick-2, f.tick); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO settlement_granary VALUES($1,'grain',11),($1,'fish',17)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,due_tick,process_after) VALUES($1,'SitosTick','{}',$2,now())`, f.worldID, f.tick); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../db/migrations/184_remove_sitos.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	for good, want := range map[string]float64{"grain": 24, "fish": 17, "silver": 99} {
		var amount, cap float64
		if err := tx.QueryRow(ctx, `SELECT settled(amount,rate,calc_tick),cap FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, f.settlementID, good).Scan(&amount, &cap); err != nil {
			t.Fatal(err)
		}
		if amount != want || cap < amount {
			t.Fatalf("%s amount%f cap%f want%f without clipping", good, amount, cap, want)
		}
	}

	// Recompute is the same implementation used by recompute-all and completed
	// building/placement changes. Existing cap is preserved on conflict; it must
	// not discard the food just returned by the migration.
	if err := RecomputeProduction(ctx, tx, f.settlementID); err != nil {
		t.Fatal(err)
	}
	for good, want := range map[string]float64{"grain": 24, "fish": 17, "silver": 99} {
		var amount, cap float64
		if err := tx.QueryRow(ctx, `SELECT amount,cap FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, f.settlementID, good).Scan(&amount, &cap); err != nil {
			t.Fatal(err)
		}
		if amount != want || cap < amount {
			t.Fatalf("recompute clipped migrated %s: amount%f cap%f want%f", good, amount, cap, want)
		}
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='SitosTick' AND processed_at IS NULL`, f.worldID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("retired pending ticks %d", pending)
	}
}
