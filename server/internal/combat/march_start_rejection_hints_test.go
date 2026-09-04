package combat

// Two of StartMarch's rejections used to be dead ends: the player read
// "it says no" and had no way to learn the verb that actually solves the
// problem, even though that verb exists and is wired up end to end
// (megaron_plan_fyra_smaslices_20260904.md §4b, §4c). A test that only checks
// the HTTP status proves nothing here — the text IS the fix — so both tests
// below assert on the rejection's wording, not just that it rejected.

import (
	"context"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// TestStartMarch_AlreadyMarchingRejectionNamesRedirect covers §4b: a march
// order against a unit that is already marching used to say only "unit
// cannot march: status is 'marching'" — never mentioning that `redirect`
// sends the unit to a new destination without waiting for it to arrive home
// first. The fix names the concrete verb the player can type.
func TestStartMarch_AlreadyMarchingRejectionNamesRedirect(t *testing.T) {
	pool := testPool(t)
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
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"already-marching-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,0,0,'plains'), ($1,1,0,'plains')`,
		worldID); err != nil {
		t.Fatalf("insert map tiles: %v", err)
	}

	// A unit already mid-march (no settlement, in transit) — the exact state
	// the marching-status guard exists to catch.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status)
		 VALUES ($1,$2,'spearman','land',100,0,'marching') RETURNING id`,
		worldID, ownerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 1, TargetR: 0,
	}, nil)
	if err == nil {
		t.Fatal("StartMarch against an already-marching unit succeeded, want a rejection")
	}
	got := err.Error()
	if !strings.Contains(got, "redirect") {
		t.Errorf("already-marching rejection = %q — want it to name \"redirect\" as the way to send "+
			"the unit to a new destination, not just say march is refused", got)
	}
}

// TestStartMarch_NoRouteRejectionNamesLoadNotEmbark covers §4c: when no
// passable land route exists (the map's only route crosses sea), the
// rejection used to say "a sea crossing needs a ship (embark)" — but "embark"
// is not a verb this CLI has; the real one is `load` (unit.CanEmbark gates
// eligibility, but api/handlers/unit.go's Load handler is what the player
// calls). The fix names `load` and its three real conditions (same
// settlement, both garrisoned, coastal/harbour — api/handlers/unit.go:680,
// 640/672, 697).
func TestStartMarch_NoRouteRejectionNamesLoadNotEmbark(t *testing.T) {
	pool := testPool(t)
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
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"no-route-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// Capital at (0,0), a strip of sea at (1,0), and an isolated land hex at
	// (2,0) that is only reachable by crossing that sea — no other tile
	// exists, so a land unit has no route across.
	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "coastal_sea"}, {2, 0, "plains"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4)`,
			worldID, tl.q, tl.r, tl.terrain); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1,0,0,'plains') RETURNING id`,
		worldID).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1,$2,'Capital City','achaean',$3,'capital',true) RETURNING id`,
		worldID, capitalProvinceID, ownerID).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1,$2,'spearman','land',100,0,'garrison',$3) RETURNING id`,
		worldID, ownerID, capitalID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create garrisoned land unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 2, TargetR: 0,
	}, nil)
	if err == nil {
		t.Fatal("StartMarch across an unreachable sea gap succeeded, want a rejection")
	}
	got := err.Error()
	if !strings.Contains(got, "load") {
		t.Errorf("no-route rejection = %q — want it to name the real verb \"load\"", got)
	}
	if strings.Contains(got, "(embark)") {
		t.Errorf("no-route rejection = %q — still names \"embark\", which is not a verb this CLI has", got)
	}
	for _, cond := range []string{"settlement", "garrison", "coastal"} {
		if !strings.Contains(got, cond) {
			t.Errorf("no-route rejection = %q — want it to state the %q condition for loading", got, cond)
		}
	}
}
