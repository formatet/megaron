package economy

// Balansmätning för P1 (megaron_plan_fysisk_gubbemodell.md): catchment
// 7→19 hexar (18 produktiva) mer än fördubblar rå produktionspotential —
// detta mäter HUR mycket, mot en verklig 18-hex-catchment, i stället för att
// gissa. Rött-före enligt planen: "fixtur-stad, assertera 18 produktiva
// hexar bidrar; regressionströskel på produktionssumma."

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// seedFullRingFixture builds a settlement whose ENTIRE 18-hex productive
// ring (hexgrid.Ring at CatchmentRadius, not just the old 6-hex inner ring)
// is seeded with terrain — recomputeFixture and recomputeWaterFixture only
// ever seed the 6 inner-ring slots, which is exactly why they were unaffected
// by the P1 radius flip (the outer 12 hexes they never seed simply
// contribute 0, disk or ring alike). This fixture is the one that actually
// exercises all 18.
func seedFullRingFixture(t *testing.T, currentTick, pop int, terrain string) (settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-p1-balance-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"p1balance-"+uuid.New().String(), "p1balance-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, $2) RETURNING id`,
		worldID, terrain,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	for _, hex := range hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, hex.Q, hex.R, terrain,
		); err != nil {
			t.Fatalf("seed ring tile (%d,%d): %v", hex.Q, hex.R, err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Balanceville', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 0, 0, 1000000, $2)`,
		settlementID, currentTick,
	); err != nil {
		t.Fatalf("seed grain row: %v", err)
	}

	return settlementID
}

// TestP1_AllEighteenRingHexesContribute is the plan's own rött-före: a
// uniform hills catchment (single-hex-rule good, no building needed) must
// show all 18 ring hexes in the base potential — 18× a single hex's rate,
// not 6× (the pre-P1 radius) and not 19× (which would mean the centre hex,
// explicitly excluded, leaked back in).
func TestP1_AllEighteenRingHexesContribute(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	settlementID := seedFullRingFixture(t, tick, /*pop*/ 100, "hills")

	potentials, err := CatchmentBasePotential(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("CatchmentBasePotential: %v", err)
	}

	var singleHillsRate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate_per_tick FROM production_rules WHERE terrain_type = 'hills' AND good_key = 'grain' AND building_type IS NULL`,
	).Scan(&singleHillsRate); err != nil {
		t.Fatalf("read hills grain rate_per_tick: %v", err)
	}

	want18 := 18 * singleHillsRate
	got := potentials["grain"]
	if diff := got - want18; diff > 1e-6*want18 || diff < -1e-6*want18 {
		t.Errorf("grain base potential = %v, want %v (18 × %v — all 18 ring hexes, centre excluded)",
			got, want18, singleHillsRate)
	}
}

// TestP1_ProductionMultiplierVsPreP1Catchment: the plan's own "BALANSCHOCK"
// warning, measured. A catchment with the SAME grain-tile density the old
// 7-hex model's richest reasonable case had (every ring hex is plains) now
// yields a specific, LOGGED multiplier over the pre-P1 6-tile equivalent —
// this is the number megaron_plan_foda_konsistens.md S3
// (grainPerCitizen) and the wider production balance should be read against.
func TestP1_ProductionMultiplierVsPreP1Catchment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 100
	settlementID := seedFullRingFixture(t, tick, /*pop*/ 100, "plains")

	potentials, err := CatchmentBasePotential(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("CatchmentBasePotential: %v", err)
	}

	var singlePlainsRate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate_per_tick FROM production_rules WHERE terrain_type = 'plains' AND good_key = 'grain' AND building_type IS NULL`,
	).Scan(&singlePlainsRate); err != nil {
		t.Fatalf("read plains grain rate_per_tick: %v", err)
	}

	pre := 6 * singlePlainsRate  // old radius-1 ring (6 neighbours)
	post := 18 * singlePlainsRate // new radius-2 ring (P1)
	got := potentials["grain"]
	if diff := got - post; diff > 1e-6*post || diff < -1e-6*post {
		t.Fatalf("grain base potential = %v, want %v (18 × %v)", got, post, singlePlainsRate)
	}
	multiplier := post / pre
	t.Logf("P1 all-plains catchment: pre-P1 (6 ring hexes) = %.1f grain/tick raw potential, "+
		"post-P1 (18 ring hexes) = %.1f grain/tick raw potential — %.1fx (plus the flat "+
		"+%.0f nearjord trickle, unconditional either way, not part of this ratio)",
		pre, post, multiplier, NearjordGrainPerTick)
	if multiplier != 3.0 {
		t.Errorf("multiplier = %.4f, want exactly 3.0 (18/6, both use the identical per-hex rate) — "+
			"if this ever fails, the ring geometry or a production_rule changed under this test", multiplier)
	}
}
