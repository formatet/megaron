package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestPlaytestKharis_SameRealmPoolInTwoCities(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	wid := settlementsOverviewTestWorld(t, pool, ctx)
	svc := auth.NewService(pool, "test-secret")
	owner, _ := settlementsOverviewRegisterPlayer(t, svc, ctx, "realm-"+uuid.NewString())
	ids := []uuid.UUID{
		settlementsOverviewSeedSettlement(t, pool, ctx, wid, owner, "Capital", true, 0, 0),
		settlementsOverviewSeedSettlement(t, pool, ctx, wid, owner, "Colony", false, 5, 0),
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET culture_id='minoan' WHERE world_id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records (player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES ($1,$2,14.7,0,1000)`, owner, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick) VALUES($1,'bronze',1,1,37.5,1000),($1,'grain',1,1,2.5,1000)`, wid); err != nil {
		t.Fatal(err)
	}
	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	for _, id := range ids {
		var pid uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, id).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/worlds/"+wid.String()+"/provinces/"+pid.String(), nil))
		if rec.Code != 200 {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Settlement struct {
				DivineValues map[string]float64 `json:"divine_values"`
				Kharis       float64            `json:"kharis"`
				Prayers      []struct {
					Reason string `json:"unavailable_reason"`
				} `json:"available_prayers"`
			} `json:"settlement"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Settlement.DivineValues["bronze"] != 37.5 || body.Settlement.DivineValues["grain"] != 2.5 {
			t.Fatalf("not the live divine values: %s", rec.Body.String())
		}
		if len(body.Settlement.Prayers) == 0 {
			t.Fatal("expected culture prayers")
		}
		for _, pr := range body.Settlement.Prayers {
			if pr.Reason != "no temple" {
				t.Fatalf("unready reason %q, want no temple", pr.Reason)
			}
		}
		if body.Settlement.Kharis != 14.7 {
			t.Fatalf("city %s kharis=%v, want realm14.7", id, body.Settlement.Kharis)
		}
	}
}

func TestPlaytestColonizePreview_FullConsumption(t *testing.T) {
	f := buildHexFixture(t)
	wh := NewWorldHandler(f.pool, nil, clock.NewTestClock(time.Now()))
	f.router.Get("/worlds/{worldID}/colonize-preview", wh.ColonizePreview)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/colonize-preview?q=0&r=0&pop=1000&seed=0", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Grain struct {
			Consumption   float64 `json:"consumption_per_tick"`
			Warning       string  `json:"loyalty_warning"`
			FoodNet       float64 `json:"food_net_per_tick"`
			TimberWarning string  `json:"timber_warning"`
		} `json:"grain"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var wood struct {
		Warning string `json:"timber_warning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wood); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(wood.Warning, "No timber on known ground") {
		t.Fatalf("wood warning not wired: %s", rec.Body.String())
	}
	if body.Grain.Consumption != 5 || ((body.Grain.Warning != "") != (body.Grain.FoodNet < 0)) {
		t.Fatalf("forecast lacks full consumption or food-balance warning: %s", rec.Body.String())
	}
}

func TestPlaytestColonizePreview_CentreFirstAndCoastalGift(t *testing.T) {
	f := buildHexFixture(t)
	if _, err := f.pool.Exec(context.Background(), `UPDATE map_tiles SET coastal=true WHERE world_id=$1 AND q=0 AND r=0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	wh := NewWorldHandler(f.pool, nil, clock.NewTestClock(time.Now()))
	f.router.Get("/worlds/{worldID}/colonize-preview", wh.ColonizePreview)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/colonize-preview?q=0&r=0&pop=1000&starter_farm=1", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Catchment []struct {
			Q int `json:"q"`
			R int `json:"r"`
		} `json:"catchment"`
		Harbour string `json:"harbour_note"`
		Gifts   []struct {
			Key string `json:"key"`
		} `json:"founding_gifts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Catchment) != 19 || body.Catchment[0].Q != 0 || body.Catchment[0].R != 0 {
		t.Fatalf("centre not first: %s", rec.Body.String())
	}
	if !strings.HasPrefix(body.Harbour, "Coastal:") {
		t.Fatalf("wrong centre's coast: %s", rec.Body.String())
	}
	if len(body.Gifts) != 1 || body.Gifts[0].Key != "poseidon_galley" {
		t.Fatalf("wrong founding gift: %s", rec.Body.String())
	}
}
