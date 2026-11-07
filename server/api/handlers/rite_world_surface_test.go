package handlers

// What the player is shown of the three world blessings (steg 5a): the catalogue's targets and
// step prices, and the city's view of what stands over it.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/religion"
	"github.com/go-chi/chi/v5"
)

type provinceView struct {
	Settlement struct {
		Prayers       []prayerRow             `json:"available_prayers"`
		CityBlessings []religion.CityBlessing `json:"city_blessings"`
	} `json:"settlement"`
}

func (f alphaFixture) province(t *testing.T, city int, token string) provinceView {
	t.Helper()
	var prov string
	if err := f.pool.QueryRow(context.Background(), `SELECT province_id::text FROM settlements WHERE id=$1`, f.cities[city]).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.With(auth.Middleware(auth.NewService(f.pool, "alpha-secret"))).Get("/worlds/{worldID}/provinces/{provinceID}", NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), nil, nil).Get)
	req := httptest.NewRequest("GET", "/worlds/"+f.world.String()+"/provinces/"+prov, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var v provinceView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestWorldBlessingCatalogue_StepsTargetsAndWhatStandsOverTheCity(t *testing.T) {
	f := newAlphaFixture(t)
	by := map[string]prayerRow{}
	for _, p := range f.province(t, 0, f.token).Settlement.Prayers {
		by[p.ID] = p
	}
	for id, want := range map[string]map[string]int{
		religion.PrayerPotniaFertility:   {"city": 2, "realm": 4},
		religion.PrayerVelchanosBuilding: {"city": 2, "realm": 4},
		religion.PrayerPotniaScapegoat:   {"realm": 4},
	} {
		p, ok := by[id]
		if !ok || p.Targets == nil {
			t.Fatalf("%s missing from the catalogue or without targets", id)
		}
		if len(p.ScopeCostFactors) != len(want) {
			t.Fatalf("%s steps %v want %v", id, p.ScopeCostFactors, want)
		}
		for k, v := range want {
			if p.ScopeCostFactors[k] != v {
				t.Fatalf("%s step %s = %d want %d", id, k, p.ScopeCostFactors[k], v)
			}
		}
		if len(p.Targets.Unit) != 0 || !p.Targets.Always || p.Targets.Kind != "world" || p.Targets.City.Name != "Alpha City" || p.Targets.Realm.Count != 2 {
			t.Fatalf("%s targets %+v", id, p.Targets)
		}
	}
	if by[religion.PrayerPotniaFertility].BaseDurationDays != 20 || by[religion.PrayerVelchanosBuilding].BaseDurationDays != 12 {
		t.Fatal("lengths are not offered")
	}

	// what stands over the city: only the owner sees it, and only what reaches that city
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerPotniaFertility, "target_scope": "city"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := f.castBody(f.cities[1], map[string]any{"prayer": religion.PrayerVelchanosBuilding, "target_scope": "realm"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	got := f.province(t, 0, f.token).Settlement.CityBlessings
	if len(got) != 2 {
		t.Fatalf("city 0 blessings %+v", got)
	}
	other := f.province(t, 1, f.token).Settlement.CityBlessings
	if len(other) != 1 || other[0].Effect != religion.EffectBlessedBuilding || other[0].Scope != "realm" || other[0].RemainingGameDays != 12 {
		t.Fatalf("city 1 blessings %+v (the city blessing of city 0 must not reach it)", other)
	}
}

func TestBuilderBlessing_TheQueueShowsTheNewFinishAndWhoWasCut(t *testing.T) {
	f := newAlphaFixture(t)
	q := f.build(t, f.cities[0], 12)
	f.setTick(t, 104)
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerVelchanosBuilding, "target_scope": "city"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	var raw struct {
		Settlement struct {
			Queue []map[string]any `json:"build_queue"`
		} `json:"settlement"`
	}
	var prov string
	_ = f.pool.QueryRow(context.Background(), `SELECT province_id::text FROM settlements WHERE id=$1`, f.cities[0]).Scan(&prov)
	router := chi.NewRouter()
	router.With(auth.Middleware(auth.NewService(f.pool, "alpha-secret"))).Get("/worlds/{worldID}/provinces/{provinceID}", NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), nil, nil).Get)
	req := httptest.NewRequest("GET", "/worlds/"+f.world.String()+"/provinces/"+prov, nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if len(raw.Settlement.Queue) != 1 || raw.Settlement.Queue[0]["id"] != q.String() || raw.Settlement.Queue[0]["blessed"] != true || raw.Settlement.Queue[0]["complete_tick"] != float64(110) {
		t.Fatalf("queue row %v, want blessed with complete_tick 110", raw.Settlement.Queue)
	}
}
