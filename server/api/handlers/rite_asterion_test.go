package handlers

// Asterion's offensive and defensive prayers (bönesystemet v3 steg 5b): the Bull's Horns
// (land troops, the first round of a battle) and the Thread (land caravans, a sentry must
// stand one hex off). Same ladder as the other blessings: one named / the city's / the realm's.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/religion"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (f alphaFixture) lastRiteEffect(t *testing.T) map[string]any {
	t.Helper()
	var raw []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM events WHERE world_id=$1 AND event_type='RiteCast' ORDER BY id DESC LIMIT 1`, f.world).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	eff, _ := p["effect"].(map[string]any)
	return eff
}

func TestHornsCast_CostScalesWithTheTargetAndWritesTheRow(t *testing.T) {
	for _, c := range []struct {
		scope  string
		factor float64
	}{{religion.ScopeUnit, 1}, {religion.ScopeCity, 2}, {religion.ScopeRealm, 4}} {
		t.Run(c.scope, func(t *testing.T) {
			f := newAlphaFixture(t)
			troop := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
			body := map[string]any{"prayer": religion.PrayerAsterionHorns, "target_scope": c.scope}
			if c.scope == religion.ScopeUnit {
				body["target"] = troop.String()
			}
			livestock, silver := f.stock(t, f.cities[0], "livestock"), f.stock(t, f.cities[0], "silver")
			r := f.castBody(f.cities[0], body)
			if r.Code != 200 || !strings.Contains(r.Body.String(), `"success":true`) {
				t.Fatalf("cast %d %s", r.Code, r.Body.String())
			}
			if got := livestock - f.stock(t, f.cities[0], "livestock"); got != 5*c.factor {
				t.Fatalf("livestock paid %v, want %v", got, 5*c.factor)
			}
			if got := silver - f.stock(t, f.cities[0], "silver"); got != 2*c.factor {
				t.Fatalf("silver paid %v, want %v", got, 2*c.factor)
			}
			var scope string
			var unitID *uuid.UUID
			var from, until int
			if err := f.pool.QueryRow(context.Background(), `SELECT scope,unit_id,from_tick,until_tick FROM prayer_blessings WHERE world_id=$1 AND effect='blessed_horns'`, f.world).Scan(&scope, &unitID, &from, &until); err != nil {
				t.Fatal(err)
			}
			if scope != c.scope || from != 100 || until != 112 || (c.scope == religion.ScopeUnit) != (unitID != nil && *unitID == troop) {
				t.Fatalf("row scope=%s from=%d until=%d unit=%v", scope, from, until, unitID)
			}
			eff := f.lastRiteEffect(t)
			if eff["target_scope"] != c.scope || eff["until_tick"] != float64(112) || eff["horns_factor"] != 1.5 {
				t.Fatalf("RiteCast effect %v", eff)
			}
		})
	}
}

func TestHornsCast_RefusesWrongTargetsAndHasNoFourthStep(t *testing.T) {
	f := newAlphaFixture(t)
	foreign := f.foreigner(t)
	ship := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	troop := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	theirs := f.unit(t, foreign, "infantry", "land", "garrison", f.cities[0])
	for _, c := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"someone else's troop", map[string]any{"target_scope": "unit", "target": theirs.String()}, 404},
		{"a ship", map[string]any{"target_scope": "unit", "target": ship.String()}, 400},
		{"no target", map[string]any{}, 400},
		{"for ever is not a step of the horns", map[string]any{"target_scope": "permanent", "target": troop.String()}, 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.body["prayer"] = religion.PrayerAsterionHorns
			if r := f.castBody(f.cities[0], c.body); r.Code != c.want {
				t.Fatalf("got %d want %d: %s", r.Code, c.want, r.Body.String())
			}
		})
	}
	if f.rows(t, religion.EffectBlessedHorns) != 0 {
		t.Fatal("a refused cast left a blessing")
	}
}

func TestHornsCast_SameTargetTwiceIs409(t *testing.T) {
	f := newAlphaFixture(t)
	a := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	cast := func() int {
		f.clearRest(t)
		return f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerAsterionHorns, "target_scope": "unit", "target": a.String()}).Code
	}
	if c := cast(); c != 200 {
		t.Fatalf("first: %d", c)
	}
	if c := cast(); c != 409 {
		t.Fatalf("same troop twice: %d, want 409", c)
	}
	if f.rows(t, religion.EffectBlessedHorns) != 1 {
		t.Fatal("the second cast left a row")
	}
}

// naval caravan (a ship's freight): the Thread never reaches it.
func (f alphaFixture) seaFreight(t *testing.T, owner, origin uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO transports(world_id,owner_id,kind,origin_id,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,status,interceptable)
 VALUES($1,$2,'trade',$3,'naval',0,0,3,0,now(),now()+interval '1 hour',108,'in_transit',true) RETURNING id`, f.world, owner, origin).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestThreadCast_CostScalesWithTheTargetAndWritesTheRow(t *testing.T) {
	for _, c := range []struct {
		scope  string
		factor float64
	}{{religion.ScopeUnit, 1}, {religion.ScopeCity, 2}, {religion.ScopeRealm, 4}} {
		t.Run(c.scope, func(t *testing.T) {
			f := newAlphaFixture(t)
			van := f.caravan(t, f.player, f.cities[0])
			body := map[string]any{"prayer": religion.PrayerAsterionThreadV2, "target_scope": c.scope}
			if c.scope == religion.ScopeUnit {
				body["target"] = van.String()
			}
			wine, silver := f.stock(t, f.cities[0], "wine"), f.stock(t, f.cities[0], "silver")
			r := f.castBody(f.cities[0], body)
			if r.Code != 200 || !strings.Contains(r.Body.String(), `"success":true`) {
				t.Fatalf("cast %d %s", r.Code, r.Body.String())
			}
			if got := wine - f.stock(t, f.cities[0], "wine"); got != 3*c.factor {
				t.Fatalf("wine paid %v, want %v", got, 3*c.factor)
			}
			if got := silver - f.stock(t, f.cities[0], "silver"); got != 2*c.factor {
				t.Fatalf("silver paid %v, want %v", got, 2*c.factor)
			}
			var scope string
			var unitID, moverID, city *uuid.UUID
			var moverKind *string
			var until int
			if err := f.pool.QueryRow(context.Background(), `SELECT scope,unit_id,mover_id,mover_kind,settlement_id,until_tick FROM prayer_blessings WHERE world_id=$1 AND effect='blessed_thread'`, f.world).Scan(&scope, &unitID, &moverID, &moverKind, &city, &until); err != nil {
				t.Fatal(err)
			}
			if scope != c.scope || until != 112 || unitID != nil {
				t.Fatalf("row scope=%s until=%d unit=%v", scope, until, unitID)
			}
			if (c.scope == religion.ScopeUnit) != (moverID != nil && *moverID == van && moverKind != nil && *moverKind == "transport") {
				t.Fatalf("mover columns: %v %v", moverID, moverKind)
			}
			if (c.scope == religion.ScopeCity) != (city != nil && *city == f.cities[0]) {
				t.Fatalf("city column: %v", city)
			}
			eff := f.lastRiteEffect(t)
			if eff["target_scope"] != c.scope || eff["until_tick"] != float64(112) {
				t.Fatalf("RiteCast effect %v", eff)
			}
			if (c.scope == religion.ScopeUnit) != (eff["target_mover_id"] == van.String()) {
				t.Fatalf("RiteCast target_mover_id %v", eff["target_mover_id"])
			}
		})
	}
}

func TestThreadCast_OnlyLandCaravansOfTheCasterAreNamable(t *testing.T) {
	f := newAlphaFixture(t)
	foreign := f.foreigner(t)
	runner := f.runner(t, f.player, f.cities[0], f.cities[1])
	sea := f.seaFreight(t, f.player, f.cities[0])
	theirs := f.caravan(t, foreign, f.cities[0])
	van := f.caravan(t, f.player, f.cities[0])
	for _, c := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"a Runner is never taken, so never blessed", map[string]any{"target_scope": "unit", "target": runner.String()}, 404},
		{"sea freight", map[string]any{"target_scope": "unit", "target": sea.String()}, 404},
		{"someone else's caravan", map[string]any{"target_scope": "unit", "target": theirs.String()}, 404},
		{"no such caravan", map[string]any{"target_scope": "unit", "target": uuid.NewString()}, 404},
		{"a unit scope without a caravan", map[string]any{"target_scope": "unit"}, 400},
		{"no scope", map[string]any{}, 400},
		{"for ever is not a step of the thread", map[string]any{"target_scope": "permanent", "target": van.String()}, 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.body["prayer"] = religion.PrayerAsterionThreadV2
			if r := f.castBody(f.cities[0], c.body); r.Code != c.want {
				t.Fatalf("got %d want %d: %s", r.Code, c.want, r.Body.String())
			}
		})
	}
	if f.rows(t, religion.EffectBlessedThread) != 0 {
		t.Fatal("a refused cast left a blessing")
	}
}

func TestThreadCast_SameTargetTwiceIs409(t *testing.T) {
	f := newAlphaFixture(t)
	van := f.caravan(t, f.player, f.cities[0])
	cast := func(scope string) int {
		f.clearRest(t)
		body := map[string]any{"prayer": religion.PrayerAsterionThreadV2, "target_scope": scope}
		if scope == "unit" {
			body["target"] = van.String()
		}
		return f.castBody(f.cities[0], body).Code
	}
	if c := cast("unit"); c != 200 {
		t.Fatalf("first: %d", c)
	}
	if c := cast("unit"); c != 409 {
		t.Fatalf("same caravan twice: %d, want 409", c)
	}
	if c := cast("realm"); c != 200 {
		t.Fatalf("the realm is another target: %d", c)
	}
	if c := cast("city"); c != 409 {
		t.Fatalf("a city under a live realm blessing: %d, want 409", c)
	}
}

// The catalogue tells the Thread which land caravans can be named and the Horns which troops.
func TestAsterionCatalogue_ListsCaravansAndTroopsAndNoFourthStep(t *testing.T) {
	f := newAlphaFixture(t)
	foreign := f.foreigner(t)
	van0 := f.caravan(t, f.player, f.cities[0])
	van1 := f.caravan(t, f.player, f.cities[1])
	f.caravan(t, foreign, f.cities[0])
	f.seaFreight(t, f.player, f.cities[0])
	f.runner(t, f.player, f.cities[0], f.cities[1])
	f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	by := f.catalogue(t, f.cities[0])
	thread, horns := by[religion.PrayerAsterionThreadV2], by[religion.PrayerAsterionHorns]
	if thread.Targets == nil || horns.Targets == nil {
		t.Fatalf("no targets: thread %+v horns %+v", thread.Targets, horns.Targets)
	}
	ids := map[string]bool{}
	for _, u := range thread.Targets.Unit {
		ids[u.ID] = true
		if u.Name == "" || u.Where == "" {
			t.Fatalf("a caravan target needs a name and a where: %+v", u)
		}
	}
	if len(ids) != 2 || !ids[van0.String()] || !ids[van1.String()] {
		t.Fatalf("thread targets are the caster's own land caravans: %+v", thread.Targets.Unit)
	}
	if thread.Targets.City.Count != 1 || thread.Targets.Realm.Count != 2 {
		t.Fatalf("thread counts city %d realm %d, want 1 and 2", thread.Targets.City.Count, thread.Targets.Realm.Count)
	}
	if len(horns.Targets.Unit) != 1 || horns.Targets.Realm.Count != 1 {
		t.Fatalf("horns targets (land troops only): %+v", horns.Targets)
	}
	for name, p := range map[string]prayerRow{"thread": thread, "horns": horns} {
		sf := p.ScopeCostFactors
		if sf["unit"] != 1 || sf["city"] != 2 || sf["realm"] != 4 {
			t.Fatalf("%s step costs %v", name, sf)
		}
		if _, has := sf["permanent"]; has {
			t.Fatalf("%s offers a fourth step: %v", name, sf)
		}
	}
	if thread.Targets.City.Name == "" || !thread.Targets.Always {
		t.Fatalf("a city or realm Thread needs no caravan now: %+v", thread.Targets)
	}
}

// catalogue reads the prayers a city offers, by id (the province view the web and keryx read).
func (f alphaFixture) catalogue(t *testing.T, city uuid.UUID) map[string]prayerRow {
	t.Helper()
	var prov uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT province_id FROM settlements WHERE id=$1`, city).Scan(&prov); err != nil {
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
	var out struct {
		Settlement struct {
			Prayers []prayerRow `json:"available_prayers"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	by := map[string]prayerRow{}
	for _, p := range out.Settlement.Prayers {
		by[p.ID] = p
	}
	return by
}
