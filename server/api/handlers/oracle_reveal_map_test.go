package handlers

// The oracle rite as of Timothy's canon decision 2026-09-02: reveal SEVEN hexes
// of map — an ore hex anywhere in the world plus its six neighbours — and lift
// the fog from all of them. The single rule is that the gods never point at ore
// the player has already discovered.
//
// This file replaces three tests that locked the mechanic that decision removed
// (a nearest COLONISABLE SITE within oracleRadius=20 whose catchment held ore):
// oracle_radius_test.go, oracle_reveal_ore_hex_test.go and
// oracle_catchment_radius_test.go. Nothing they asserted still exists to assert.
// A fourth, settlement_test.go's TestOracleRevealPayloadShape, went with them
// for a different reason: it built its own payload literal and asserted against
// that literal, never calling the function, so it locked nothing at all.
//
// The draw is random, so these tests pin PROPERTIES, never which hex came up.
// Each fixture is shaped so that the property is decidable no matter what the
// roll returns — one candidate, or two where one is already known.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// oracleMapFixture builds a world with one owned settlement and returns the ids
// needed to call applyOracleRevealDeposits directly (kharis, temple and cooldown
// are Rite's concerns, not this function's).
type oracleMapFixture struct {
	pool         *pgxpool.Pool
	worldID      uuid.UUID
	playerID     uuid.UUID
	settlementID uuid.UUID
}

func setupOracleMap(t *testing.T) (oracleMapFixture, func(q, r int, terrain string, cu, sn, ag bool)) {
	t.Helper()
	pool := riteTestPool(t) // helper from settlement_rite_offering_test.go
	ctx := context.Background()
	f := oracleMapFixture{pool: pool}

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "oracle-map-" + uuid.New().String()
	if _, _, err := authSvc.Register(ctx, username, "x"); err != nil {
		t.Fatalf("register test player: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM players WHERE username = $1`, username).Scan(&f.playerID); err != nil {
		t.Fatalf("look up minted player id: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Oracle Map Colony', 'akhaier', $3, 'colony', false) RETURNING id`,
		f.worldID, provinceID, f.playerID,
	).Scan(&f.settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	seed := func(q, r int, terrain string, cu, sn, ag bool) {
		t.Helper()
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, tin_deposit, silver_deposit)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			f.worldID, q, r, terrain, cu, sn, ag,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", q, r, err)
		}
	}
	return f, seed
}

// seedDisk seeds the centre and every hex within distance 1 of it, so a reveal
// patch centred there has all seven tiles to find.
func seedDisk(seed func(int, int, string, bool, bool, bool), cq, cr int, centreTerrain string, cu, sn, ag bool) {
	seed(cq, cr, centreTerrain, cu, sn, ag)
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
		seed(cq+d[0], cr+d[1], "deep_sea", false, false, false)
	}
}

func castOracle(t *testing.T, f oracleMapFixture) (map[string]any, string) {
	t.Helper()
	pool := f.pool
	ctx := context.Background()
	spec := religion.PrayerSpecs["akhaier_oracle_deposits"]
	sh := &SettlementHandler{pool: pool}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	payload, msg, err := sh.applyOracleRevealDeposits(ctx, tx, f.settlementID, f.worldID, f.playerID, spec)
	if err != nil {
		t.Fatalf("applyOracleRevealDeposits: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	return payload, msg
}

func scoutedTiles(t *testing.T, f oracleMapFixture) [][2]int {
	t.Helper()
	rows, err := f.pool.Query(context.Background(),
		`SELECT q, r FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2 ORDER BY q, r`,
		f.worldID, f.playerID)
	if err != nil {
		t.Fatalf("query player_scouted_tiles: %v", err)
	}
	defer rows.Close()
	var out [][2]int
	for rows.Next() {
		var q, r int
		if err := rows.Scan(&q, &r); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, [2]int{q, r})
	}
	return out
}

// Property 1: the centre of the reveal carries ore, and the patch is the centre
// plus its six neighbours — seven hexes, fog lifted on all of them. The ore hex
// is deliberately placed 40 hexes from the caster, far outside any radius the
// old mechanic had, to prove reach no longer gates the rite.
func TestOracle_RevealsSevenHexesCentredOnOre(t *testing.T) {
	f, seed := setupOracleMap(t)
	seedDisk(seed, 40, 0, "hills", true, false, false)

	payload, msg := castOracle(t, f)
	t.Logf("oracle message: %s", msg)

	ore, ok := payload["revealed_ore"].(map[string]any)
	if !ok {
		t.Fatalf("payload has no revealed_ore: %+v (message %q)", payload, msg)
	}
	if ore["q"] != 40 || ore["r"] != 0 {
		t.Errorf("revealed_ore = (%v,%v), want the only ore hex on the map, (40,0) — "+
			"distance 40 from the caster, so reach must not gate the rite", ore["q"], ore["r"])
	}
	if ore["ore"] != "copper" {
		t.Errorf("revealed_ore.ore = %v, want copper", ore["ore"])
	}

	tiles, _ := payload["revealed_tiles"].([]map[string]any)
	if len(tiles) != 7 {
		t.Errorf("revealed_tiles has %d entries, want 7 (centre + six neighbours)", len(tiles))
	}
	if got := scoutedTiles(t, f); len(got) != 7 {
		t.Errorf("player_scouted_tiles = %v (%d rows), want 7 — the fog must lift on the whole patch, "+
			"not just the ore hex", got, len(got))
	}
}

// Property 2: the one rule. A deposit the player has already discovered is never
// pointed at again. Two ore hexes exist; one is already scouted, so the draw has
// exactly one legal answer and the assertion is decidable despite the randomness.
func TestOracle_NeverPointsAtAlreadyDiscoveredOre(t *testing.T) {
	f, seed := setupOracleMap(t)
	seedDisk(seed, 40, 0, "hills", true, false, false)  // already known
	seedDisk(seed, 10, 30, "hills", false, true, false) // the only legal answer

	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1,$2,40,0)`,
		f.worldID, f.playerID,
	); err != nil {
		t.Fatalf("pre-scout the known ore hex: %v", err)
	}

	payload, msg := castOracle(t, f)
	t.Logf("oracle message: %s", msg)

	ore, ok := payload["revealed_ore"].(map[string]any)
	if !ok {
		t.Fatalf("payload has no revealed_ore: %+v (message %q)", payload, msg)
	}
	if ore["q"] == 40 && ore["r"] == 0 {
		t.Fatalf("the oracle pointed at (40,0), which the player had already discovered — " +
			"the gods never show ore on an already-discovered hex")
	}
	if ore["q"] != 10 || ore["r"] != 30 {
		t.Errorf("revealed_ore = (%v,%v), want the only undiscovered ore hex, (10,30)", ore["q"], ore["r"])
	}
}

// Property 3: when every ore hex is already known the rite answers honestly
// instead of erroring or silently handing back something the player has. A paid
// rite must never be a silent no-op.
func TestOracle_AllOreKnownAnswersHonestly(t *testing.T) {
	f, seed := setupOracleMap(t)
	seedDisk(seed, 40, 0, "hills", true, false, false)

	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1,$2,40,0)`,
		f.worldID, f.playerID,
	); err != nil {
		t.Fatalf("pre-scout the only ore hex: %v", err)
	}

	payload, msg := castOracle(t, f)
	t.Logf("oracle message: %s", msg)

	if ore := payload["revealed_ore"]; ore != nil {
		t.Errorf("revealed_ore = %+v, want nil — every ore hex was already known", ore)
	}
	if msg == "" {
		t.Error("the rite returned an empty message; a paid rite must say what happened")
	}
	if got := scoutedTiles(t, f); len(got) != 1 {
		t.Errorf("player_scouted_tiles = %v, want only the pre-scouted hex — nothing new may be revealed", got)
	}
}

// Property 4: the patch reveals only hexes that exist. At the map's edge there
// is no row beyond the boundary, so fewer than seven come back — and no phantom
// coordinate is ever written into the player's map memory. (The sea is NOT such
// a case: sea hexes are ordinary map_tiles rows and are revealed normally, as
// property 1 covers with its deep_sea neighbours.)
func TestOracle_RevealsOnlyHexesThatExist(t *testing.T) {
	f, seed := setupOracleMap(t)
	// Ore at the corner: only three of the six neighbours exist as rows.
	seed(0, 0, "hills", true, false, false)
	seed(1, 0, "deep_sea", false, false, false)
	seed(0, 1, "deep_sea", false, false, false)
	seed(1, -1, "deep_sea", false, false, false)

	payload, msg := castOracle(t, f)
	t.Logf("oracle message: %s", msg)

	tiles, _ := payload["revealed_tiles"].([]map[string]any)
	if len(tiles) != 4 {
		t.Errorf("revealed_tiles has %d entries, want 4 (the ore hex + the three neighbours that exist)", len(tiles))
	}
	for _, got := range scoutedTiles(t, f) {
		if got == [2]int{0, -1} || got == [2]int{-1, 0} || got == [2]int{-1, 1} {
			t.Errorf("player_scouted_tiles contains %v, a coordinate with no map_tiles row — "+
				"the reveal must not invent map", got)
		}
	}
}
