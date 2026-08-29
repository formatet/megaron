package economy

// DB integration test for A3 of megaron_plan_flodbudget_och_vadstalle.md: no
// river_ford hex may contribute anything OTHER than fish to a settlement's
// base_potential — the same "water only matches its own terrain_type rule"
// invariant migration 101 established for river (megaron_floden_plan.md §4),
// now proven against river_ford through the real CatchmentBasePotential path
// (catchment.go) rather than a hand-rolled duplicate of its SQL. Gated by
// DATABASE_URL via testPool (sitos_conservation_test.go), same as every other
// DB test in this package. Run against a FRESH clone migrated to 108 — never
// a long-lived local docker DB, which can sit migrations behind (see the
// plan's bevisplan).

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// riverFordCatchmentFixture builds an active world + one settlement whose
// entire 7-hex catchment (all 6 ring neighbours; the centre is deliberately
// left untiled, same convention as recomputeWaterFixture) is river_ford — the
// worst case for a silent-fallback leak, since a universal (terrain_type IS
// NULL) rule like timber's would apply to every one of these hexes if the
// catchment predicate's OR-exception ever went missing for this terrain.
func riverFordCatchmentFixture(t *testing.T) (settlementID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 200) RETURNING id`,
		"test-river-ford-catchment-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"river-ford-catchment-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'river_ford') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	offsets := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	for _, d := range offsets {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, $2, $3, 'river_ford', true)`,
			worldID, d[0], d[1],
		); err != nil {
			t.Fatalf("seed river_ford catchment tile (%d,%d): %v", d[0], d[1], err)
		}
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Porthaven', 'achaean', $3, 'capital', true, 1000) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	return settlementID
}

// TestCatchmentBasePotential_RiverFordOnlyProducesFish is A3: a settlement
// whose entire catchment is river_ford must see EXACTLY ONE good in its base
// potential — fish, at a positive rate — never grain, timber, stone, or any
// other terrain_type-IS-NULL "universal" rule silently leaking through.
func TestCatchmentBasePotential_RiverFordOnlyProducesFish(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := riverFordCatchmentFixture(t)

	potentials, err := CatchmentBasePotential(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("CatchmentBasePotential: %v", err)
	}

	if len(potentials) != 1 {
		t.Fatalf("A3: expected exactly one good (fish) from an all-river_ford catchment, got %d: %v", len(potentials), potentials)
	}
	fishRate, ok := potentials["fish"]
	if !ok {
		t.Fatalf("A3: expected 'fish' in base potentials, got %v", potentials)
	}
	if fishRate <= 0 {
		t.Errorf("A3: fish base potential must be positive for a river_ford catchment, got %v", fishRate)
	}

	// Cross-check: fish's rate must equal river's own rate — migration 108
	// copies it straight out of production_rules (mig 108's own contract),
	// never a re-guessed literal.
	var riverFishRate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate_per_tick FROM production_rules WHERE terrain_type = 'river' AND good_key = 'fish'`,
	).Scan(&riverFishRate); err != nil {
		t.Fatalf("read river's own fish rate: %v", err)
	}
	// 6 catchment hexes each contribute the same per-hex rate.
	wantTotal := riverFishRate * 6
	if diff := fishRate - wantTotal; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("A3: river_ford fish potential = %v, want %v (6 hexes × river's own rate %v)", fishRate, wantTotal, riverFishRate)
	}
}
