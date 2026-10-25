package combat

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestTimedStormContactsDoNotReachFutureHexes(t *testing.T) {
	r := StoredRoute{Version: 2, OffsetMilli: 733, StartTick: 103, EndTick: 104, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}}, Costs: []int64{200, 200, 200}}
	enter, err := RouteEnterMilli(r)
	if err != nil {
		t.Fatal(err)
	}
	storm := uuid.New()
	graph := province.TileGraph{}
	for _, h := range r.Hexes {
		graph[h] = "deep_sea"
	}
	occupied := stormOccupancy{104: map[[2]int]uuid.UUID{{2, 0}: storm, {3, 0}: storm}}
	got, previous := timedStormContacts(r, enter, graph, occupied, 103732, 104399, uuid.Nil)
	if len(got) != 0 {
		t.Fatalf("future storm hit: %+v", got)
	}
	got, previous = timedStormContacts(r, enter, graph, occupied, 104399, 104400, previous)
	if len(got) != 1 || got[0].hex != r.Hexes[2] {
		t.Fatalf("missing reached storm: %+v", got)
	}
	got, _ = timedStormContacts(r, enter, graph, occupied, 104400, 104733, previous)
	if len(got) != 0 {
		t.Fatalf("continuous storm rolled twice: %+v", got)
	}
}

func TestTimedEncounterCatchesCrossingBetweenTickSamples(t *testing.T) {
	testTimedCrossing(t, false)
}
func TestTimedAdvanceStopsBeforeFutureHexMemory(t *testing.T) { testTimedCrossing(t, true) }
func testTimedCrossing(t *testing.T, integrated bool) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	ctx := context.Background()
	pool := testPool(t)
	f := newMarchEncounterFixture(t, pool)
	origin := time.Date(2026, 10, 25, 6, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=104,last_tick_at=$2 WHERE id=$1`, f.worldID, origin.Add(60*time.Second)); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(origin.Add(104 * time.Second))
	first := mkCrossingUnit(t, pool, f.worldID, f.ownerA, clk, 0, 2, 100, nil)
	second := mkCrossingUnit(t, pool, f.worldID, f.ownerB, clk, 2, 0, 100, nil)
	for i, id := range []uuid.UUID{first, second} {
		path := [][2]int{{0, 0}, {1, 0}, {2, 0}}
		if i == 1 {
			path = [][2]int{{2, 0}, {1, 0}, {0, 0}}
		}
		r := StoredRoute{Version: 2, OffsetMilli: 733, StartTick: 103, EndTick: 104, Hexes: path, Costs: []int64{1, 1}}
		raw, _ := json.Marshal(r)
		if _, err := pool.Exec(ctx, `UPDATE units SET depart_tick=103,arrive_tick=104,march_route=$2 WHERE id=$1`, id, raw); err != nil {
			t.Fatal(err)
		}
	}
	h := NewMarchEncounterHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	e := events.ScheduledEvent{ID: 987654, WorldID: f.worldID, DueTick: 104}
	if err := h.timedEncounters(ctx, e, 104232); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadBattleIDAt(t, pool, f.worldID, 1, 0); ok {
		t.Fatal("battle before middle reached")
	}
	if integrated {
		progress := NewPlaceReachScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil)
		if err := progress.advanceTimed(ctx, e); err != nil {
			t.Fatal(err)
		}
		var known bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=2 AND r=0)`, f.worldID, f.ownerA).Scan(&known); err != nil {
			t.Fatal(err)
		}
		if known {
			t.Fatal("stopped army remembered its untravelled future target")
		}
		if err := progress.advanceTimed(ctx, e); err != nil {
			t.Fatal(err)
		}
	} else {

		if err := h.timedEncounters(ctx, e, 104733); err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []uuid.UUID{first, second} {
		status, q, _ := unitStatusPos(t, pool, id)
		if status != "positioned" || q == nil || *q != 1 {
			t.Fatalf("crossing missed: %s %v", status, q)
		}
	}
	if _, ok := loadBattleIDAt(t, pool, f.worldID, 1, 0); !ok {
		t.Fatal("no battle at swept midpoint")
	}
	if err := h.timedEncounters(ctx, e, 104733); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM battles WHERE world_id=$1`, f.worldID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("retry battle count%d err%v", n, err)
	}
}
