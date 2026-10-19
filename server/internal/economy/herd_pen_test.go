package economy

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/events"
)

// Order 28b: the herd has a cap; animals above it are ordinary food, animals
// still above it after the meal are lost, animals under it are emergency only.

func TestHerdOverCapIsEatenInThePool(t *testing.T) {
	// 60 animals, cap 50, 20 grain, people need 30: the pool is 20 grain + 10
	// surplus animals, all eaten.
	meal := PlanFoodHerd(30, map[string]float64{GoodGrain: 20, GoodLivestock: 60}, true, HerdCap(0))
	if math.Abs(meal.Used[GoodLivestock]-10) > 1e-9 || math.Abs(meal.Used[GoodGrain]-20) > 1e-9 || meal.Unmet != 0 || meal.Lost != 0 {
		t.Fatalf("%+v", meal)
	}
}

func TestHerdUnderCapIsNotEatenWhenFed(t *testing.T) {
	meal := PlanFoodHerd(30, map[string]float64{GoodGrain: 500, GoodLivestock: 40}, true, HerdCap(0))
	if meal.Used[GoodLivestock] != 0 || meal.Lost != 0 || meal.Unmet != 0 {
		t.Fatalf("%+v", meal)
	}
}

func TestHerdUnderCapIsEmergencySlaughterWhenStarving(t *testing.T) {
	meal := PlanFoodHerd(5, map[string]float64{GoodLivestock: 40}, true, HerdCap(0))
	if meal.Used[GoodLivestock] != 5 || meal.Unmet != 0 {
		t.Fatalf("%+v", meal)
	}
}

func TestHerdStillOverCapAfterMealIsLost(t *testing.T) {
	// Fed by grain; 60 animals, cap 50, small meal: 10 surplus, eaten share
	// 10*1/110, the rest of the excess is lost.
	meal := PlanFoodHerd(1, map[string]float64{GoodGrain: 100, GoodLivestock: 60}, true, HerdCap(0))
	eaten := meal.Used[GoodLivestock]
	if want := math.Floor(60 - eaten - 50); meal.Lost != want || meal.Lost < 8 {
		t.Fatalf("lost=%g eaten=%g", meal.Lost, eaten)
	}
	// A bigger pen holds them: nothing eaten, nothing lost.
	big := PlanFoodHerd(1, map[string]float64{GoodGrain: 100, GoodLivestock: 60}, true, HerdCap(1))
	if big.Used[GoodLivestock] != 0 || big.Lost != 0 {
		t.Fatalf("%+v", big)
	}
}

func TestHerdCapPerPenLevel(t *testing.T) {
	if HerdCap(0) != 50 || HerdCap(1) != 150 || HerdCap(3) != 350 {
		t.Fatal(HerdCap(0), HerdCap(1), HerdCap(3))
	}
}

func TestFoodTick_HerdOverCapEatenAndRestLost(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "herd-lost", 1000) // demand 5
	seedFoodStock(t, pool, f, 0, 0, 60)
	event := events.ScheduledEvent{ID: 281001, WorldID: f.worldID, DueTick: f.tick}
	if err := newFoodTickHandler(pool).Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
	// 5 eaten from the 10 over the cap, the other 5 lost: herd is exactly the cap.
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 50 {
		t.Fatalf("herd after tick=%g want 50", got)
	}
	if foodUnmetOf(t, pool, f.settlementID) != 0 {
		t.Fatal("people went hungry")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE stream_id=$1 AND event_type=$2 AND (payload->>'lost')::int=5`, f.settlementID, EventHerdLost).Scan(&n); err != nil || n != 1 {
		t.Fatalf("HerdLost events=%d err=%v", n, err)
	}
}

func TestFoodTick_PenHoldsTheHerd(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFoodFixture(t, pool, "herd-pen", 1000)
	seedFoodStock(t, pool, f, 1000, 0, 60)
	if _, err := pool.Exec(ctx, `INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'pen', 1)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	event := events.ScheduledEvent{ID: 281002, WorldID: f.worldID, DueTick: f.tick}
	if err := newFoodTickHandler(pool).Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 60 {
		t.Fatalf("herd after tick=%g want 60", got)
	}
}
