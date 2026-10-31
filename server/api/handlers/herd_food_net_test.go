package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Timothy 2026-10-31: a herd standing at its pen cap counts its overflow in the
// food net every surface reads (province GET), the same term the growth gate adds.
func TestProvinceGet_HerdAtCapRaisesFoodNet(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	authSvc := auth.NewService(pool, "test-secret")
	cases := []struct {
		name string
		herd float64
		want float64
	}{
		{"no herd", 0, 10.5},
		{"herd under the cap", 49, 10.5},
		{"herd at the cap", 50, 10.75}, // 5 calves per 20 game days, one food each
	}
	for i, c := range cases {
		worldID := settlementsOverviewTestWorld(t, pool, ctx)
		ownerID, _ := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "wanax-herdnet-"+uuid.New().String())
		sid := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerID, "Herdton", true, i, 0)
		if c.herd > 0 {
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1,'livestock',$2,0,1000000,current_world_tick())`, sid, c.herd); err != nil {
				t.Fatal(err)
			}
		}
		var provinceID uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, sid).Scan(&provinceID); err != nil {
			t.Fatal(err)
		}
		ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), nil, nil)
		r := chi.NewRouter()
		r.Get("/worlds/{worldID}/provinces/{provinceID}", asPlayer(ownerID, ph.Get))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces/"+provinceID.String(), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: GET = %d: %s", c.name, rec.Code, rec.Body.String())
		}
		var resp struct {
			Settlement struct {
				Food struct {
					FoodNetPerTick float64            `json:"food_net_per_tick"`
					Herd           map[string]float64 `json:"herd"`
				} `json:"food"`
			} `json:"settlement"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if got := resp.Settlement.Food.FoodNetPerTick; math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: food_net_per_tick=%g, want %g", c.name, got, c.want)
		}
	}
}
