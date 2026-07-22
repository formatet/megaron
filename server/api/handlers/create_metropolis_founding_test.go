package handlers

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/economy"
)

// DB integration test (real Postgres, gated by DATABASE_URL) for the
// building-free founding + Demeter's conditional farm gift
// (megaron_todo #6 "Startbyggnader BORT ur founding"). Founding is pure SQL
// orchestration (catchment potential, RecomputeProduction) a mock can't stand
// in for. Skips when DATABASE_URL isn't set so `go test ./...` stays green.
func foundingTestPool(t *testing.T) *pgxpool.Pool {
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

// foundingCatchmentOffsets is the standard axial catchment order: the centre hex
// followed by its 6 neighbours (mirrors the offsets in createMetropolis).
var foundingCatchmentOffsets = [7][2]int{
	{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1},
}

// foundMetropolisFixture seeds an active world + player + the 7 catchment tiles
// (terrains in standard order, centre first), then raises a metropolis on the
// centre hex through the real createMetropolis path. Returns pool + settlementID.
func foundMetropolisFixture(t *testing.T, terrains [7]string) (*pgxpool.Pool, uuid.UUID) {
	t.Helper()
	pool := foundingTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status='archived' WHERE status='active' AND name LIKE 'test-founding-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-founding-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, worldID) })

	var playerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"founding-"+uuid.New().String(), "founding-"+uuid.New().String()+"@test.invalid",
	).Scan(&playerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	for i, off := range foundingCatchmentOffsets {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, off[0], off[1], terrains[i],
		); err != nil {
			t.Fatalf("seed catchment tile %d: %v", i, err)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	m, err := createMetropolis(ctx, tx, economy.LoadSitosConfig(), metropolisParams{
		WorldID:    worldID,
		PlayerID:   playerID,
		Q:          0,
		R:          0,
		Terrain:    terrains[0],
		Name:       "Foundington-" + uuid.New().String(),
		Culture:    "achaean",
		Population: 4000,
	})
	if err != nil {
		t.Fatalf("createMetropolis: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit founding tx: %v", err)
	}
	return pool, m.SettlementID
}

func foundingHasBuilding(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, bt string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id=$1 AND building_type=$2)`,
		settlementID, bt,
	).Scan(&exists); err != nil {
		t.Fatalf("check building %s: %v", bt, err)
	}
	return exists
}

func foundingLaborWeight(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, good string) (float64, bool) {
	t.Helper()
	var w float64
	err := pool.QueryRow(context.Background(),
		`SELECT weight FROM settlement_labor WHERE settlement_id=$1 AND good_key=$2`,
		settlementID, good,
	).Scan(&w)
	if err != nil {
		return 0, false
	}
	return w, true
}

// TestFounding_WheatCatchment_GrantsDemeterFarmOnly: a metropolis founded on
// wheat-friendly ground (a plains catchment) founds building-free EXCEPT for
// Demeter's farm, with all opening labor on grain and no cult floor.
func TestFounding_WheatCatchment_GrantsDemeterFarmOnly(t *testing.T) {
	terrains := [7]string{"plains", "plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, sid := foundMetropolisFixture(t, terrains)

	if !foundingHasBuilding(t, pool, sid, "farm") {
		t.Error("wheat catchment: expected Demeter's farm, got none")
	}
	for _, bt := range []string{"temple", "lumbermill", "market"} {
		if foundingHasBuilding(t, pool, sid, bt) {
			t.Errorf("wheat catchment: metropolis should found building-free but has a %s", bt)
		}
	}
	if w, ok := foundingLaborWeight(t, pool, sid, "grain"); !ok || w != 1.0 {
		t.Errorf("grain labor weight = %v (present=%v), want 1.0", w, ok)
	}
	if _, ok := foundingLaborWeight(t, pool, sid, "cult"); ok {
		t.Error("cult labor weight should not be seeded (no starter temple)")
	}
}

// TestFounding_BarrenCatchment_GrantsNoFarm: on ground where no farm-compatible
// terrain exists, Demeter grants nothing — the metropolis founds fully
// building-free. The forecast still reads true there (with-farm == building-free).
func TestFounding_BarrenCatchment_GrantsNoFarm(t *testing.T) {
	terrains := [7]string{"mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, sid := foundMetropolisFixture(t, terrains)

	for _, bt := range []string{"farm", "temple", "lumbermill", "market"} {
		if foundingHasBuilding(t, pool, sid, bt) {
			t.Errorf("barren catchment: metropolis should found building-free but has a %s", bt)
		}
	}
	if w, ok := foundingLaborWeight(t, pool, sid, "grain"); !ok || w != 1.0 {
		t.Errorf("grain labor weight = %v (present=%v), want 1.0", w, ok)
	}
}
