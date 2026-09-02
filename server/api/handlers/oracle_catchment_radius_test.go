package handlers

// P1 (2026-08-07) raised the economic catchment from radius 1 (7 hexes) to
// radius 2 (19 hexes, 18 worked) — hexgrid.CatchmentRadius. The DATA moved;
// two surfaces did not. One was the map's catchment highlight (fixed
// 2026-09-01, render/catchment-radie-19). The other is this one:
// applyOracleRevealDeposits still asked "does one of the site's SIX axial
// neighbours carry ore?", so it could only ever reveal a colony site with ore
// immediately adjacent.
//
// RED BEFORE this slice: a site whose ore sits exactly 2 hexes away — fully
// inside the real catchment, fully workable once colonised — is invisible to
// the oracle, which answers "no ore deposits lie within reach to reveal".
// That bites the chain gate (CLAUDE.md §Gate 1) precisely at its measured
// bottleneck: the oracle is the ONLY discovery surface for deposits outside a
// settlement's own catchment, and bronze is the step no playtest agent has
// ever reached.
//
// The fix must move BOTH hardcoded neighbour lists in that function — the
// sites CTE and the ore-hex lookup that follows it. This test locks both:
// assertion 1 needs the CTE widened, assertions 2 and 3 need the lookup
// widened too. The negative case (ore 3 hexes out) guards against "fixing" it
// by dropping the bound instead of raising it to CatchmentRadius.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func TestOracleRevealDeposits_UsesCatchmentRadiusNotSixNeighbours(t *testing.T) {
	pool := riteTestPool(t) // helper from settlement_rite_offering_test.go
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "oracle-catchment-" + uuid.New().String()
	if _, _, err := authSvc.Register(ctx, username, "x"); err != nil {
		t.Fatalf("register test player: %v", err)
	}
	var playerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM players WHERE username = $1`, username,
	).Scan(&playerID); err != nil {
		t.Fatalf("look up minted player id: %v", err)
	}

	var originProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&originProvinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Oracle Catchment Colony', 'akhaier', $3, 'colony', false) RETURNING id`,
		worldID, originProvinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	seedTile := func(q, r int, terrain string, cu, sn bool) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, tin_deposit, silver_deposit)
			 VALUES ($1, $2, $3, $4, $5, $6, false)`,
			worldID, q, r, terrain, cu, sn,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", q, r, err)
		}
	}

	// POSITIVE — ore at catchment distance 2, which the old 6-neighbour search
	// cannot see. Site (10,0) is 10 hexes from origin, well inside oracleRadius.
	// Copper sits at (12,0): hex-distance 2 from the site. Nothing is seeded at
	// (11,0), so there is no distance-1 path to this ore at all — a radius-1
	// search finds NOTHING here, which is exactly the red.
	seedTile(10, 0, "hills", false, false)
	seedTile(12, 0, "hills", true, false)

	// NEGATIVE — ore at distance 3, outside the catchment. Site (0,10), tin at
	// (0,13). A colonist at (0,10) could never work this hex, so the oracle must
	// not name it. (0,13) is mountain_limestone and therefore not a colonisable
	// site itself, so it cannot leak in through another route either.
	seedTile(0, 10, "hills", false, false)
	seedTile(0, 13, "mountain_limestone", false, true)

	spec := religion.PrayerSpecs["akhaier_oracle_deposits"]
	sh := &SettlementHandler{pool: pool}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	payload, msg, err := sh.applyOracleRevealDeposits(ctx, tx, settlementID, worldID, playerID, spec)
	if err != nil {
		t.Fatalf("applyOracleRevealDeposits: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	t.Logf("oracle message: %s", msg)

	reveals, _ := payload["reveals"].([]map[string]any)

	// 1. The CTE must find the site whose ore is 2 hexes out.
	var copperReveal map[string]any
	for _, rv := range reveals {
		if rv["ore"] == "copper" {
			copperReveal = rv
		}
		if rv["ore"] == "tin" {
			t.Errorf("oracle revealed tin at (%v,%v) — the only tin is 3 hexes from its site, "+
				"outside hexgrid.CatchmentRadius; a colonist there could never work it", rv["q"], rv["r"])
		}
	}
	if copperReveal == nil {
		t.Fatalf("oracle revealed no copper site — the copper at (12,0) is 2 hexes from the "+
			"colonisable site (10,0), inside the radius-2 catchment P1 established on 2026-08-07. "+
			"Payload=%+v, message=%q", payload, msg)
	}
	if copperReveal["q"] != 10 || copperReveal["r"] != 0 {
		t.Errorf("copper reveal = (%v,%v), want the colonisable site (10,0)",
			copperReveal["q"], copperReveal["r"])
	}

	// 2. The ore-hex lookup after the CTE must use the same radius, or the
	//    revealed site names an ore hex it cannot produce.
	oreTiles, ok := copperReveal["ore_tiles"].([]map[string]any)
	if !ok || len(oreTiles) != 1 {
		t.Fatalf("ore_tiles = %+v, want exactly one entry (the copper hex at (12,0)) — "+
			"the second hardcoded 6-neighbour list must move to CatchmentRadius too",
			copperReveal["ore_tiles"])
	}
	if oreTiles[0]["q"] != 12 || oreTiles[0]["r"] != 0 {
		t.Errorf("ore_tiles[0] = %+v, want (12,0)", oreTiles[0])
	}

	// 3. Both hexes must be scouted: the site the player can settle, and the ore
	//    hex that makes it worth settling. Without the second the map shows a
	//    marker over fog.
	rows, err := pool.Query(ctx,
		`SELECT q, r FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2 ORDER BY q, r`,
		worldID, playerID,
	)
	if err != nil {
		t.Fatalf("query player_scouted_tiles: %v", err)
	}
	var got [][2]int
	for rows.Next() {
		var q, r int
		if err := rows.Scan(&q, &r); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, [2]int{q, r})
	}
	rows.Close()

	want := [][2]int{{10, 0}, {12, 0}}
	if len(got) != len(want) {
		t.Fatalf("player_scouted_tiles = %v, want exactly %v (the site + its copper hex)", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("player_scouted_tiles[%d] = %v, want %v", i, got[i], w)
		}
	}
}
