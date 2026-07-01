package combat

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

// testPool connects to a real Postgres instance — foundColony is pure SQL
// orchestration across settlements/units/provinces that a mock can't
// meaningfully stand in for. Skips (not fails) when DATABASE_URL isn't set,
// so `go test ./...` stays green in environments without a database.
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

// TestFoundColony_UnitDisbandsIntoPopulace verifies the "colonists become
// citizens, not garrison" fix: after founding a colony, the colonizing unit is
// 'disbanded' (no 'garrison' unit remains for it), and the colony's starting
// population includes the unit's size on top of the baseline.
func TestFoundColony_UnitDisbandsIntoPopulace(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// foundColony's settlement_goods seeding calls current_world_tick(), which
	// reads the single globally-active world (status='active', enforced by a
	// unique partial index) — so this fixture world must be active. It is
	// cleaned up (deleted) at the end of the test, restoring "no active world".
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"colonizer-"+uuid.New().String(), "colonizer-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
		_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id = $1`, ownerID)
	})

	// The owner needs a capital settlement — foundColony looks up the parent/
	// culture from it.
	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital City', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, ownerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// Empty province for the new colony (reused, so foundColony skips the
	// map_tiles lookup path).
	var colonyProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 10, 10, 'plains') RETURNING id`,
		worldID,
	).Scan(&colonyProvinceID); err != nil {
		t.Fatalf("create colony province: %v", err)
	}

	const unitSize = 50
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', $3, 'marching', 10, 10) RETURNING id`,
		worldID, ownerID, unitSize,
	).Scan(&unitID); err != nil {
		t.Fatalf("create colonizing unit: %v", err)
	}

	colonyName := "Newhaven"
	u := unitRow{
		id:         unitID,
		ownerID:    ownerID,
		utype:      "spearman",
		category:   "land",
		size:       unitSize,
		status:     "marching",
		q:          10,
		r:          10,
		colonyName: &colonyName,
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        nil,
		scheduler:  nil,
		clk:        clock.NewTestClock(time.Now()),
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := h.foundColony(ctx, tx, u, colonyProvinceID, 10, 10, worldID); err != nil {
		t.Fatalf("foundColony: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var population int
	if err := pool.QueryRow(ctx,
		`SELECT population FROM settlements WHERE world_id = $1 AND province_id = $2`,
		worldID, colonyProvinceID,
	).Scan(&population); err != nil {
		t.Fatalf("load colony population: %v", err)
	}
	const colonyBasePopulation = 1500
	if population != colonyBasePopulation+unitSize {
		t.Errorf("expected population=%d (baseline+unit size), got %d", colonyBasePopulation+unitSize, population)
	}

	var unitStatus string
	var unitSettlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id FROM units WHERE id = $1`,
		unitID,
	).Scan(&unitStatus, &unitSettlementID); err != nil {
		t.Fatalf("load unit after colonize: %v", err)
	}
	if unitStatus != "disbanded" {
		t.Errorf("expected unit status=disbanded, got %q", unitStatus)
	}

	var garrisonCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM units WHERE world_id = $1 AND status = 'garrison'`,
		worldID,
	).Scan(&garrisonCount); err != nil {
		t.Fatalf("count garrison units: %v", err)
	}
	if garrisonCount != 0 {
		t.Errorf("expected no garrison unit left behind by colonization, got %d", garrisonCount)
	}
}
