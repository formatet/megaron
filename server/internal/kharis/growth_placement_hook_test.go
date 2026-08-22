package kharis

// Verifies applyDecay's P4 growth hook (economy.PlaceNextGubbeOnBestFoodHex)
// actually fires when population crosses a new full hundred — not just that
// the wider grain-funded-growth suite stays green with it wired in.

import (
	"context"
	"testing"
)

func TestApplyDecay_PopulationCrossingNewHundredPlacesOneGubbe(t *testing.T) {
	// A rich catchment (all plains, 6 hexes × cap 4 = 24 grain slots) with a
	// starting population just below a hundred boundary, so ONE day's growth
	// is virtually certain to cross it — the tiny headcount here (1-2 gubbar)
	// is nowhere near grain's real per-hex cap (megaron_plan_grain_cap.md),
	// so this test isn't exercising the cap at all, just the growth hook.
	terrains := [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 199)
	h := newTestTickHandler(pool)

	var before int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&before); err != nil {
		t.Fatalf("count placements before: %v", err)
	}

	advanceOneDay(t, h, pool, worldID)

	pop, _ := snapshot(t, pool, settlementID)
	if pop < 200 {
		t.Skipf("population did not cross the 200 boundary this tick (pop=%d) — not what this test exercises", pop)
	}

	var after int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&after); err != nil {
		t.Fatalf("count placements after: %v", err)
	}
	if after <= before {
		t.Errorf("placements after crossing 198→%d = %d, want > %d (the new gubbe should have been auto-placed)", pop, after, before)
	}
}
