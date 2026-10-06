package combat

// Slice B AC1 (megaron_plan_skeppsfart_besattning.md §4): a shorthanded
// galley must take measurably longer to cover the SAME route than a fully
// crewed one — proving the wiring end to end (StartMarch reads units.crew
// from the DB and feeds it into TravelFactor), not just the pure function.
// Before this slice, StartMarch never read the crew column at all, so a
// crew=1 galley and a crew=20 galley over the same 10-hex sea lane got
// identical DurationTicks.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestStartMarch_ShorthandedGalleyIsSlower(t *testing.T) {
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
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"crew-tester-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// A straight 10-hex lane of open coastal sea, q=0..10 at r=0 (consecutive
	// q at fixed r are adjacent hexes — same convention as the explore test).
	for q := 0; q <= 10; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'coastal_sea')`,
			worldID, q,
		); err != nil {
			t.Fatalf("insert map tile (%d,0): %v", q, err)
		}
	}

	// R4 (megaron_plan_skeppsuppdrag_landsatt.md): a naval unit's plain march
	// must now end next to a port of its own — this test measures crew speed,
	// not port validation, so give it a settlement at (11,0), right off the
	// end of the lane, purely to keep the destination (10,0) legal.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 11, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("insert port map tile (11,0): %v", err)
	}
	var portProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 11, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&portProvinceID); err != nil {
		t.Fatalf("create port province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Port', 'achaean', $3, 'capital', true)`,
		worldID, portProvinceID, ownerID,
	); err != nil {
		t.Fatalf("create port settlement: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	dispatch := func(crew int) int {
		var shipID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
			 VALUES ($1, $2, 'war_galley', 'naval', 1, $3, 'positioned', 0, 0) RETURNING id`,
			worldID, ownerID, crew,
		).Scan(&shipID); err != nil {
			t.Fatalf("create positioned galley (crew=%d): %v", crew, err)
		}
		preview, err := PreviewMarch(ctx, pool, clk, MarchOrder{
			WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
			TargetQ: 10, TargetR: 0,
		}, nil)
		if err != nil {
			t.Fatalf("PreviewMarch (crew=%d): %v", crew, err)
		}
		res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
			WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
			TargetQ: 10, TargetR: 0,
		}, nil)
		if err != nil {
			t.Fatalf("StartMarch (crew=%d): %v", crew, err)
		}
		if preview.DurationTicks != res.DurationTicks || preview.ArrivalTick != res.ArrivalTick || !preview.ArrivesAt.Equal(res.ArrivesAt) {
			t.Fatalf("crew=%d preview %+v differs from march %+v", crew, preview, res)
		}
		return res.DurationTicks
	}

	fullCrewTicks := dispatch(50) // war_galley's full crew (unit.CrewFor)
	shorthandedTicks := dispatch(2)

	if shorthandedTicks <= fullCrewTicks {
		t.Errorf("shorthanded galley (crew=2) took %d ticks, full crew (crew=50) took %d — "+
			"a decimated crew must take measurably LONGER over the same route",
			shorthandedTicks, fullCrewTicks)
	}
}
