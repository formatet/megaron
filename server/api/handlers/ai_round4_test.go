package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestAIRound4_AutomaticFoodAPIAndFixedQueue(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	wid := settlementsOverviewTestWorld(t, pool, ctx)
	svc := auth.NewService(pool, "test-secret")
	owner, token := settlementsOverviewRegisterPlayer(t, svc, ctx, "round4-"+uuid.New().String())
	sid := settlementsOverviewSeedSettlement(t, pool, ctx, wid, owner, "Scheria", true, 0, 0)
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population=1042 WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	for i, h := range hexgrid.Ring(hexgrid.Coord{}, hexgrid.CatchmentRadius) {
		terrain := "forest_olive_grove"
		if i < 9 {
			terrain = "plains"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,$4)`, wid, h.Q, h.R, terrain); err != nil {
			t.Fatal(err)
		}
	}
	if err := economy.RecomputeProduction(ctx, pool, sid); err != nil {
		t.Fatal(err)
	}
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Unix(0, 0))
	cfg := economy.LoadSitosConfig()
	ph := NewProvinceHandler(pool, nil, clk, cfg, nil, nil)
	sh := NewSettlementHandler(pool, nil, nil, clk, cfg)
	router := chi.NewRouter()
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/settlements/overview", sh.SettlementsOverview)
	router.With(auth.Middleware(svc)).Post("/worlds/{worldID}/provinces/{provinceID}/build", ph.Build)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"/worlds/" + wid.String() + "/provinces/" + pid.String(), "/worlds/" + wid.String() + "/settlements/overview"} {
		rec := request("GET", path, "")
		if rec.Code != 200 {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		var raw any
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		var s map[string]any
		if list, ok := raw.([]any); ok {
			s = list[0].(map[string]any)
		} else {
			s = raw.(map[string]any)["settlement"].(map[string]any)
		}
		a, ok := s["automatic_food"].(map[string]any)
		if !ok || a["remainder_people"] != float64(42) || a["nearjord_grain_per_tick"] != float64(.5) || math.Abs(a["remainder_grain_per_tick"].(float64)-3.78) > 1e-9 {
			t.Fatalf("missing actual automatic contribution: %s", rec.Body.String())
		}
		if math.Abs(s["grain_prod_rate"].(float64)-4.28) > 1e-9 {
			t.Fatalf("unexpected stored production: %v", s)
		}
	}
	for _, kind := range []string{"market", "temple"} {
		if _, err := pool.Exec(ctx, `INSERT INTO build_queue(world_id,settlement_id,building_type,complete_at) VALUES($1,$2,$3,$4)`, wid, sid, kind, time.Unix(4102444800, 0)); err != nil {
			t.Fatal(err)
		}
	}
	// A different valid city building hits the fixed queue guard before costs.
	rec := request("POST", "/worlds/"+wid.String()+"/provinces/"+pid.String()+"/build", `{"building_type":"wall"}`)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "fixed at 2 concurrent") || !strings.Contains(rec.Body.String(), "population and buildings do not add slots") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
