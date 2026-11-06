package handlers

// Targeted prayers (bönesystemet v3 steg 2a): a blessing is laid on one named
// unit, on the units of the city where it is said, or on the whole realm; the
// wider the target, the dearer the gift (x1 / x2 / x4).

import (
	"bytes"
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

func (f alphaFixture) castBody(city uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/worlds/"+f.world.String()+"/settlements/"+city.String()+"/rite", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f alphaFixture) unit(t *testing.T, owner uuid.UUID, typ, cat, status string, support uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,support_settlement_id) VALUES($1,$2,$3,$4,10,0,$5,0,0,$6) RETURNING id`,
		f.world, owner, typ, cat, status, support).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f alphaFixture) rows(t *testing.T, effect string) (n int) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM prayer_blessings WHERE world_id=$1 AND effect=$2`, f.world, effect).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return
}

func TestBlessingCast_CostScalesWithTheTargetAndWritesTheRow(t *testing.T) {
	for _, c := range []struct {
		scope  string
		factor float64
	}{{religion.ScopeUnit, 1}, {religion.ScopeCity, 2}, {religion.ScopeRealm, 4}} {
		t.Run(c.scope, func(t *testing.T) {
			f := newAlphaFixture(t)
			ctx := context.Background()
			troop := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
			body := map[string]any{"prayer": religion.PrayerVelchanosStrength, "target_scope": c.scope}
			if c.scope == religion.ScopeUnit {
				body["target"] = troop.String()
			}
			livestock, wine := f.stock(t, f.cities[0], "livestock"), f.stock(t, f.cities[0], "wine")
			r := f.castBody(f.cities[0], body)
			if r.Code != 200 || !strings.Contains(r.Body.String(), `"success":true`) {
				t.Fatalf("cast %d %s", r.Code, r.Body.String())
			}
			if got := livestock - f.stock(t, f.cities[0], "livestock"); got != 6*c.factor {
				t.Fatalf("livestock paid %v, want %v", got, 6*c.factor)
			}
			if got := wine - f.stock(t, f.cities[0], "wine"); got != 4*c.factor {
				t.Fatalf("wine paid %v, want %v", got, 4*c.factor)
			}
			var scope string
			var unitID, settlementID *uuid.UUID
			var from, until int
			var castEvent *int64
			var owner uuid.UUID
			if err := f.pool.QueryRow(ctx, `SELECT scope,unit_id,settlement_id,from_tick,until_tick,cast_event_id,owner_id FROM prayer_blessings WHERE world_id=$1`, f.world).Scan(&scope, &unitID, &settlementID, &from, &until, &castEvent, &owner); err != nil {
				t.Fatal(err)
			}
			if scope != c.scope || from != 100 || until != 106 || owner != f.player || castEvent == nil {
				t.Fatalf("row scope=%s from=%d until=%d owner=%v event=%v", scope, from, until, owner, castEvent)
			}
			if (c.scope == religion.ScopeUnit) != (unitID != nil && *unitID == troop) || (c.scope == religion.ScopeCity) != (settlementID != nil && *settlementID == f.cities[0]) {
				t.Fatalf("target columns wrong: unit=%v city=%v", unitID, settlementID)
			}
			var payload map[string]any
			var raw []byte
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM events WHERE world_id=$1 AND event_type='RiteCast'`, f.world).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			_ = json.Unmarshal(raw, &payload)
			eff, _ := payload["effect"].(map[string]any)
			if eff["target_scope"] != c.scope || eff["until_tick"] != float64(106) || eff["blessing_id"] == nil {
				t.Fatalf("RiteCast effect lacks the target: %v", eff)
			}
			if (c.scope == religion.ScopeUnit) != (eff["target_unit_id"] == troop.String()) {
				t.Fatalf("RiteCast target_unit_id %v", eff["target_unit_id"])
			}
		})
	}
}

func TestBlessingCast_RefusesWrongTargets(t *testing.T) {
	f := newAlphaFixture(t)
	var foreign uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	ship := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	troop := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	dead := f.unit(t, f.player, "infantry", "land", "disbanded", f.cities[0])
	theirs := f.unit(t, foreign, "infantry", "land", "garrison", f.cities[0])
	stock := f.stock(t, f.cities[0], "livestock")
	for _, c := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"someone else's unit", map[string]any{"target_scope": "unit", "target": theirs.String()}, 404},
		{"a ship for a strength prayer", map[string]any{"target_scope": "unit", "target": ship.String()}, 400},
		{"a disbanded unit", map[string]any{"target_scope": "unit", "target": dead.String()}, 400},
		{"no such unit", map[string]any{"target_scope": "unit", "target": uuid.NewString()}, 404},
		{"unit scope without a unit", map[string]any{"target_scope": "unit"}, 400},
		{"no target at all", map[string]any{}, 400},
		{"unknown scope", map[string]any{"target_scope": "world", "target": troop.String()}, 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.body["prayer"] = religion.PrayerVelchanosStrength
			if r := f.castBody(f.cities[0], c.body); r.Code != c.want {
				t.Fatalf("got %d want %d: %s", r.Code, c.want, r.Body.String())
			}
		})
	}
	if f.stock(t, f.cities[0], "livestock") != stock || f.rows(t, religion.EffectBlessedStrength) != 0 {
		t.Fatal("a refused cast cost goods or left a blessing")
	}
	// Hull prayer: a troop is the wrong kind; no ships in a city = nothing to bless.
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessHull, "target_scope": "unit", "target": troop.String()}); r.Code != 400 {
		t.Fatalf("a troop for a hull prayer: %d", r.Code)
	}
	if r := f.castBody(f.cities[1], map[string]any{"prayer": religion.PrayerStillnessHull, "target_scope": "city"}); r.Code != 400 {
		t.Fatalf("city scope with no ships of that city: %d %s", r.Code, r.Body.String())
	}
}

func TestBlessingCast_SameTargetTwiceIs409TwoTargetsAreFine(t *testing.T) {
	f := newAlphaFixture(t)
	a := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	b := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	cast := func(target uuid.UUID) int {
		f.clearRest(t) // the temple's own rest is not what is under test
		return f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessHull, "target_scope": "unit", "target": target.String()}).Code
	}
	if code := cast(a); code != 200 {
		t.Fatalf("first: %d", code)
	}
	before := f.stock(t, f.cities[0], "fish")
	if code := cast(a); code != 409 {
		t.Fatalf("same ship twice: %d, want 409", code)
	}
	if f.stock(t, f.cities[0], "fish") != before || f.rows(t, religion.EffectBlessedHull) != 1 {
		t.Fatal("the refused cast cost goods or left a second row")
	}
	if code := cast(b); code != 200 {
		t.Fatalf("a different ship: %d, want 200", code)
	}
	// A realm blessing now covers both, so a city blessing over them is redundant too.
	f.clearRest(t)
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessHull, "target_scope": "realm"}); r.Code != 200 {
		t.Fatalf("realm: %d %s", r.Code, r.Body.String())
	}
	f.clearRest(t)
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessHull, "target_scope": "realm"}); r.Code != 409 {
		t.Fatalf("realm twice: %d", r.Code)
	}
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerStillnessHull, "target_scope": "city"}); r.Code != 409 {
		t.Fatalf("city under a live realm blessing: %d", r.Code)
	}
}

func TestBlessingCast_WiderTargetNeedsTheWiderGift(t *testing.T) {
	f := newAlphaFixture(t)
	f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	// livestock 6 + wine 4 is the unit-scope recipe; city scope asks twice that in worth.
	r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerVelchanosStrength, "target_scope": "city", "offering": map[string]float64{"livestock": 6, "wine": 4}})
	if r.Code != 400 {
		t.Fatalf("a unit-sized gift for a city blessing: %d %s", r.Code, r.Body.String())
	}
	r = f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerVelchanosStrength, "target_scope": "city", "offering": map[string]float64{"livestock": 12, "wine": 8}})
	if r.Code != 200 {
		t.Fatalf("the city-sized gift: %d %s", r.Code, r.Body.String())
	}
	// Twice the traditional city gift (x4 of the unit recipe) lasts longer: the length is the gift against ITS OWN baseline.
	f.clearRest(t)
	f.unit(t, f.player, "infantry", "land", "garrison", f.cities[1])
	r = f.castBody(f.cities[1], map[string]any{"prayer": religion.PrayerVelchanosStrength, "target_scope": "city", "offering": map[string]float64{"livestock": 24, "wine": 16}})
	if r.Code != 200 {
		t.Fatalf("a rich gift: %d %s", r.Code, r.Body.String())
	}
	var until int
	if err := f.pool.QueryRow(context.Background(), `SELECT max(until_tick) FROM prayer_blessings WHERE world_id=$1`, f.world).Scan(&until); err != nil {
		t.Fatal(err)
	}
	if until <= 106 || until > 100+6*religion.MaxGiftFactor {
		t.Fatalf("a double gift should last longer than 6 days: until %d", until)
	}
}

// The catalogue tells each blessing prayer what can be named and what each step costs.
func TestBlessingCatalogue_ListsTargetsAndStepCosts(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	f.unit(t, f.player, "galley", "naval", "garrison", f.cities[1])
	f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	f.unit(t, f.player, "infantry", "land", "disbanded", f.cities[0])
	var foreign uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	f.unit(t, foreign, "galley", "naval", "garrison", f.cities[0])
	var prov uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, f.cities[0]).Scan(&prov); err != nil {
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
	hull, strength := by[religion.PrayerStillnessHull], by[religion.PrayerVelchanosStrength]
	if hull.Targets == nil || strength.Targets == nil {
		t.Fatalf("blessing prayers lack targets: %+v / %+v", hull.Targets, strength.Targets)
	}
	if len(hull.Targets.Unit) != 2 || hull.Targets.City.Count != 1 || hull.Targets.Realm.Count != 2 || hull.Targets.City.Name != "Alpha City" {
		t.Fatalf("hull targets (own ships only, city = support city): %+v", hull.Targets)
	}
	if len(strength.Targets.Unit) != 1 || strength.Targets.Realm.Count != 1 {
		t.Fatalf("strength targets (the disbanded unit is not one): %+v", strength.Targets)
	}
	if hull.Targets.Unit[0].Name == "" || hull.Targets.Unit[0].Where == "" || hull.Targets.Unit[0].ID == "" {
		t.Fatalf("a target needs id, name and where: %+v", hull.Targets.Unit[0])
	}
	if hull.ScopeCostFactors["unit"] != 1 || hull.ScopeCostFactors["city"] != 2 || hull.ScopeCostFactors["realm"] != 4 {
		t.Fatalf("step costs %v", hull.ScopeCostFactors)
	}
	if by[religion.PrayerPotniaWalls].Targets != nil {
		t.Fatal("a prayer without a target got a target list")
	}
}
