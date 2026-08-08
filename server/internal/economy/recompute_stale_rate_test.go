package economy

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// seedStaleGood writes a settlement_goods row directly (bypassing
// RecomputeProduction) to simulate a good whose production_rule existed at
// some earlier point but no longer matches the settlement's catchment or
// buildings — the scenario empirically hit 2026-07-29 (a rate=42 cedar row
// survived on a cedar-less city through a later recompute).
func seedStaleGood(t *testing.T, settlementID uuid.UUID, goodKey string, amount, rate float64, calcTick int) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, $4, 100000, $5)
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = $3, rate = $4, calc_tick = $5`,
		settlementID, goodKey, amount, rate, calcTick,
	); err != nil {
		t.Fatalf("seed stale good %s: %v", goodKey, err)
	}
}

func readGood(t *testing.T, settlementID uuid.UUID, goodKey string) (amount, rate float64) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`SELECT amount, rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, goodKey,
	).Scan(&amount, &rate); err != nil {
		t.Fatalf("read good %s: %v", goodKey, err)
	}
	return amount, rate
}

// TestRecomputeProduction_NullsStaleGoodKeepsLiveGood covers AK1 + AK3.
//
// recomputeFixture's catchment is entirely mountain_limestone: the terrain
// produces timber (universal trickle) and stone (unconditional field rule),
// but NOT cedar (cedar only comes from forest_cedar terrain, mig 102). A
// stale cedar rate injected directly must be zeroed by RecomputeProduction,
// with its amount settled at the OLD rate up to the moment of nulling (no
// lost or fabricated stock) — AK1. A good that still has a matching
// production rule (stone) must keep producing and must be stable across a
// second, no-op recompute call — proving the new nulling step causes no
// collateral damage to goods still in the potentials set — AK3.
func TestRecomputeProduction_NullsStaleGoodKeepsLiveGood(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	// pop=400 (not 100): demand (200) must exceed NearjordGrainPerTick (50, P1)
	// for AK4 below to still hold — at pop=100 the flat home-hex trickle alone
	// would exactly cover demand and grainRate would land on 0, not negative.
	settlementID := recomputeFixture(t, tick, /*pop*/ 400, /*grainAmount*/ 50, /*grainRate*/ 0)

	// Stale cedar: settled at rate 42 for 5 ticks before this recompute call.
	seedStaleGood(t, settlementID, "cedar", /*amount*/ 10, /*rate*/ 42, /*calcTick*/ tick-5)
	// P4: stone-on-mountain_limestone has no P3 hexCapacityRule entry (falls
	// back to HexFallbackCap), so it needs an explicit placement to produce
	// anything — place one gubbe on one of the fixture's 6 seeded hexes.
	placeHexGubbe(t, pool, settlementID, 1, hexgrid.Coord{Q: 1, R: 0}, "stone")

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	cedarAmount, cedarRate := readGood(t, settlementID, "cedar")
	if cedarRate != 0 {
		t.Errorf("stale cedar rate must null to exactly 0, got %.4f", cedarRate)
	}
	const wantCedarAmount = 10.0 + 42.0*5 // settled at the OLD rate for the 5 elapsed ticks
	if cedarAmount != wantCedarAmount {
		t.Errorf("cedar amount must settle at the old rate before nulling: want %.4f, got %.4f", wantCedarAmount, cedarAmount)
	}

	stoneAmount1, stoneRate1 := readGood(t, settlementID, "stone")
	if stoneRate1 == 0 {
		t.Fatalf("stone has a live production_rule on mountain_limestone — rate must not be 0")
	}

	// Second call, same tick (no time has passed): a good still in the
	// potentials set must be untouched by the stale-nulling step, and the
	// already-nulled cedar row must stay at 0 (rate <> 0 guard is idempotent).
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (2nd call): %v", err)
	}
	stoneAmount2, stoneRate2 := readGood(t, settlementID, "stone")
	if stoneRate2 != stoneRate1 {
		t.Errorf("stone rate must be stable across an unchanged recompute: was %.6f, now %.6f", stoneRate1, stoneRate2)
	}
	if stoneAmount2 != stoneAmount1 {
		t.Errorf("stone amount must be stable when no ticks elapsed between calls: was %.6f, now %.6f", stoneAmount1, stoneAmount2)
	}
	if _, cedarRate2 := readGood(t, settlementID, "cedar"); cedarRate2 != 0 {
		t.Errorf("cedar rate must remain 0 after a second recompute, got %.4f", cedarRate2)
	}

	// AK4 (also true in this fixture): mountain_limestone has no grain rule,
	// so a populated settlement must still carry a negative grain
	// consumption rate — not zeroed by the new stale-nulling step.
	_, grainRate := readGood(t, settlementID, "grain")
	if grainRate >= 0 {
		t.Errorf("grain consumption rate must stay negative for a non-farming city, got %.4f", grainRate)
	}
}

// TestRecomputeProduction_NullsAllRatesWhenSettlementHasNoProducibleGoods
// covers AK2 — the second leak named in the contract: RecomputeProduction
// used to `return nil` immediately when potentials was empty, skipping the
// settlement entirely and leaving EVERY stale rate untouched. Deleting the
// fixture's catchment map_tiles makes the production_rules JOIN return zero
// rows (potentials == nil), which used to hit that early return.
func TestRecomputeProduction_NullsAllRatesWhenSettlementHasNoProducibleGoods(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	// pop=400 (not 100): see the sibling test above — demand must exceed
	// NearjordGrainPerTick (50, P1) for the AK4 "still negative" assertion.
	settlementID := recomputeFixture(t, tick, /*pop*/ 400, /*grainAmount*/ 5, /*grainRate*/ 0)

	// Strip the catchment bare: no map_tiles at all for this world means the
	// production_rules JOIN in RecomputeProduction returns zero rows.
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT world_id FROM settlements WHERE id = $1`, settlementID,
	).Scan(&worldID); err != nil {
		t.Fatalf("read world id: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id = $1`, worldID); err != nil {
		t.Fatalf("strip catchment: %v", err)
	}

	seedStaleGood(t, settlementID, "cedar", /*amount*/ 10, /*rate*/ 42, /*calcTick*/ tick-3)
	seedStaleGood(t, settlementID, "stone", /*amount*/ 4, /*rate*/ 7, /*calcTick*/ tick-2)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	cedarAmount, cedarRate := readGood(t, settlementID, "cedar")
	if cedarRate != 0 {
		t.Errorf("cedar rate must null to 0 even with zero producible goods, got %.4f", cedarRate)
	}
	const wantCedarAmount = 10.0 + 42.0*3
	if cedarAmount != wantCedarAmount {
		t.Errorf("cedar amount must settle at old rate before nulling: want %.4f, got %.4f", wantCedarAmount, cedarAmount)
	}

	stoneAmount, stoneRate := readGood(t, settlementID, "stone")
	if stoneRate != 0 {
		t.Errorf("stone rate must null to 0 even with zero producible goods, got %.4f", stoneRate)
	}
	const wantStoneAmount = 4.0 + 7.0*2
	if stoneAmount != wantStoneAmount {
		t.Errorf("stone amount must settle at old rate before nulling: want %.4f, got %.4f", wantStoneAmount, stoneAmount)
	}

	// AK4: a populated settlement with literally no catchment tiles still
	// eats — grain's negative consumption rate must survive the same call
	// that zeroed cedar and stone.
	_, grainRate := readGood(t, settlementID, "grain")
	if grainRate >= 0 {
		t.Errorf("grain consumption rate must stay negative even with zero producible goods, got %.4f", grainRate)
	}
}
