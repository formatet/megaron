package province

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to a real Postgres instance — SettlementCatchmentOverlap
// joins settlements/provinces/map_tiles, which a mock can't meaningfully stand
// in for. Skips (not fails) when DATABASE_URL isn't set, so `go test ./...`
// stays green without a database. Mirrors internal/combat's testPool
// (unit_arrival_colonize_test.go).
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// testWorld creates a throwaway world for a catchment-overlap test.
// SettlementCatchmentOverlap never reads current_world_tick() (it only joins
// settlements/provinces/map_tiles) — unlike internal/combat or internal/kharis
// DB tests, it does NOT need status='active', so this deliberately uses
// 'forming' and never touches the one_active_world unique index (no race with
// concurrently-run packages). Deleting the world cascades provinces and
// settlements (ON DELETE CASCADE).
func testWorld(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'forming') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM worlds WHERE id = $1`, worldID)
	})
	return worldID
}

func testOwner(t *testing.T, pool *pgxpool.Pool, tag string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
	).Scan(&id); err != nil {
		t.Fatalf("create test player %s: %v", tag, err)
	}
	return id
}

// seedSettlement inserts a settlement (and its province) at (q, r), owned by
// ownerID, in the given state — the raw-SQL shape every existing settlement
// fixture in this codebase uses (e.g. unit_arrival_sack_test.go), which
// deliberately bypasses createMetropolis/foundColony/StartMarch and therefore
// this slice's gate entirely, exactly like a pre-existing world would.
func seedSettlement(t *testing.T, pool *pgxpool.Pool, worldID, ownerID uuid.UUID, q, r int, name, state string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		worldID, q, r,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province (%d,%d): %v", q, r, err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, $5) RETURNING id`,
		worldID, provinceID, name, ownerID, state,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement %q at (%d,%d): %v", name, q, r, err)
	}
	return settlementID
}

// TestSettlementCatchmentOverlap_Distance1Blocked: adjacent centres share 5 of
// their 7 catchment hexes — the most obvious case.
func TestSettlementCatchmentOverlap_Distance1Blocked(t *testing.T) {
	pool := testPool(t)
	worldID := testWorld(t, pool)
	owner := testOwner(t, pool, "owner")
	existing := seedSettlement(t, pool, worldID, owner, 0, 0, "Mykene", "active")

	conflict, err := SettlementCatchmentOverlap(context.Background(), pool, worldID, 1, 0)
	if err != nil {
		t.Fatalf("SettlementCatchmentOverlap: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected a conflict at hex distance 1")
	}
	if conflict.SettlementID != existing {
		t.Errorf("expected conflict to name Mykene (%s), got %s", existing, conflict.SettlementID)
	}
}

// TestSettlementCatchmentOverlap_Distance2Blocked is the case the founding
// contract calls out explicitly: at centre-distance 2 the two catchments
// still share 2 hexes ("delar 2 hexar" — the original misunderstanding this
// invariant fixes). Existing at (0,0), candidate at (2,-1): distance 2, and
// their rings share (1,-1) and (1,0).
func TestSettlementCatchmentOverlap_Distance2Blocked(t *testing.T) {
	pool := testPool(t)
	worldID := testWorld(t, pool)
	owner := testOwner(t, pool, "owner")
	seedSettlement(t, pool, worldID, owner, 0, 0, "Mykene", "active")

	if got := HexDistance(MapPosition{Q: 0, R: 0}, MapPosition{Q: 2, R: -1}); got != 2 {
		t.Fatalf("test setup: expected hex distance 2, got %d", got)
	}

	conflict, err := SettlementCatchmentOverlap(context.Background(), pool, worldID, 2, -1)
	if err != nil {
		t.Fatalf("SettlementCatchmentOverlap: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected a conflict at hex distance 2 — the two catchments still share 2 hexes")
	}
}

// TestSettlementCatchmentOverlap_Distance4StillBlocked: P1 doubled the
// catchment radius to 2 (megaron_plan_fysisk_gubbemodell.md) — distance 3,
// the old radius-1 clearance boundary, must now still conflict.
func TestSettlementCatchmentOverlap_Distance4StillBlocked(t *testing.T) {
	pool := testPool(t)
	worldID := testWorld(t, pool)
	owner := testOwner(t, pool, "owner")
	existing := seedSettlement(t, pool, worldID, owner, 0, 0, "Mykene", "active")

	conflict, err := SettlementCatchmentOverlap(context.Background(), pool, worldID, 4, 0)
	if err != nil {
		t.Fatalf("SettlementCatchmentOverlap: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected a conflict at hex distance 4 — radius-2 catchments still touch there")
	}
	if conflict.SettlementID != existing {
		t.Errorf("expected conflict to name Mykene (%s), got %s", existing, conflict.SettlementID)
	}
}

// TestSettlementCatchmentOverlap_Distance5Allowed: centre-distance 5 is the
// first distance at which two radius-2 catchments (P1, 19 hexes each) stop
// touching — the site must be clear.
func TestSettlementCatchmentOverlap_Distance5Allowed(t *testing.T) {
	pool := testPool(t)
	worldID := testWorld(t, pool)
	owner := testOwner(t, pool, "owner")
	seedSettlement(t, pool, worldID, owner, 0, 0, "Mykene", "active")

	conflict, err := SettlementCatchmentOverlap(context.Background(), pool, worldID, 5, 0)
	if err != nil {
		t.Fatalf("SettlementCatchmentOverlap: %v", err)
	}
	if conflict != nil {
		t.Fatalf("expected no conflict at hex distance 5, got conflict with settlement %s", conflict.SettlementID)
	}
}

// TestSettlementCatchmentOverlap_ForeignOwnerBlocksEqually: the invariant is
// owner-agnostic (Timothy 2026-07-27/28 — "gäller ALLA städer oavsett
// ägare") — a foreign settlement blocks exactly as hard as the founding
// player's own would. SettlementCatchmentOverlap doesn't even take a
// candidate-owner parameter; this proves the query itself never filters by
// owner_id.
func TestSettlementCatchmentOverlap_ForeignOwnerBlocksEqually(t *testing.T) {
	pool := testPool(t)
	worldID := testWorld(t, pool)
	victim := testOwner(t, pool, "victim")
	seedSettlement(t, pool, worldID, victim, 5, 5, "Foreign City", "active")

	conflict, err := SettlementCatchmentOverlap(context.Background(), pool, worldID, 6, 5)
	if err != nil {
		t.Fatalf("SettlementCatchmentOverlap: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected the foreign-owned settlement to block just as hard as an own one")
	}
	if conflict.OwnerID == nil || *conflict.OwnerID != victim {
		t.Errorf("expected conflict owner to be the foreign player %s, got %v", victim, conflict.OwnerID)
	}
}

// TestSettlementCatchmentOverlap_DeadStateSettlementDoesNotBlock: collapse,
// razing, sinking and abandonment all free the province (combat/collapse.go,
// combat/sack.go, api/handlers/settlement.go Abandon) — a settlement in one
// of these states is a ruin, not a farm, and must not block a new founding
// nearby. This is also what keeps grandfathering from becoming a permanent
// soft-lock: a player's own collapsed capital cannot wall off its own hex
// forever.
func TestSettlementCatchmentOverlap_DeadStateSettlementDoesNotBlock(t *testing.T) {
	for _, state := range []string{"collapsed", "razed", "sunk", "abandoned"} {
		t.Run(state, func(t *testing.T) {
			pool := testPool(t)
			worldID := testWorld(t, pool)
			owner := testOwner(t, pool, "owner")
			ruin := seedSettlement(t, pool, worldID, owner, 0, 0, "Ruin", state)

			conflict, err := SettlementCatchmentOverlap(context.Background(), pool, worldID, 1, 0)
			if err != nil {
				t.Fatalf("SettlementCatchmentOverlap: %v", err)
			}
			if conflict != nil {
				t.Fatalf("a %s settlement (%s) must not block a new founding nearby, got conflict %s",
					state, ruin, conflict.SettlementID)
			}
		})
	}
}

// TestSettlementCatchmentOverlap_ExistingOverlapNeverRetroactivelyFlagged is
// the grandfather clause (Timothy 2026-07-28): the gate blocks FOUNDING,
// never existence. Seeds two settlements at hex distance 1 — an overlap that
// should never have been allowed to found, created directly the way every
// settlement fixture in this codebase does (bypassing createMetropolis/
// foundColony/StartMarch), standing in for pre-existing world data. Proves
// two things: (1) the pre-existing overlap does not "poison" an unrelated,
// genuinely clear candidate elsewhere in the world, and (2) the check never
// mutates the rows it reads — it is a pure read, so nothing anywhere could
// retroactively punish the grandfathered pair by construction.
func TestSettlementCatchmentOverlap_ExistingOverlapNeverRetroactivelyFlagged(t *testing.T) {
	pool := testPool(t)
	worldID := testWorld(t, pool)
	ownerA := testOwner(t, pool, "a")
	ownerB := testOwner(t, pool, "b")

	settlementA := seedSettlement(t, pool, worldID, ownerA, 0, 0, "Old Capital A", "active")
	settlementB := seedSettlement(t, pool, worldID, ownerB, 1, 0, "Old Capital B", "active")

	conflict, err := SettlementCatchmentOverlap(context.Background(), pool, worldID, 20, 20)
	if err != nil {
		t.Fatalf("SettlementCatchmentOverlap: %v", err)
	}
	if conflict != nil {
		t.Fatalf("a clear site far from the grandfathered pair must not be blocked, got conflict %s", conflict.SettlementID)
	}

	var stateA, stateB string
	if err := pool.QueryRow(context.Background(), `SELECT state FROM settlements WHERE id = $1`, settlementA).Scan(&stateA); err != nil {
		t.Fatalf("reload settlement A: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT state FROM settlements WHERE id = $1`, settlementB).Scan(&stateB); err != nil {
		t.Fatalf("reload settlement B: %v", err)
	}
	if stateA != "active" || stateB != "active" {
		t.Fatalf("grandfathered pair must remain untouched by the check: A=%q B=%q", stateA, stateB)
	}
}

// TestCatchmentClearanceHexes is the pure "how far to move" arithmetic the
// founding error/preview messages use.
func TestCatchmentClearanceHexes(t *testing.T) {
	// safeCentreDistance = 2*CatchmentRadius+1 = 5 (P1, radius 2).
	cases := []struct{ dist, want int }{
		{0, 5}, {1, 4}, {2, 3}, {3, 2}, {4, 1}, {5, 0}, {7, 0},
	}
	for _, tc := range cases {
		if got := CatchmentClearanceHexes(tc.dist); got != tc.want {
			t.Errorf("CatchmentClearanceHexes(%d) = %d, want %d", tc.dist, got, tc.want)
		}
	}
}
