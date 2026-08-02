package handlers

// Regression test: applyOracleRevealDeposits (settlement.go) only ever wrote the
// COLONISATION SITE to player_scouted_tiles, never the neighbour hex that actually
// carries the deposit (the sites CTE aggregates BOOL_OR(nb.*_deposit) across all 6
// neighbours via GROUP BY site.q, site.r, so it loses which neighbour it was).
// Consequence: after a successful oracle cast the player gets a coordinate in prose
// ("tin at (47,12)") but the tin hex sitting next to it stays fog forever — /map only
// emits *_deposit fields for tiles whose tier is "live" or "remembered"
// (world.go loadRememberedTiles reads directly from player_scouted_tiles), and the
// deposit-bearing neighbour was never inserted there.
//
// This test seeds a single colonisable site whose ONLY ore-bearing neighbour carries
// tin, calls applyOracleRevealDeposits directly (bypassing kharis/temple/cooldown —
// those are Rite's concerns, not this function's), and asserts:
//   - both the site and the tin neighbour land in player_scouted_tiles (exactly 2 rows)
//   - no unrelated hex is added (a lone tin neighbour must not drag in extra rows)
//   - the payload's "q"/"r" keep meaning the site (unchanged shape), with the ore
//     hex exposed alongside as a new "ore_tiles" field
//   - calling it twice (idempotent retry, e.g. a retried TX) produces no duplicate
//     rows and no error
//
// On the pre-fix code this test fails at the "exactly 2 rows" assertion: it only
// finds 1 row (the site) because the tin neighbour is never persisted.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func TestOracleRevealDeposits_PersistsOreNeighbourHex(t *testing.T) {
	pool := riteTestPool(t) // helper from settlement_rite_offering_test.go
	ctx := context.Background()

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
	username := "oracle-ore-hex-" + uuid.New().String()
	_, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	var playerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM players WHERE username = $1`, username,
	).Scan(&playerID); err != nil {
		t.Fatalf("look up minted player id: %v", err)
	}

	// Caster's own settlement at origin (0,0).
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
		 VALUES ($1, $2, 'Oracle Test Colony', 'akhaier', $3, 'colony', false) RETURNING id`,
		worldID, originProvinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// Colonisable site at (3,0), hex-distance 3 from origin, well inside oracleRadius.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, tin_deposit, silver_deposit)
		 VALUES ($1, 3, 0, 'hills', false, false, false)`,
		worldID,
	); err != nil {
		t.Fatalf("seed candidate site tile: %v", err)
	}
	// Its ONLY ore-bearing neighbour: (4,-1) carries tin. The other 5 neighbours of
	// (3,0) — (2,0),(3,1),(3,-1),(4,0),(2,1) — are left as plain sea/whatever the
	// world default is (not inserted), so if the fix ever grabbed more than the
	// reported ore type it would show up as extra rows in the assertion below.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, tin_deposit, silver_deposit)
		 VALUES ($1, 4, -1, 'mountain_limestone', false, true, false)`,
		worldID,
	); err != nil {
		t.Fatalf("seed tin deposit tile: %v", err)
	}

	spec := religion.PrayerSpecs["akhaier_oracle_deposits"]
	sh := &SettlementHandler{pool: pool}

	runOnce := func() map[string]any {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		payload, msg, err := sh.applyOracleRevealDeposits(ctx, tx, settlementID, worldID, playerID, spec)
		if err != nil {
			t.Fatalf("applyOracleRevealDeposits: %v", err)
		}
		t.Logf("oracle message: %s", msg)
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit tx: %v", err)
		}
		return payload
	}

	payload := runOnce()

	reveals, ok := payload["reveals"].([]map[string]any)
	if !ok || len(reveals) == 0 {
		t.Fatalf("expected at least one reveal, got payload=%+v", payload)
	}
	rv := reveals[0]
	if rv["q"] != 3 || rv["r"] != 0 {
		t.Errorf("expected reveal q/r to still be the colonisable site (3,0), got q=%v r=%v", rv["q"], rv["r"])
	}
	if rv["ore"] != "tin" {
		t.Errorf("expected ore=tin, got %v", rv["ore"])
	}
	if oreTiles, ok := rv["ore_tiles"].([]map[string]any); !ok || len(oreTiles) != 1 {
		t.Errorf("expected exactly one ore_tiles entry (the tin neighbour), got %+v", rv["ore_tiles"])
	} else if oreTiles[0]["q"] != 4 || oreTiles[0]["r"] != -1 {
		t.Errorf("expected ore_tiles[0] = (4,-1), got %+v", oreTiles[0])
	}

	// The crux of the bug: BOTH the site and the ore-bearing neighbour must be
	// scouted. Pre-fix, only the site (3,0) lands here — 1 row, not 2 — because
	// applyOracleRevealDeposits never wrote the deposit-bearing neighbour, only the
	// colonisation site itself, to player_scouted_tiles.
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

	want := [][2]int{{3, 0}, {4, -1}}
	if len(got) != len(want) {
		t.Fatalf("player_scouted_tiles = %v, want exactly %v (site + its tin neighbour, nothing else) — "+
			"the tin hex at (4,-1) is missing from player_scouted_tiles", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("player_scouted_tiles[%d] = %v, want %v", i, got[i], w)
		}
	}

	// Idempotency: re-running (e.g. a retried TX) must not duplicate rows or error.
	runOnce()

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2`,
		worldID, playerID,
	).Scan(&count); err != nil {
		t.Fatalf("count player_scouted_tiles after retry: %v", err)
	}
	if count != 2 {
		t.Fatalf("after idempotent retry, player_scouted_tiles count = %d, want 2 (no duplicates)", count)
	}
}
