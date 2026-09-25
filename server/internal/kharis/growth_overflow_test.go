package kharis

import (
	"context"
	"testing"
)

// TestApplyDecay_GrowthLeavesTheStoresToFill is the point of the 2026-09-26
// model (Timothy: "GRAIN ska inte GÅ IN I TILLVÄXT, FÖRRÅDEN SKA FYLLAS PÅ").
// Under the grain-priced model a self-sufficient city's stock never rose
// above a remainder — growth spent everything over the reserve. Now a city
// with an överflöd grows AND its grain stock climbs.
func TestApplyDecay_GrowthLeavesTheStoresToFill(t *testing.T) {
	terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 1500)
	h := newTestTickHandler(pool)

	startPop, _ := snapshot(t, pool, settlementID)
	advanceOneDay(t, h, pool, worldID)
	_, day1Grain := snapshot(t, pool, settlementID)

	const days = 20
	prevPop := startPop
	var pop int
	var grain float64
	for day := 2; day <= days; day++ {
		advanceOneDay(t, h, pool, worldID)
		pop, grain = snapshot(t, pool, settlementID)
		t.Logf("day %d: pop=%d grain=%.2f", day, pop, grain)
		if pop < prevPop {
			t.Fatalf("day %d: population shrank %d -> %d in a self-sufficient city", day, prevPop, pop)
		}
		prevPop = pop
	}
	if pop <= startPop {
		t.Errorf("pop %d -> %d over %d days: a city with an överflöd must grow", startPop, pop, days)
	}
	if grain <= day1Grain {
		t.Errorf("grain %.2f on day 1 -> %.2f on day %d: the stores must fill while the city grows", day1Grain, grain, days)
	}
}

// TestApplyDecay_FedWithoutSurplusHolds pins the middle state: a city that
// eats more than it makes but is still fed from its stores (food_unmet_amount
// 0, economy.FoodNet ≤ 0) neither grows nor starves — no överflöd, no growth.
func TestApplyDecay_FedWithoutSurplusHolds(t *testing.T) {
	// 50 gubbar on one cap-limited grain hex eat far more than it grows
	// (see TestApplyDecay_Growth_OverCapPopulationStarves) — a large seeded
	// stock keeps them fed for the length of the test.
	terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 5000)
	h := newTestTickHandler(pool)
	if _, err := pool.Exec(context.Background(),
		`UPDATE settlement_goods SET amount = 1000000, cap = 2000000,
		        calc_tick = (SELECT current_tick FROM worlds WHERE id = $2)
		 WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID, worldID,
	); err != nil {
		t.Fatalf("seed grain stock: %v", err)
	}

	startPop, _ := snapshot(t, pool, settlementID)
	for day := 1; day <= 3; day++ {
		advanceOneDay(t, h, pool, worldID)
		pop, grain := snapshot(t, pool, settlementID)
		t.Logf("day %d: pop=%d grain=%.2f", day, pop, grain)
		if pop != startPop {
			t.Fatalf("day %d: pop %d -> %d — a fed city with no food surplus must hold", day, startPop, pop)
		}
	}
}
