package combat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// Vägar V1 (megaron_plan_vagar.md): a march ordered over road edges saves a
// route whose per-step costs are the road-scaled ones (plains 1 × land 0.75 =
// 750 milli), read from the road_edges ROWS — and a march started before the
// edges existed keeps its frozen costs (F3). Fixture: setupLandMarchWorld, the
// mountain forces the detour (0,0)-(0,1)-(1,1)-(2,0).
func TestRoads_StartMarch_SavesRoadScaledCosts(t *testing.T) {
	pool, worldID, ownerID := setupLandMarchWorld(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	newUnit := func() uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
			 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 0) RETURNING id`,
			worldID, ownerID,
		).Scan(&id); err != nil {
			t.Fatalf("create positioned land unit: %v", err)
		}
		return id
	}
	var lastDuration int
	march := func(id uuid.UUID) StoredRoute {
		res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
			WorldID: worldID, PlayerID: ownerID, UnitID: id, TargetQ: 2, TargetR: 0,
		}, nil)
		if err != nil {
			t.Fatalf("StartMarch: %v", err)
		}
		lastDuration = res.DurationTicks
		raw, _, _ := loadMarchRoute(t, pool, id)
		var r StoredRoute
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatalf("unmarshal route: %v", err)
		}
		return r
	}

	firstUnit := newUnit()
	before := march(firstUnit)
	if lastDuration != 3 {
		t.Fatalf("roadless duration = %d ticks, want 3", lastDuration)
	}
	if len(before.Costs) != 3 {
		t.Fatalf("route %v, want the 3-step detour", before.Hexes)
	}
	for i, c := range before.Costs {
		if c != 1000 {
			t.Fatalf("roadless Costs[%d] = %d, want 1000", i, c)
		}
	}

	for _, e := range [][4]int{{0, 0, 0, 1}, {0, 1, 1, 1}, {1, 1, 2, 0}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO road_edges (world_id, q1, r1, q2, r2, built_tick) VALUES ($1,$2,$3,$4,$5,0)`,
			worldID, e[0], e[1], e[2], e[3]); err != nil {
			t.Fatalf("insert road edge %v: %v", e, err)
		}
	}
	after := march(newUnit())
	if len(after.Costs) != 3 {
		t.Fatalf("route %v, want the 3-step detour", after.Hexes)
	}
	if lastDuration != 2 { // 3 × 0.75 = 2.25, rounded to whole ticks
		t.Errorf("road duration = %d ticks, want 2", lastDuration)
	}
	for i, c := range after.Costs {
		if c != 750 {
			t.Errorf("road Costs[%d] = %d, want 750", i, c)
		}
	}

	// F3: the first march's saved route is untouched by the new edges.
	raw, _, _ := loadMarchRoute(t, pool, firstUnit)
	var frozen StoredRoute
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	for i, c := range frozen.Costs {
		if c != 1000 {
			t.Errorf("frozen Costs[%d] = %d after edges were added, want 1000", i, c)
		}
	}
}
