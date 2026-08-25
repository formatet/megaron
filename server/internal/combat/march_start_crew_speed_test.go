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
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"crew-tester-"+uuid.New().String(), "crew-tester-"+uuid.New().String()+"@test.invalid",
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
		res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
			WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
			TargetQ: 10, TargetR: 0,
		}, nil)
		if err != nil {
			t.Fatalf("StartMarch (crew=%d): %v", crew, err)
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
