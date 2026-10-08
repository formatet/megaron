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

// stormDice returns v for every Float64 and counts the rolls.
type stormDice struct {
	v     float64
	calls int
}

func (d *stormDice) Float64() float64 { d.calls++; return d.v }
func (d *stormDice) Intn(int) int     { return 0 }

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

// Every entered SEA hex is rolled exactly once (not the start port, not the
// river), a storm costs one hull point, a retried day rolls nothing, and hull
// 0 founders the ship with its embarked troops.
func TestSeaStorm_EachEnteredSeaHexOnceThenFounders(t *testing.T) {
	pool, worldID, owner, route := seaStormWorld(t)
	ship, troops := mkStormFleet(t, pool, worldID, owner, route)
	dice := &stormDice{v: 0}
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = dice

	// Hex q is entered at tick q-1 (hex 1 at departure). By the end of day 3
	// the ship has entered (1,0) (2,0) (3,0 river) (4,0): three sea hexes.
	for day := 1; day <= 3; day++ {
		runStormDay(t, h, worldID, day)
	}
	runStormDay(t, h, worldID, 3) // retry of the same day
	if dice.calls != 3 {
		t.Fatalf("rolls after day 3 = %d, want 3 (start port and river never roll, retry rolls nothing)", dice.calls)
	}
	if status, hull := shipState(t, pool, ship); status != "marching" || hull != 2 {
		t.Fatalf("after three storms: status=%s hull=%d, want marching/2", status, hull)
	}
	if n := countShipEvents(t, pool, ship, EventShipStormDamaged); n != 3 {
		t.Fatalf("ShipStormDamaged events = %d, want 3", n)
	}

	runStormDay(t, h, worldID, 4)
	runStormDay(t, h, worldID, 5)
	if status, hull := shipState(t, pool, ship); status != "disbanded" || hull != 0 {
		t.Fatalf("after fifth storm: status=%s hull=%d, want disbanded/0 (foundered)", status, hull)
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
	if p.OwnerID != owner || p.Hull != 0 || p.Q != 6 || p.Troops == nil || p.Troops.Size != 100 {
		t.Fatalf("founder payload = %+v, want owner, hull 0 at (6,0), 100 drowned troops", p)
	}
	runStormDay(t, h, worldID, 6)
	if dice.calls != 5 {
		t.Fatalf("rolls after founder = %d, want 5 (a sunk ship sails no further)", dice.calls)
	}
}

// Calm seas: rolls happen, nothing changes but the progress mark.
func TestSeaStorm_CalmSeaLeavesShipWhole(t *testing.T) {
	pool, worldID, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, worldID, owner, route)
	dice := &stormDice{v: 0.99}
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = dice
	for day := 1; day <= 10; day++ {
		runStormDay(t, h, worldID, day)
	}
	if dice.calls != 9 {
		t.Fatalf("rolls = %d, want 9 (ten entered hexes minus the river)", dice.calls)
	}
	if status, hull := shipState(t, pool, ship); status != "marching" || hull != 5 {
		t.Fatalf("calm voyage: status=%s hull=%d, want marching/5", status, hull)
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
	h.Dice = &stormDice{v: 0}
	for day := 1; day <= 2; day++ {
		runStormDay(t, h, worldID, day)
	}
	_, fleetHull := shipState(t, pool, fleet)
	_, freightHull := shipState(t, pool, freighter)
	if fleetHull != 3 || freightHull != 3 {
		t.Fatalf("hull after two sea hexes: fleet=%d freighter=%d, want 3/3", fleetHull, freightHull)
	}
	for day := 3; day <= 6; day++ {
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
