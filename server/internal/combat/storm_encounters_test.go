package combat

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

func TestStormEnteredHexesActualScan(t *testing.T) {
	for _, tc := range []struct {
		name      string
		qs        []int
		protected bool
		want      int
	}{
		{"intermediate storm", []int{2}, false, 3},
		{"same storm adjacent hexes", []int{1, 2}, false, 3},
		{"different storms same tick", []int{2, 4}, false, 1},
		{"Stillness", []int{2, 4}, true, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, wid, owner, route := seaStormWorld(t)
			route.Hexes = route.Hexes[:5]
			route.Costs = route.Costs[:4]
			route.EndTick = 1
			ship, _ := mkStormFleet(t, pool, wid, owner, route)
			if _, err := pool.Exec(context.Background(), `UPDATE units SET arrive_tick=1 WHERE id=$1`, ship); err != nil {
				t.Fatal(err)
			}
			stormsOverHexes(t, pool, wid, tc.qs...)
			if tc.protected {
				if _, err := pool.Exec(context.Background(), `UPDATE units SET stillness_depart_tick=0,stillness_arrive_tick=1,stillness_owner_id=owner_id WHERE id=$1`, ship); err != nil {
					t.Fatal(err)
				}
			}
			h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
			h.Dice = newStormDice(1)
			runStormDay(t, h, wid, 0)
			runStormDay(t, h, wid, 1)
			runStormDay(t, h, wid, 1)
			if _, hull := shipState(t, pool, ship); hull != tc.want {
				t.Fatalf("hull%d want%d after real entered-hex scan+retry", hull, tc.want)
			}
			wantEvents := (5 - tc.want) / StormDamage
			if n := countShipEvents(t, pool, ship, EventShipStormDamaged); n != wantEvents {
				t.Fatalf("events%d want%d", n, wantEvents)
			}
			var raw []byte
			if err := pool.QueryRow(context.Background(), `SELECT march_route FROM units WHERE id=$1`, ship).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var stored StoredRoute
			if err := json.Unmarshal(raw, &stored); err != nil {
				t.Fatal(err)
			}
			if stored.EndTick != 1 || len(stored.Hexes) != 5 {
				t.Fatalf("frozen route altered %+v", stored)
			}
		})
	}
}

func TestStormEnteredHexesLateScan(t *testing.T) {
	pool, wid, owner, route := seaStormWorld(t)
	route.Hexes = route.Hexes[:5]
	route.Costs = route.Costs[:4]
	route.EndTick = 1
	ship, _ := mkStormFleet(t, pool, wid, owner, route)
	if _, err := pool.Exec(context.Background(), `UPDATE units SET arrive_tick=1,target_q=4 WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	stormsOverHexes(t, pool, wid, 2)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(3)
	runStormDay(t, h, wid, 0)
	runStormDay(t, h, wid, 4)
	runStormDay(t, h, wid, 4)
	if _, hull := shipState(t, pool, ship); hull != 3 {
		t.Fatalf("late scan hull%d want3", hull)
	}
	var tick int
	if err := pool.QueryRow(context.Background(), `SELECT (payload->>'tick')::int FROM events WHERE stream_id=$1 AND event_type=$2`, ship, EventShipStormDamaged).Scan(&tick); err != nil {
		t.Fatal(err)
	}
	if tick != 1 {
		t.Fatalf("late scan event tick%d want original crossed tick1", tick)
	}
}

func TestStormEnteredHexesAllTransportKinds(t *testing.T) {
	for _, kind := range []string{"trade", "transfer", "gift", "trade_return"} {
		t.Run(kind, func(t *testing.T) {
			pool, wid, owner, route := seaStormWorld(t)
			ship, tid := mkStormFreight(t, pool, wid, owner, route)
			j := province.TradeJourney{Category: "naval", TravelTicks: 1, Distance: 4, StepCosts: []int64{167, 167, 167, 167}}
			for q := 0; q <= 4; q++ {
				j.Path = append(j.Path, province.MapPosition{Q: q, R: 0})
			}
			if err := j.Validate(); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(j)
			if _, err := pool.Exec(context.Background(), `UPDATE transports SET kind=$2,due_tick=1,journey=$3 WHERE id=$1`, tid, kind, raw); err != nil {
				t.Fatal(err)
			}
			stormsOverHexes(t, pool, wid, 2)
			h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
			h.Dice = newStormDice(2)
			runStormDay(t, h, wid, 0)
			runStormDay(t, h, wid, 1)
			runStormDay(t, h, wid, 1)
			if _, hull := shipState(t, pool, ship); hull != 3 {
				t.Fatalf("%s intermediate storm hull%d want3", kind, hull)
			}
			var payload SeaStormPayload
			if err := pool.QueryRow(context.Background(), `SELECT payload FROM events WHERE stream_id=$1 AND event_type=$2`, ship, EventShipStormDamaged).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Errand != kind || payload.TransportID == nil || *payload.TransportID != tid || payload.Q != 2 {
				t.Fatalf("transport outcome %+v", payload)
			}
		})
	}
}

func TestStormEnteredHexesContinuityAndSlowShip(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	route := StoredRoute{StartTick: 0, EndTick: 4, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}}, Costs: []int64{1000, 1000}}
	enter, err := RouteEnterMilli(route)
	if err != nil {
		t.Fatal(err)
	}
	graph := province.TileGraph{{0, 0}: "plains", {1, 0}: "deep_sea", {2, 0}: "deep_sea"}
	weather := stormOccupancy{0: {{1, 0}: a}, 1: {{1, 0}: a}, 2: {{1, 0}: b, {2, 0}: b}, 3: {{2, 0}: b}, 4: {{2, 0}: b}}
	got := stormEncounters(route, enter, graph, weather, -1, 4)
	if len(got) != 2 || got[0].tick != 0 || got[0].id != a || got[1].tick != 2 || got[1].id != b {
		t.Fatalf("distinct storm continuity %+v", got)
	}
	// A moving storm enters the slow ship's existing hex without a new route entry.
	weather = stormOccupancy{1: {{1, 0}: a}, 2: {{1, 0}: a}}
	got = stormEncounters(route, enter, graph, weather, 0, 1)
	if len(got) != 1 || got[0].hex != [2]int{1, 0} {
		t.Fatalf("stationary storm contact %+v", got)
	}
	// Fresh scan starts from last tick; it must not replay earlier crossings.
	weather = stormOccupancy{0: {{1, 0}: a}}
	if got := stormEncounters(route, enter, graph, weather, 2, 4); len(got) != 0 {
		t.Fatalf("retroactive hit %+v", got)
	}
}

func TestStormEnteredHexesFractionalBoundariesAndLandArrival(t *testing.T) {
	a := uuid.New()
	route := StoredRoute{StartTick: 3, EndTick: 4, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}}, Costs: []int64{200, 300, 500}}
	enter, err := RouteEnterMilli(route)
	if err != nil {
		t.Fatal(err)
	}
	graph := province.TileGraph{{0, 0}: "plains", {1, 0}: "coastal_sea", {2, 0}: "deep_sea", {3, 0}: "plains"}
	weather := stormOccupancy{4: {{2, 0}: a, {3, 0}: a}}
	got := stormEncounters(route, enter, graph, weather, 3, 4)
	if len(got) != 1 || got[0].tick != 4 || got[0].hex != [2]int{2, 0} {
		t.Fatalf("fractional sea crossing into land %+v", got)
	}
	if got := stormEncounters(route, enter, graph, weather, 4, 5); len(got) != 0 {
		t.Fatalf("past arrival %+v", got)
	}
}
