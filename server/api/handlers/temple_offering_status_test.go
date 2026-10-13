package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/kharis"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestEarlyTempleStatusPlanAndOwnerPrivacy(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	wid := settlementsOverviewTestWorld(t, pool, ctx)
	svc := auth.NewService(pool, "early-temple-test")
	pid, token := settlementsOverviewRegisterPlayer(t, svc, ctx, "temple-owner-"+uuid.New().String())
	_, otherToken := settlementsOverviewRegisterPlayer(t, svc, ctx, "temple-viewer-"+uuid.New().String())
	sid := settlementsOverviewSeedSettlement(t, pool, ctx, wid, pid, "Phaistos", true, 0, 0)
	var prov uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, sid).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'temple',1);
	`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',.25,0,100,current_world_tick())`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick)
	 SELECT $1,key,0,0,1,current_world_tick() FROM goods WHERE key IN ('grain','fish','timber','silver')`, wid); err != nil {
		t.Fatal(err)
	}
	h := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.With(auth.Middleware(svc)).Get("/worlds/{worldID}/provinces/{provinceID}", h.Get)
	for _, tc := range []struct {
		name, token string
		own         bool
	}{{"owner", token, true}, {"other Wanax", otherToken, false}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/worlds/"+wid.String()+"/provinces/"+prov.String(), nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("GET %d %s", rec.Code, rec.Body.String())
			}
			var response struct {
				Settlement struct {
					Offers []kharis.TempleOffer `json:"temple_offers"`
				} `json:"settlement"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			offers := response.Settlement.Offers
			if !tc.own {
				if len(offers) != 0 {
					t.Fatalf("foreign realm's silver leaked %+v", offers)
				}
				return
			}
			if len(offers) != 1 || !offers[0].Fed || offers[0].Offering["silver"] != .25 || offers[0].Stock["silver"] != .25 || offers[0].Required["grain"] != .25 {
				t.Fatalf("actual API plan %+v", offers)
			}
		})
	}
}
