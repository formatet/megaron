package handlers

// Prayers of speed (bönesystemet v3 steg 2b): a journey under way is shortened when the prayer
// is answered; scope picks the right journeys; a journey is sped up at most once.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
)

// speedRoute is a v2 route 8 ticks long that started two ticks ago (the fixture clock stands at tick 100).
func (f alphaFixture) speedRoute() movement.StoredRoute {
	cad := time.Duration(tick.TickSeconds) * time.Second
	return movement.StoredRoute{DepartureAt: f.h.clk.Now().Add(-2 * cad), TickNanoseconds: int64(cad), Version: 2, StartTick: 98, EndTick: 108,
		Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}}, Costs: []int64{1000, 1000, 1000, 1000, 1000}}
}

func (f alphaFixture) runner(t *testing.T, sender, origin, dest uuid.UUID) uuid.UUID {
	t.Helper()
	raw, _ := json.Marshal(f.speedRoute())
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,arrives_at,land_route) VALUES($1,$2,$3,$4,'x','outbound',0,0,now(),$5) RETURNING id`,
		f.world, sender, origin, dest, raw).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority) VALUES($1,'MessengerArrivalV2',$2,now(),108,0)`,
		f.world, `{"messenger_id":"`+id.String()+`"}`); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f alphaFixture) ship(t *testing.T, owner, support uuid.UUID) uuid.UUID {
	t.Helper()
	raw, _ := json.Marshal(f.speedRoute())
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,target_q,target_r,depart_tick,arrive_tick,march_route,support_settlement_id) VALUES($1,$2,'galley','naval',1,20,'marching',0,0,5,0,98,108,$3,$4) RETURNING id`,
		f.world, owner, raw, support).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f alphaFixture) caravan(t *testing.T, owner, origin uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	for q := 2; q <= 9; q++ {
		_, _ = f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,'plains') ON CONFLICT DO NOTHING`, f.world, q)
	}
	tx, _ := f.pool.Begin(ctx)
	defer tx.Rollback(ctx)
	id, err := transport.Dispatch(ctx, tx, f.h.scheduler, transport.DispatchParams{WorldID: f.world, OwnerID: owner, OriginID: origin, DestID: uuid.Nil, Kind: "transfer", Category: "land", DestQ: 9, DepartsAt: f.h.clk.Now(), Manifest: transport.Manifest{"grain": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f alphaFixture) arriveTick(t *testing.T, table string, id uuid.UUID) int {
	t.Helper()
	col := map[string]string{"messengers": "(land_route->>'end_tick')::int", "units": "arrive_tick", "transports": "due_tick"}[table]
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT `+col+` FROM `+table+` WHERE id=$1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f alphaFixture) spedUp(t *testing.T) []religion.SpedUp {
	t.Helper()
	var raw []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM events WHERE world_id=$1 AND event_type='RiteCast' ORDER BY id DESC LIMIT 1`, f.world).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Effect struct {
			Sped []religion.SpedUp `json:"sped_up"`
		} `json:"effect"`
	}
	_ = json.Unmarshal(raw, &p)
	return p.Effect.Sped
}

func (f alphaFixture) foreigner(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Scope picks the journeys: one named Runner; the city's Runners and caravans; the realm's. A foreign
// journey is never touched. A journey sped by one prayer is not sped again by the next.
func TestSpeedCast_LandScopesOwnerAndNoStacking(t *testing.T) {
	f := newAlphaFixture(t)
	foreign := f.foreigner(t)
	a := f.runner(t, f.player, f.cities[0], f.cities[1])
	b := f.runner(t, f.player, f.cities[1], f.cities[0])
	theirs := f.runner(t, foreign, f.cities[0], f.cities[1])
	van := f.caravan(t, f.player, f.cities[0])
	vanDue := f.arriveTick(t, "transports", van)
	theirVan := f.caravan(t, foreign, f.cities[0])
	theirVanDue := f.arriveTick(t, "transports", theirVan)

	// 1. named: only Runner a.
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerAsterionSwift, "target_scope": "unit", "target": a.String()}); r.Code != 200 {
		t.Fatalf("named cast %d %s", r.Code, r.Body.String())
	}
	if got := f.arriveTick(t, "messengers", a); got != 106 { // 2 ticks gone, 8 left -> 6
		t.Fatalf("named Runner arrives %d, want 106", got)
	}
	if f.arriveTick(t, "messengers", b) != 108 || f.arriveTick(t, "messengers", theirs) != 108 || f.arriveTick(t, "transports", van) != vanDue {
		t.Fatal("a named blessing reached more than the named Runner")
	}
	if s := f.spedUp(t); len(s) != 1 || s[0].ID != a || s[0].OldArriveTick != 108 || s[0].NewArriveTick != 106 {
		t.Fatalf("RiteCast sped_up %+v", s)
	}
	// someone else's Runner cannot be named
	f.clearRest(t)
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerAsterionSwift, "target_scope": "unit", "target": theirs.String()}); r.Code != 404 {
		t.Fatalf("foreign Runner named: %d %s", r.Code, r.Body.String())
	}

	// 2. the city: caravan (origin city 0) is sped, Runner b (origin city 1) is not, a is not sped again.
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerAsterionSwift, "target_scope": "city"}); r.Code != 200 {
		t.Fatalf("city cast %d %s", r.Code, r.Body.String())
	}
	if f.arriveTick(t, "messengers", a) != 106 {
		t.Fatal("Runner a was sped twice")
	}
	if f.arriveTick(t, "messengers", b) != 108 || f.arriveTick(t, "messengers", theirs) != 108 {
		t.Fatal("a city blessing reached another city's or a foreign Runner")
	}
	if got := f.arriveTick(t, "transports", van); got >= vanDue {
		t.Fatalf("the city's caravan was not sped: %d (was %d)", got, vanDue)
	}
	vanOnce := f.arriveTick(t, "transports", van)

	// 3. the realm: b is sped now; a and the caravan are not sped again; the foreigner is untouched.
	f.clearRest(t)
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerAsterionSwift, "target_scope": "realm"}); r.Code != 200 {
		t.Fatalf("realm cast %d %s", r.Code, r.Body.String())
	}
	if f.arriveTick(t, "messengers", b) != 106 || f.arriveTick(t, "messengers", a) != 106 || f.arriveTick(t, "transports", van) != vanOnce {
		t.Fatal("realm blessing: b should be sped, a and the caravan only once")
	}
	if f.arriveTick(t, "messengers", theirs) != 108 || f.arriveTick(t, "transports", theirVan) != theirVanDue {
		t.Fatal("a foreign Runner or caravan was sped")
	}
	if s := f.spedUp(t); len(s) != 1 || s[0].ID != b {
		t.Fatalf("realm cast sped_up %+v, want only b", s)
	}
}

func TestSpeedCast_SeaShipsOfTheOwnerOnly(t *testing.T) {
	f := newAlphaFixture(t)
	foreign := f.foreigner(t)
	s1 := f.ship(t, f.player, f.cities[0])
	s2 := f.ship(t, f.player, f.cities[1])
	theirs := f.ship(t, foreign, f.cities[0])
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessSwift, "target_scope": "city"}); r.Code != 200 {
		t.Fatalf("city cast %d %s", r.Code, r.Body.String())
	}
	if f.arriveTick(t, "units", s1) != 106 || f.arriveTick(t, "units", s2) != 108 || f.arriveTick(t, "units", theirs) != 108 {
		t.Fatalf("city blessing: s1=%d s2=%d foreign=%d", f.arriveTick(t, "units", s1), f.arriveTick(t, "units", s2), f.arriveTick(t, "units", theirs))
	}
	f.clearRest(t)
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessSwift, "target_scope": "realm"}); r.Code != 200 {
		t.Fatalf("realm cast %d %s", r.Code, r.Body.String())
	}
	if f.arriveTick(t, "units", s1) != 106 || f.arriveTick(t, "units", s2) != 106 || f.arriveTick(t, "units", theirs) != 108 {
		t.Fatal("realm blessing: s1 only once, s2 now, the foreigner never")
	}
	var n int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_events WHERE event_type='UnitArrivalV2' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, s2.String()).Scan(&n)
	if n != 1 {
		t.Fatalf("ship s2 has %d pending arrival jobs, want its one new job", n)
	}
	// a ship cannot be named by a land-speed prayer, and a Runner not by a sea prayer
	f.clearRest(t)
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessSwift, "target_scope": "unit", "target": uuid.NewString()}); r.Code != 404 {
		t.Fatalf("unknown ship: %d", r.Code)
	}
}
