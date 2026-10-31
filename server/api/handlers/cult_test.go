package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/religion"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestRealmCultThreeTemplesTwoGodsAndOwnerPrivacy(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	world := settlementsOverviewTestWorld(t, pool, ctx)
	svc := auth.NewService(pool, "cult-test")
	owner, token := settlementsOverviewRegisterPlayer(t, svc, ctx, "cult-owner-"+uuid.NewString())
	other, otherToken := settlementsOverviewRegisterPlayer(t, svc, ctx, "cult-other-"+uuid.NewString())
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// realm projection must also work for the joined, not-yet-founded Wanax.
	exec(`INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$3,40,0,1000),($2,$3,70,0,1000)`, owner, other, world)
	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), nil, nil)
	router := chi.NewRouter()
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/cult", ph.Cult)
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	request := func(path, token string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes()
	}
	path := "/worlds/" + world.String() + "/cult"
	read := func(token string) CultView {
		t.Helper()
		code, b := request(path+"?player_id="+owner.String(), token)
		if code != 200 {
			t.Fatalf("%d %s", code, b)
		}
		var out CultView
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if code, _ := request(path, ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous %d", code)
	}
	empty := read(token)
	if len(empty.Temples) != 0 || len(empty.Prayers) != 0 || len(empty.UnbuiltTemples) != 0 || empty.Kharis["kharis"] != float64(40) {
		t.Fatalf("unfounded realm %+v", empty)
	}
	cities := []uuid.UUID{}
	for i, name := range []string{"Tylissos", "Amnissos", "Kommos"} {
		sid := settlementsOverviewSeedSettlement(t, pool, ctx, world, owner, name, i == 0, i*6, 0)
		cities = append(cities, sid)
		exec(`UPDATE settlements SET culture_id='minoan' WHERE id=$1`, sid)
		exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'timber',500,0,5000,1000),($1,'silver',500,0,5000,1000)`, sid)
		god := "asterion"
		if i == 0 {
			god = "sea_daimons_stillness"
		}
		exec(`INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,$2)`, sid, god)
	}
	exec(`UPDATE player_world_records SET settlement_id=$1 WHERE player_id=$2 AND world_id=$3`, cities[0], owner, world)
	foreign := settlementsOverviewSeedSettlement(t, pool, ctx, world, other, "Foreign private", true, 24, 0)
	exec(`UPDATE settlements SET culture_id='minoan' WHERE id=$1`, foreign)
	exec(`INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',3,'potnia')`, foreign)
	// A physical worker belongs to the exact building, not a realm-wide weight.
	exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key,building_id) SELECT $1,1,'building','temple','cult',id FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, cities[0])
	exec(`UPDATE settlement_goods SET amount=0 WHERE settlement_id=$1 AND good_key='timber'`, cities[1])
	view := read(token)
	unreadySeen := false
	for _, p := range view.Prayers {
		if !p.Affordable {
			unreadySeen = true
		} else if unreadySeen {
			t.Fatal("affordable prayer sorted after one needing a composed offering or unavailable temple")
		}
	}
	if len(view.Temples) != 3 {
		t.Fatalf("realm temples=%d want3", len(view.Temples))
	}
	gods := map[string]int{}
	locations := map[uuid.UUID]bool{}
	for _, temple := range view.Temples {
		gods[temple.GodID]++
		locations[temple.SettlementID] = true
		staff, err := economy.LoadTempleStaffing(ctx, pool, temple.SettlementID)
		if err != nil || staff.Staffed != temple.Staffed {
			t.Fatalf("staffing drift %+v %v", temple, err)
		}
		if temple.Name != religion.TempleName(temple.GodID) || temple.Level != 1 || temple.Capacity != 2 || temple.ProvinceID == uuid.Nil || temple.CityName == "" || temple.Offer.BuildingID != temple.ID || temple.DemolitionKharis == nil {
			t.Fatalf("incomplete temple %+v", temple)
		}
	}
	if gods["asterion"] != 2 || gods["sea_daimons_stillness"] != 1 || len(locations) != 3 {
		t.Fatalf("deduplicated cities/gods: %v %v", gods, locations)
	}
	available, missing := 0, 0
	seenMissing := false
	for _, p := range view.Prayers {
		if p.SettlementID == foreign {
			t.Fatal("foreign prayer leaked")
		}
		if !p.HasTemple {
			seenMissing = true
			missing++
		} else if seenMissing {
			t.Fatal("owned temple prayer placed after missing-temple prayer")
		}
		if p.CanPrepareOffering {
			available++
			if !p.HasTemple {
				t.Fatal("prayer ready without its temple")
			}
		}
		// Exact parity with the normal city API, including context and active/wait fields.
		code, b := request("/worlds/"+world.String()+"/provinces/"+p.ProvinceID.String(), token)
		if code != 200 {
			t.Fatalf("province %d %s", code, b)
		}
		var city struct {
			Settlement struct {
				Prayers []prayerRow `json:"available_prayers"`
			} `json:"settlement"`
		}
		if err := json.Unmarshal(b, &city); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, local := range city.Settlement.Prayers {
			if local.ID == p.ID {
				found = true
				if !reflect.DeepEqual(local, p.prayerRow) {
					t.Fatalf("realm/city prayer drift %+v vs %+v", p.prayerRow, local)
				}
			}
		}
		if !found {
			t.Fatal("realm invented prayer")
		}
	}
	if available == 0 || missing == 0 {
		t.Fatalf("no ready/missing split ready%d missing%d", available, missing)
	}
	if len(view.UnbuiltTemples) != 9 {
		t.Fatalf("unbuilt contexts %d want9", len(view.UnbuiltTemples))
	}
	for _, u := range view.UnbuiltTemples {
		if u.SettlementID == foreign {
			t.Fatal("foreign construction context leaked")
		}
	}
	rival := read(otherToken)
	if len(rival.Temples) != 1 || rival.Temples[0].SettlementID != foreign || rival.Kharis["kharis"] != float64(70) {
		t.Fatalf("caller identity override %+v", rival)
	}
	// Rest is per temple: Amnissos' Asterion temple rests (and, short of servants,
	// stands still); Kommos' Asterion temple is untouched.
	exec(`INSERT INTO temple_rest(building_id,remaining_ticks,last_tick) SELECT id,12,1000 FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, cities[1])
	restSeen, readySeen := false, false
	for _, p := range read(token).Prayers {
		if p.GodID != "asterion" || !p.HasTemple {
			continue
		}
		switch p.SettlementID {
		case cities[1]:
			restSeen = true
			if p.CanPrepareOffering || p.Affordable || p.CooldownRemainingDays != 12 || p.RestRemainingDays != 12 || !p.RestPaused || p.TempleLevel != 1 || p.TempleStaffed != 0 || p.TempleCapacity != 2 {
				t.Fatalf("rest omitted %+v", p)
			}
		case cities[2]:
			readySeen = true
			if p.RestRemainingDays != 0 || p.RestPaused {
				t.Fatalf("other city's temple rests %+v", p)
			}
		}
	}
	if !restSeen || !readySeen {
		t.Fatalf("asterion prayers rest%v ready%v", restSeen, readySeen)
	}
}
