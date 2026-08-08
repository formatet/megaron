package economy

// Röd-först-test för belägring S1+S2 (megaron_plan_belagring.md
// §Implementeringskontrakt): en fiendeenhet i status='positioned' på en
// stads enda landkorridor nekar hexarna bakom den — inte bara hexen den
// själv står på — vilket är precis skillnaden mellan "reachability via en
// riktig graf" och "en enkel grannringskoll" (delbeslut 2). En galär på ett
// havshex intill staden nekar HELA sjö-catchmenten på en gång (delbeslut,
// Timothy 2026-08-07), inte bara det enskilda havshexet. Ingen av delarna är
// FOW-gated (§S1.6).

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// seedSiegeFixture builds a world + capital settlement at (0,0) with a
// population and an owner, WITHOUT seeding any map_tiles for its catchment
// ring — callers seed exactly the hexes their scenario needs. Mirrors
// seedFullRingFixture's world/player/province/settlement setup
// (catchment_p1_balance_test.go) but leaves ring seeding to the caller.
func seedSiegeFixture(t *testing.T, currentTick, pop int) (settlementID, worldID, ownerID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-siege-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"siege-"+uuid.New().String(), "siege-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'hills') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Siegeville', 'achaean', $3, 'capital', true, $4) RETURNING id`,
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
	return settlementID, worldID, ownerID
}

func seedTile(t *testing.T, worldID uuid.UUID, q, r int, terrain string) {
	t.Helper()
	pool := testPool(t)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
		worldID, q, r, terrain,
	); err != nil {
		t.Fatalf("seed tile (%d,%d): %v", q, r, err)
	}
}

func placeGubbe(t *testing.T, settlementID uuid.UUID, ordinal, hexQ, hexR int, goodKey string) {
	t.Helper()
	pool := testPool(t)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
		 VALUES ($1, $2, 'hex', $3, $4, $5)`,
		settlementID, ordinal, hexQ, hexR, goodKey,
	); err != nil {
		t.Fatalf("place gubbe at (%d,%d): %v", hexQ, hexR, err)
	}
}

func placeEnemyUnit(t *testing.T, worldID, enemyOwner uuid.UUID, q, r int) uuid.UUID {
	t.Helper()
	pool := testPool(t)
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 40, 'positioned', $3, $4) RETURNING id`,
		worldID, enemyOwner, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("place enemy unit at (%d,%d): %v", q, r, err)
	}
	return id
}

func removeUnit(t *testing.T, id uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	if _, err := pool.Exec(context.Background(), `DELETE FROM units WHERE id = $1`, id); err != nil {
		t.Fatalf("remove unit %s: %v", id, err)
	}
}

func readGrainRate(t *testing.T, settlementID uuid.UUID) float64 {
	t.Helper()
	pool := testPool(t)
	var rate float64
	if err := pool.QueryRow(context.Background(),
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	).Scan(&rate); err != nil {
		t.Fatalf("read grain rate: %v", err)
	}
	return rate
}

func readBesieged(t *testing.T, settlementID uuid.UUID) bool {
	t.Helper()
	pool := testPool(t)
	var besieged bool
	if err := pool.QueryRow(context.Background(),
		`SELECT besieged FROM settlements WHERE id = $1`, settlementID,
	).Scan(&besieged); err != nil {
		t.Fatalf("read besieged: %v", err)
	}
	return besieged
}

// TestSiege_LandChokepointDeniesHexBehindIt_AndProductionDrops is the plan's
// own röd-först-test (§Implementeringskontrakt): a settlement whose ONLY
// land route to a ring hex passes through a single corridor hex. An enemy
// standing on that corridor (not on the target hex itself) must deny the
// hex behind it — the whole point of a real graph walk over a simple
// "is MY hex enemy-occupied" check (delbeslut 2). Removing the enemy must
// restore full production (delbeslut 3's "hävs" — no lingering state).
func TestSiege_LandChokepointDeniesHexBehindIt_AndProductionDrops(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, ownerID := seedSiegeFixture(t, tick, /*pop*/ 100)

	// Corridor hex (1,0), distance 1 from center — the sole way in.
	seedTile(t, worldID, 1, 0, "hills")
	// Target hex (2,0), distance 2 — its only distance-1 neighbour in the
	// ring is (1,0); its other ring-adjacent neighbours are sealed off below.
	seedTile(t, worldID, 2, 0, "hills")
	seedTile(t, worldID, 1, 1, "mountain_limestone")
	seedTile(t, worldID, 2, -1, "mountain_limestone")

	// One gubbe farming (2,0) — grain is placementYield's uncapped good, so
	// its whole contribution shows up directly in the settled rate once
	// nearjord's flat 50/tick already covers the population's own demand
	// (pop 100 × 0.5/tick = 50 demand — see GrainConsumptionPerTick).
	placeGubbe(t, settlementID, 1, 2, 0, GoodGrain)

	// ── No enemy: full access, hex behind the corridor contributes. ────────
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (no enemy): %v", err)
	}
	if besieged := readBesieged(t, settlementID); besieged {
		t.Fatalf("besieged = true with no enemy nearby, want false")
	}
	rateBefore := readGrainRate(t, settlementID)
	if rateBefore <= 0 {
		t.Fatalf("grain rate = %v with no enemy, want > 0 (nearjord matches demand exactly — the placed gubbe on (2,0) is what must push it positive)", rateBefore)
	}

	// ── Enemy sits on the CORRIDOR (1,0), not on the target hex (2,0). ─────
	enemyOwner := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO players (id, username, email, password_hash) VALUES ($1, $2, $3, 'x')`,
		enemyOwner, "siege-enemy-"+enemyOwner.String(), "siege-enemy-"+enemyOwner.String()+"@test.invalid",
	); err != nil {
		t.Fatalf("create enemy player: %v", err)
	}
	enemyUnitID := placeEnemyUnit(t, worldID, enemyOwner, 1, 0)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (besieged): %v", err)
	}
	if besieged := readBesieged(t, settlementID); !besieged {
		t.Fatalf("besieged = false with an enemy on the sole corridor hex, want true")
	}
	rateBesieged := readGrainRate(t, settlementID)
	if rateBesieged >= rateBefore {
		t.Fatalf("grain rate = %v while besieged, want it to fall below the unbesieged rate %v — the corridor hex being enemy-held must cut off (2,0) even though the enemy never stood ON (2,0)", rateBesieged, rateBefore)
	}
	if rateBesieged > 1e-9 {
		t.Errorf("grain rate = %v while besieged, want ~0 (nearjord exactly covers demand; the placed gubbe's whole contribution should be denied)", rateBesieged)
	}

	// Sanity: the fixture itself is a real chokepoint, not an accident of
	// ReachableCatchmentHexes' own bookkeeping — verify (2,0) is unreachable
	// but the corridor's blocking is the actual cause by checking directly.
	reachable, besieged, err := ReachableCatchmentHexes(ctx, pool, worldID, ownerID,
		hexgrid.Coord{Q: 0, R: 0}, hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius))
	if err != nil {
		t.Fatalf("ReachableCatchmentHexes: %v", err)
	}
	if !besieged {
		t.Fatalf("ReachableCatchmentHexes besieged = false, want true")
	}
	if reachable[hexgrid.Coord{Q: 2, R: 0}] {
		t.Fatalf("(2,0) reported reachable — the corridor block did not propagate through the graph walk")
	}

	// ── Enemy withdraws: access and production must fully recover. ────────
	removeUnit(t, enemyUnitID)
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (lifted): %v", err)
	}
	if besieged := readBesieged(t, settlementID); besieged {
		t.Fatalf("besieged = true after the enemy withdrew, want false")
	}
	rateAfter := readGrainRate(t, settlementID)
	if diff := rateAfter - rateBefore; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("grain rate after the siege lifted = %v, want it to match the pre-siege rate %v exactly", rateAfter, rateBefore)
	}
}

// TestSiege_EnemyGalleyAtHarbourDeniesAllSeaCatchmentAtOnce covers delbeslut
// (Timothy 2026-08-07): a single enemy naval unit holding ANY sea hex next
// to the settlement denies EVERY sea ring hex, not just the one it stands
// on — "hamnen är sjöns pass", not a per-hex blockade.
func TestSiege_EnemyGalleyAtHarbourDeniesAllSeaCatchmentAtOnce(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, ownerID := seedSiegeFixture(t, tick, /*pop*/ 100)
	_ = settlementID

	// Two sea ring hexes: one adjacent to the settlement (the galley sits
	// here), one further out and never touched directly.
	seedTile(t, worldID, 1, 0, "coastal_sea")
	seedTile(t, worldID, 2, -1, "deep_sea")
	// One untouched land hex, to prove the land side is unaffected by a
	// purely naval blockade.
	seedTile(t, worldID, -1, 0, "hills")

	center := hexgrid.Coord{Q: 0, R: 0}
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)

	reachable, besieged, err := ReachableCatchmentHexes(ctx, pool, worldID, ownerID, center, ring)
	if err != nil {
		t.Fatalf("ReachableCatchmentHexes (no enemy): %v", err)
	}
	if besieged {
		t.Fatalf("besieged = true with no enemy, want false")
	}
	if !reachable[hexgrid.Coord{Q: 1, R: 0}] || !reachable[hexgrid.Coord{Q: 2, R: -1}] {
		t.Fatalf("sea hexes not reachable before any enemy is present")
	}

	enemyOwner := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO players (id, username, email, password_hash) VALUES ($1, $2, $3, 'x')`,
		enemyOwner, "siege-galley-"+enemyOwner.String(), "siege-galley-"+enemyOwner.String()+"@test.invalid",
	); err != nil {
		t.Fatalf("create enemy player: %v", err)
	}
	// Galley sits at (1,0) — the OTHER sea hex, (2,-1), is untouched but
	// must still be denied.
	placeEnemyUnit(t, worldID, enemyOwner, 1, 0)

	reachable, besieged, err = ReachableCatchmentHexes(ctx, pool, worldID, ownerID, center, ring)
	if err != nil {
		t.Fatalf("ReachableCatchmentHexes (galley present): %v", err)
	}
	if !besieged {
		t.Fatalf("besieged = false with a galley at the harbour hex, want true")
	}
	if reachable[hexgrid.Coord{Q: 1, R: 0}] {
		t.Fatalf("(1,0) reported reachable — the galley stands directly on it")
	}
	if reachable[hexgrid.Coord{Q: 2, R: -1}] {
		t.Fatalf("(2,-1) reported reachable — a galley anywhere at the harbour must deny EVERY sea ring hex, not just the one it occupies")
	}
	if !reachable[hexgrid.Coord{Q: -1, R: 0}] {
		t.Fatalf("(-1,0) (land) reported unreachable — a purely naval blockade must not touch the land side")
	}
}

// TestSiege_NoEnemyNearby_SkipsGraphWalkEntirely is the "billig förkoll"
// (§S1.2): with no enemy positioned unit anywhere near the settlement,
// ReachableCatchmentHexes must return every ring hex reachable WITHOUT ever
// needing map_tiles data for them — proven here by seeding none at all and
// still getting a full, besieged=false result.
func TestSiege_NoEnemyNearby_SkipsGraphWalkEntirely(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, ownerID := seedSiegeFixture(t, tick, /*pop*/ 100)
	_ = settlementID

	center := hexgrid.Coord{Q: 0, R: 0}
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)

	reachable, besieged, err := ReachableCatchmentHexes(ctx, pool, worldID, ownerID, center, ring)
	if err != nil {
		t.Fatalf("ReachableCatchmentHexes: %v", err)
	}
	if besieged {
		t.Fatalf("besieged = true with no enemy anywhere in the world, want false")
	}
	if len(reachable) != len(ring) {
		t.Fatalf("reachable has %d hexes, want all %d ring hexes (fast path grants full access unconditionally)", len(reachable), len(ring))
	}
}
