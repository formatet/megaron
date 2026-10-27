package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListForcesIncludesBoundRouteOnIdleShip(t *testing.T) {
	f, router := setupRecallCourierWorld(t)
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	var home uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id=$1 AND owner_id=$2 ORDER BY is_capital DESC LIMIT 1`, f.worldID, f.playerID).Scan(&home); err != nil {
		t.Fatal(err)
	}
	dest := settlementsOverviewSeedSettlement(t, pool, ctx, f.worldID, f.playerID, "Route destination", false, 8, 0)
	var ship, route uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id) VALUES($1,$2,'merchantman','naval',1,10,'garrison',$3) RETURNING id`, f.worldID, f.playerID, home).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO standing_orders(world_id,owner_id,from_settlement_id,to_settlement_id,crewed_by_settlement_id,ship_unit_id,status) VALUES($1,$2,$3,$4,$3,$5,'paused') RETURNING id`, f.worldID, f.playerID, home, dest, ship).Scan(&route); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Now())
	handler := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	router.Get("/worlds/{worldID}/units", handler.ListUnits)
	req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("roster %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Units []unitSummary `json:"units"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, u := range got.Units {
		if u.ID == ship {
			if u.Status != "garrison" || u.BoundRouteID == nil || *u.BoundRouteID != route || u.BoundRouteName == nil || u.FreightingNote != nil {
				t.Fatalf("idle route missing or false cargo leg %+v", u)
			}
			return
		}
	}
	t.Fatal("own idle ship omitted")
}
