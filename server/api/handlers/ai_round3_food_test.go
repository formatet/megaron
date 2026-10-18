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
	"formatet/megaron/server/internal/combat"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Real DB + actual HTTP handlers: grain-only legacy and population food balance
// differ on a fish-fed coast; the new army-food figure is identical on both APIs.
func TestAIRound3_FoodAfterArmyAPIParity(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	wid := settlementsOverviewTestWorld(t, pool, ctx)
	authSvc := auth.NewService(pool, "test-secret")
	owner, token := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "round3-"+uuid.New().String())
	sid := settlementsOverviewSeedSettlement(t, pool, ctx, wid, owner, "Scheria", true, 0, 0)
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population=1000 WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=CASE good_key WHEN 'grain' THEN 0.5 ELSE 5 END WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,support_settlement_id) VALUES($1,$2,'galley','naval',1,'garrison',$3,$3)`, wid, owner, sid); err != nil {
		t.Fatal(err)
	}
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Unix(0, 0))
	ph := NewProvinceHandler(pool, nil, clk, nil, nil)
	sh := NewSettlementHandler(pool, nil, nil, clk)
	r := chi.NewRouter()
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/settlements/overview", sh.SettlementsOverview)
	up := combat.UnitUpkeep("galley", "naval", 1, "garrison")
	if up.Grain <= 0 {
		t.Fatal("no upkeep in fixture")
	}
	for _, path := range []string{"/worlds/" + wid.String() + "/provinces/" + pid.String(), "/worlds/" + wid.String() + "/settlements/overview"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		var data any
		if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		var s map[string]any
		if list, ok := data.([]any); ok {
			s = list[0].(map[string]any)
		} else {
			s = data.(map[string]any)["settlement"].(map[string]any)
		}
		net, ok := s["food_net_per_tick_after_upkeep"].(float64)
		if !ok || math.Abs(net-(.5-up.Grain)) > 1e-9 {
			t.Fatalf("food after army: want %f, got %v", .5-up.Grain, s)
		}
		if s["food"].(map[string]any)["food_net_per_tick"].(float64) != .5 {
			t.Fatal("population/growth gate changed")
		}
	}
}
