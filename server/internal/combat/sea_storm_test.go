package combat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stormDice is a deterministic Dice (xorshift) so weather tests replay exactly.
type stormDice struct{ x uint64 }

func newStormDice(seed uint64) *stormDice { return &stormDice{x: seed*2654435761 + 88172645463325252} }
func (d *stormDice) next() uint64 {
	d.x ^= d.x << 13
	d.x ^= d.x >> 7
	d.x ^= d.x << 17
	return d.x
}
func (d *stormDice) Float64() float64 { return float64(d.next()>>11) / float64(1<<53) }
func (d *stormDice) Intn(n int) int   { return int(d.next() % uint64(n)) }

// stormsOverHexes stores one storm hex at each given q on the lane (r=0) for ticks 0..20,
// so a test can leave calm gaps between storms: a gap is what makes two blows of one voyage.
func stormsOverHexes(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, qs ...int) {
	t.Helper()
	ctx := context.Background()
	for _, q := range qs {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO sea_storms (world_id, heading, created_tick) VALUES ($1, 0, 0) RETURNING id`, worldID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for tick := 0; tick <= 20; tick++ {
			if _, err := pool.Exec(ctx, `INSERT INTO sea_storm_track (storm_id, tick, slot, q, r) VALUES ($1, $2, 0, $3, 0)`, id, tick, q); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// seaStormWorld lays out a ten-step lane along r=0: (0,0) is the land port the
// voyage leaves from (never entered), (3,0) is a river (navigable, not sea),
// every other hex up to (10,0) is sea.
func seaStormWorld(t *testing.T) (*pgxpool.Pool, uuid.UUID, uuid.UUID, StoredRoute) {
	t.Helper()
	pool := testPool(t)
	worldID := sweepTestWorld(t)
	ctx := context.Background()
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"storm-"+uuid.New().String()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	route := StoredRoute{StartTick: 0, EndTick: 10}
	for q := 0; q <= 10; q++ {
		terrain := "coastal_sea"
		switch {
		case q == 0:
			terrain = "plains"
		case q == 3:
			terrain = "river"
		case q >= 7:
			terrain = "deep_sea"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, $3)`, worldID, q, terrain); err != nil {
			t.Fatal(err)
		}
		route.Hexes = append(route.Hexes, [2]int{q, 0})
		if q > 0 {
			route.Costs = append(route.Costs, 1000)
		}
	}
	return pool, worldID, owner, route
}

func mkStormFleet(t *testing.T, pool *pgxpool.Pool, worldID, owner uuid.UUID, route StoredRoute) (ship, troops uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'infantry', 'land', 100, 0, 'embarked', 0, 0) RETURNING id`, worldID, owner,
	).Scan(&troops); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(route)
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r,
		                    depart_tick, arrive_tick, march_route, cargo_unit_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'marching', 0, 0, 10, 0, 0, 10, $3, $4) RETURNING id`,
		worldID, owner, raw, troops,
	).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	return ship, troops
}

func mkStormFreight(t *testing.T, pool *pgxpool.Pool, worldID, owner uuid.UUID, route StoredRoute) (ship, transportID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'freighting', 0, 0) RETURNING id`, worldID, owner,
	).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	// A saved journey's costs are caravan-scale (TradeJourney.Validate: ticks =
	// round(1.5 * total / 1000)); 667 per step gives the same ten days.
	j := province.TradeJourney{Category: "naval", TravelTicks: route.EndTick - route.StartTick, Distance: len(route.Hexes) - 1}
	for i, h := range route.Hexes {
		j.Path = append(j.Path, province.MapPosition{Q: h[0], R: h[1]})
		if i > 0 {
			j.StepCosts = append(j.StepCosts, 667)
		}
	}
	if err := j.Validate(); err != nil {
		t.Fatalf("fixture journey: %v", err)
	}
	raw, _ := json.Marshal(j)
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports (world_id, owner_id, kind, category, origin_q, origin_r, dest_q, dest_r,
		                         departs_at, arrives_at, due_tick, status, interceptable, ship_unit_id, journey, departed_tick)
		 VALUES ($1, $2, 'gift', 'naval', 0, 0, 10, 0, now(), now(), 10, 'in_transit', true, $3, $4, 0) RETURNING id`,
		worldID, owner, ship, raw,
	).Scan(&transportID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1, 'silver', 40)`, transportID); err != nil {
		t.Fatal(err)
	}
	return ship, transportID
}

func runStormDay(t *testing.T, h *SeaStormScanHandler, worldID uuid.UUID, day int) {
	t.Helper()
	if err := h.Handle(context.Background(), events.ScheduledEvent{ID: int64(day), WorldID: worldID, DueTick: day}); err != nil {
		t.Fatalf("storm scan day %d: %v", day, err)
	}
}

func shipState(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) (string, int) {
	t.Helper()
	var status string
	var hull int
	if err := pool.QueryRow(context.Background(), `SELECT status, hull FROM units WHERE id = $1`, id).Scan(&status, &hull); err != nil {
		t.Fatal(err)
	}
	return status, hull
}

func countShipEvents(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, kind string) int {
	t.Helper()
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = $2`, id, kind).Scan(&n)
	return n
}

// A ship is hit once per storm it sails into (StormDamage hull points), not once per tick it
// spends inside: lane hexes 1,2 | river 3 | 4,5 | calm 6 | 7 are three storms, so three blows
// and hull 5 founders at the third. A retried day hits nothing twice. (The ship holds hex
// (t+1,0) at tick t: tick 0 → (1,0).)
func TestSeaStorm_OneBlowPerStormThenFounders(t *testing.T) {
	pool, worldID, owner, route := seaStormWorld(t)
	ship, troops := mkStormFleet(t, pool, worldID, owner, route)
	stormsOverHexes(t, pool, worldID, 1, 2, 4, 5, 7)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(1)

	for day := 0; day <= 3; day++ {
		runStormDay(t, h, worldID, day)
	}
	runStormDay(t, h, worldID, 3) // retry of the same day
	if status, hull := shipState(t, pool, ship); status != "marching" || hull != 1 {
		t.Fatalf("after day 3: status=%s hull=%d, want marching/1 (two storms, %d each; the retry nothing)", status, hull, StormDamage)
	}
	if n := countShipEvents(t, pool, ship, EventShipStormDamaged); n != 2 {
		t.Fatalf("ShipStormDamaged events = %d, want 2", n)
	}
	runStormDay(t, h, worldID, 4)
	runStormDay(t, h, worldID, 5)
	if _, hull := shipState(t, pool, ship); hull != 1 {
		t.Fatalf("hull after the second tick inside storm two and a calm hex = %d, want 1", hull)
	}

	runStormDay(t, h, worldID, 6)
	if status, hull := shipState(t, pool, ship); status != "disbanded" || hull != 0 {
		t.Fatalf("after day 6: status=%s hull=%d, want disbanded/0 (third storm, hull clamps at 0)", status, hull)
	}
	if status, _ := shipState(t, pool, troops); status != "disbanded" {
		t.Fatalf("embarked troops status=%s, want disbanded (drowned with the ship)", status)
	}
	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = $2`, ship, EventShipFoundered).Scan(&raw); err != nil {
		t.Fatalf("no ShipFoundered event: %v", err)
	}
	var p SeaStormPayload
	_ = json.Unmarshal(raw, &p)
	if p.OwnerID != owner || p.Hull != 0 || p.HullBefore != 1 || p.Damage != 1 || p.Q != 7 || p.Troops == nil || p.Troops.Size != 100 {
		t.Fatalf("founder payload = %+v, want owner, hull 1→0 (damage 1), at (7,0), 100 drowned troops", p)
	}
	runStormDay(t, h, worldID, 7)
	if n := countShipEvents(t, pool, ship, EventShipFoundered); n != 1 {
		t.Fatalf("ShipFoundered events = %d, want 1 (a sunk ship sails no further)", n)
	}
}

// One storm is one blow however slowly the ship crosses it: a storm that covers lane hexes 4..8
// is five ticks of the same weather and costs StormDamage once.
func TestSeaStorm_LingeringInOneStormIsOneBlow(t *testing.T) {
	pool, worldID, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, worldID, owner, route)
	stormsOverHexes(t, pool, worldID, 4, 5, 6, 7, 8)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(1)
	for day := 0; day <= 7; day++ {
		runStormDay(t, h, worldID, day)
	}
	if status, hull := shipState(t, pool, ship); status != "marching" || hull != 5-StormDamage {
		t.Fatalf("status=%s hull=%d, want marching/%d (five ticks in one storm, one blow)", status, hull, 5-StormDamage)
	}
	if n := countShipEvents(t, pool, ship, EventShipStormDamaged); n != 1 {
		t.Fatalf("ShipStormDamaged events = %d, want 1", n)
	}
}

// Calm seas: no storm over the lane, nothing changes; a small world gets no storms of its own.
func TestSeaStorm_CalmSeaLeavesShipWhole(t *testing.T) {
	pool, worldID, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, worldID, owner, route)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(2)
	for day := 1; day <= 10; day++ {
		runStormDay(t, h, worldID, day)
	}
	if status, hull := shipState(t, pool, ship); status != "marching" || hull != 5 {
		t.Fatalf("calm voyage: status=%s hull=%d, want marching/5", status, hull)
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&n)
	if n != 0 {
		t.Fatalf("storms in a %d-sea-hex world = %d, want 0 (one per %d)", 9, n, SeaHexesPerStorm)
	}
}

// A storm beside the ship, on the neighbouring hex, does nothing.
func TestSeaStorm_NeighbouringHexDoesNotHit(t *testing.T) {
	pool, worldID, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, worldID, owner, route)
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO sea_storms (world_id, heading, created_tick) VALUES ($1, 0, 0) RETURNING id`, worldID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for tick := 0; tick <= 20; tick++ {
		for slot := 0; slot < 3; slot++ {
			// the lane hex of tick t is q = t+1; the storm sits one hex ahead of it
			if _, err := pool.Exec(ctx, `INSERT INTO sea_storm_track (storm_id, tick, slot, q, r) VALUES ($1, $2, $3, $4, 0)`, id, tick, slot, tick+2+slot); err != nil {
				t.Fatal(err)
			}
		}
	}
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(3)
	for day := 1; day <= 8; day++ {
		runStormDay(t, h, worldID, day)
	}
	if _, hull := shipState(t, pool, ship); hull != 5 {
		t.Fatalf("hull = %d, want 5: a storm one hex ahead never shares the ship's hex", hull)
	}
}

// A galley carrying a gift and a war fleet of another Wanax meet the same sea:
// same storms, same hull; the freighter founders with its cargo, and the leg
// is marked foundered so no delivery credits it.
func TestSeaStorm_SameRiskForEveryErrandAndOwner(t *testing.T) {
	pool, worldID, owner, route := seaStormWorld(t)
	var other uuid.UUID
	if err := pool.QueryRow(context.Background(), `INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"storm-other-"+uuid.New().String()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	fleet, _ := mkStormFleet(t, pool, worldID, other, route)
	freighter, transportID := mkStormFreight(t, pool, worldID, owner, route)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	stormsOverHexes(t, pool, worldID, 1, 2, 4, 5, 7)
	h.Dice = newStormDice(4)
	for day := 0; day <= 2; day++ {
		runStormDay(t, h, worldID, day)
	}
	_, fleetHull := shipState(t, pool, fleet)
	_, freightHull := shipState(t, pool, freighter)
	if fleetHull != 3 || freightHull != 3 {
		t.Fatalf("hull after day 2 (first storm crossed): fleet=%d freighter=%d, want 3/3", fleetHull, freightHull)
	}
	for day := 3; day <= 7; day++ {
		runStormDay(t, h, worldID, day)
	}
	var status string
	_ = pool.QueryRow(context.Background(), `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
	if status != "foundered" {
		t.Fatalf("transport status = %q, want foundered", status)
	}
	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = $2`, freighter, EventShipFoundered).Scan(&raw); err != nil {
		t.Fatalf("no ShipFoundered for freighter: %v", err)
	}
	var p SeaStormPayload
	_ = json.Unmarshal(raw, &p)
	assertSameKeysAsWebFixture(t, raw, "ship_foundered.json")
	if p.TransportID == nil || *p.TransportID != transportID || len(p.Cargo) != 1 || p.Cargo[0].Quantity != 40 || p.Errand != "gift" {
		t.Fatalf("freighter founder payload = %+v, want gift transport with 40 silver lost", p)
	}
}

// assertSameKeysAsWebFixture keeps the web formatter's test fixture honest:
// its JSON keys must be exactly those of a payload this handler really
// persisted (a stub that drifts from the server would test nothing).
func assertSameKeysAsWebFixture(t *testing.T, raw []byte, name string) {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "static", "js", "megaron", "ui", "testdata", name))
	if err != nil {
		t.Fatalf("read web fixture: %v", err)
	}
	var got, want map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatal(err)
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("persisted key %q missing from web fixture %s", k, name)
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("web fixture %s key %q is not in the persisted payload", name, k)
		}
	}
}
