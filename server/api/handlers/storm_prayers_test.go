package handlers

// Stormbönerna 3b, Rite side: Still the Storm (a named storm on your storm map) and Wake the Storm
// (a sea hex of your own map). Targets are validated before the gift is taken.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/religion"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// seaFor lays sea tiles q 2..9 on rows 3..5 (inside the fixture's reach) and one far away at (30,30).
func (f alphaFixture) seaFor(t *testing.T) {
	t.Helper()
	for q := 2; q <= 9; q++ {
		for r := 3; r <= 5; r++ {
			if _, err := f.pool.Exec(context.Background(), `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,'deep_sea') ON CONFLICT DO NOTHING`, f.world, q, r); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, _ = f.pool.Exec(context.Background(), `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,30,30,'deep_sea') ON CONFLICT DO NOTHING`, f.world)
}

func (f alphaFixture) know(t *testing.T, q, r int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, f.world, f.player, q, r); err != nil {
		t.Fatal(err)
	}
}

// namedStorm stores a living 3-hex storm on row 4, with a track row at the world tick, and (optionally) the
// caster's sighting of it.
func (f alphaFixture) namedStorm(t *testing.T, name string, seen bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	var nm any
	if name != "" {
		nm = name
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO sea_storms(world_id,heading,created_tick,dies_tick,name) VALUES($1,0,0,100000,$2) RETURNING id`, f.world, nm).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for slot, q := range []int{4, 5, 6} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES($1,100,$2,$3,4)`, id, slot, q); err != nil {
			t.Fatal(err)
		}
	}
	if seen {
		if _, err := f.pool.Exec(ctx, `INSERT INTO player_storm_sightings(player_id,storm_id,seen_tick,hexes) VALUES($1,$2,99,'[{"q":4,"r":4},{"q":5,"r":4},{"q":6,"r":4}]')`, f.player, id); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = f.pool.Exec(ctx, `UPDATE worlds SET storm_weather_tick=100 WHERE id=$1`, f.world)
	return id
}

func (f alphaFixture) trackLen(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM sea_storm_track WHERE storm_id=$1 AND tick=100`, id).Scan(&n)
	return n
}

func (f alphaFixture) lastRite(t *testing.T) map[string]any {
	t.Helper()
	var raw []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM events WHERE world_id=$1 AND event_type='RiteCast' ORDER BY id DESC LIMIT 1`, f.world).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	return p
}

func TestStillTheStorm_ShrinksANamedStormOnYourMapAndHidesWhereItIsNow(t *testing.T) {
	f := newAlphaFixture(t)
	f.seaFor(t)
	id := f.namedStorm(t, "Old Grey", true)
	fish, oil := f.stock(t, f.cities[0], "fish"), f.stock(t, f.cities[0], "oil")
	rec := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessCalm, "target": "old grey"})
	if rec.Code != 200 {
		t.Fatalf("cast %d %s", rec.Code, rec.Body.String())
	}
	if n := f.trackLen(t, id); n != 2 {
		t.Fatalf("storm has %d hexes, want 2", n)
	}
	if got := fish - f.stock(t, f.cities[0], "fish"); got != 10 {
		t.Fatalf("fish paid %v, want 10", got)
	}
	if got := oil - f.stock(t, f.cities[0], "oil"); got != 6 {
		t.Fatalf("oil paid %v, want 6", got)
	}
	// The Wanax is told the storm shrank, never where it stands now.
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	shown := resp["effect"].(map[string]any)
	if shown["shrunk"] != true || shown["removed_hex"] != nil || shown["hexes"] != nil {
		t.Fatalf("shown effect %v", shown)
	}
	// The event records the outcome.
	ev := f.lastRite(t)["effect"].(map[string]any)
	if ev["storm_id"] != id.String() || ev["removed_hex"] == nil || ev["regrow_tick"] == nil {
		t.Fatalf("recorded effect %v", ev)
	}
}

func TestStillTheStorm_RefusesWhatIsNotANamedLivingStormOnYourMap(t *testing.T) {
	f := newAlphaFixture(t)
	f.seaFor(t)
	unnamed := f.namedStorm(t, "", true)
	unseen := f.namedStorm(t, "Not Mine", false)
	dead := f.namedStorm(t, "Gone", true)
	_, _ = f.pool.Exec(context.Background(), `UPDATE sea_storms SET died_tick=90, dies_tick=90 WHERE id=$1`, dead)
	for name, target := range map[string]string{"unnamed": unnamed.String(), "not on my map": "Not Mine", "dead": "Gone", "nothing": "", "no such": "Nope"} {
		fish := f.stock(t, f.cities[0], "fish")
		rec := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessCalm, "target": target})
		if rec.Code < 400 || rec.Code >= 500 {
			t.Fatalf("%s: status %d %s, want 4xx", name, rec.Code, rec.Body.String())
		}
		if f.stock(t, f.cities[0], "fish") != fish {
			t.Fatalf("%s: the gift was taken", name)
		}
	}
	if f.trackLen(t, unnamed) != 3 || f.trackLen(t, unseen) != 3 {
		t.Fatal("a refused prayer shrank a storm")
	}
}

func TestWakeTheStorm_BirthsAnOrdinaryStormOnAKnownSeaHex(t *testing.T) {
	f := newAlphaFixture(t)
	f.seaFor(t)
	f.know(t, 7, 3)
	fish, wine := f.stock(t, f.cities[0], "fish"), f.stock(t, f.cities[0], "wine")
	rec := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessWake, "target": "7,3"})
	if rec.Code != 200 {
		t.Fatalf("cast %d %s", rec.Code, rec.Body.String())
	}
	if got := fish - f.stock(t, f.cities[0], "fish"); got != 10 {
		t.Fatalf("fish paid %v", got)
	}
	if got := wine - f.stock(t, f.cities[0], "wine"); got != 6 {
		t.Fatalf("wine paid %v", got)
	}
	ev := f.lastRite(t)["effect"].(map[string]any)
	sid, _ := uuid.Parse(ev["storm_id"].(string))
	var name, claimed *string
	var dies, hexes int
	if err := f.pool.QueryRow(context.Background(), `SELECT name, claimed_by::text, dies_tick, (SELECT count(*) FROM sea_storm_track WHERE storm_id=s.id AND tick=100) FROM sea_storms s WHERE id=$1`, sid).Scan(&name, &claimed, &dies, &hexes); err != nil {
		t.Fatal(err)
	}
	if name != nil || claimed != nil || hexes != 3 || dies < 100+200 || dies > 100+300 {
		t.Fatalf("name %v claimed %v hexes %d dies %d", name, claimed, hexes, dies)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["effect"].(map[string]any)["storm_id"] != nil {
		t.Fatalf("the response carries the storm id: %v", resp["effect"])
	}
}

func TestWakeTheStorm_RefusesUnknownLandAndCrowdedHexesWithoutTakingTheGift(t *testing.T) {
	f := newAlphaFixture(t)
	f.seaFor(t)
	f.know(t, 7, 3)
	f.know(t, 0, 0) // land
	f.know(t, 5, 4)
	f.namedStorm(t, "Crowd", false) // stands on (4..6, 4)
	cases := map[string]struct {
		target string
		code   int
	}{"unknown sea": {"30,30", 404}, "land": {"0,0", 400}, "occupied": {"5,4", 409}, "garbled": {"here", 400}, "off the map": {"99,99", 404}}
	for name, c := range cases {
		fish := f.stock(t, f.cities[0], "fish")
		rec := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessWake, "target": c.target})
		if rec.Code != c.code {
			t.Fatalf("%s: status %d %s, want %d", name, rec.Code, rec.Body.String(), c.code)
		}
		if f.stock(t, f.cities[0], "fish") != fish {
			t.Fatalf("%s: the gift was taken", name)
		}
	}
	var n int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM sea_storm_wakes w JOIN sea_storms s ON s.id=w.storm_id WHERE s.world_id=$1`, f.world).Scan(&n)
	if n != 0 {
		t.Fatalf("%d storms woken by refused prayers", n)
	}
}

// Nobody can tell a woken storm from a born one: a foreign Wanax who sees it gets the ordinary storm view,
// with no extra field.
func TestWakeTheStorm_AnotherWanaxSeesAnOrdinaryStorm(t *testing.T) {
	f := newAlphaFixture(t)
	f.seaFor(t)
	f.know(t, 7, 3)
	ctx := context.Background()
	if rec := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessWake, "target": "7,3"}); rec.Code != 200 {
		t.Fatalf("cast %d %s", rec.Code, rec.Body.String())
	}
	svc := auth.NewService(f.pool, "alpha-secret")
	otherID, token := registerViewer(t, ctx, svc, "watcher")
	var prov uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,7,2,'plains') RETURNING id`, f.world).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital) VALUES($1,$2,'Watchtown','minoan',$3,'capital',true)`, f.world, prov, otherID); err != nil {
		t.Fatal(err)
	}
	wh := NewWorldHandler(f.pool, svc, clock.NewTestClock(time.Now()))
	r := chi.NewRouter()
	r.Use(auth.Middleware(svc))
	r.Get("/worlds/{worldID}/storms", wh.Storms)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.world.String()+"/storms", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var out struct {
		Storms []map[string]any `json:"storms"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Storms) != 1 {
		t.Fatalf("the watcher sees %d storms: %s", len(out.Storms), rec.Body.String())
	}
	for k := range out.Storms[0] {
		switch k {
		case "id", "tier", "hexes", "heading", "seen_tick":
		default:
			t.Fatalf("the woken storm shows an extra field %q", k)
		}
	}
}

func TestStormPrayers_CatalogueListsNamedStormsAndAsksAHexForTheWake(t *testing.T) {
	f := newAlphaFixture(t)
	f.seaFor(t)
	f.namedStorm(t, "Old Grey", true)
	f.namedStorm(t, "", true)        // unnamed: not a target
	f.namedStorm(t, "Unseen", false) // not on your map
	var prov uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT province_id FROM settlements WHERE id=$1`, f.cities[0]).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.With(auth.Middleware(auth.NewService(f.pool, "alpha-secret"))).Get("/worlds/{worldID}/provinces/{provinceID}", NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), nil, nil).Get)
	req := httptest.NewRequest("GET", "/worlds/"+f.world.String()+"/provinces/"+prov.String(), nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var city struct {
		Settlement struct {
			Prayers []prayerRow `json:"available_prayers"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &city); err != nil {
		t.Fatal(err)
	}
	by := map[string]prayerRow{}
	for _, p := range city.Settlement.Prayers {
		by[p.ID] = p
	}
	calm, wake := by[religion.PrayerStillnessCalm], by[religion.PrayerStillnessWake]
	if calm.Targets == nil || calm.Targets.Kind != "storm" || len(calm.Targets.Storms) != 1 || calm.Targets.Storms[0].Name != "Old Grey" || calm.Targets.Storms[0].Where == "" {
		t.Fatalf("calm targets %+v", calm.Targets)
	}
	if wake.Targets == nil || wake.Targets.Kind != "hex" || calm.ScopeCostFactors != nil {
		t.Fatalf("wake targets %+v", wake.Targets)
	}
}
