package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestColonizePreview_FishFoodWarning(t *testing.T) {
	f := buildHexFixture(t)
	// A real coastal production fixture, visible to the same authenticated player.
	_, err := f.pool.Exec(context.Background(), `UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id=$1 AND (q<>0 OR r<>0)`, f.worldID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO map_tiles (world_id,q,r,terrain) VALUES ($1,1,-1,'coastal_sea')`, f.worldID); err != nil {
		t.Fatal(err)
	}
	wh := NewWorldHandler(f.pool, nil, clock.NewTestClock(time.Now()))
	f.router.Get("/worlds/{worldID}/colonize-preview", wh.ColonizePreview)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/colonize-preview?q=0&r=0&pop=1000&seed=0", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var raw struct {
		Grain struct {
			Fish    float64 `json:"fish_per_tick"`
			Net     float64 `json:"food_net_per_tick"`
			Warning string  `json:"loyalty_warning"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Grain.Fish < 5 || raw.Grain.Net < 0 || raw.Grain.Warning != "" {
		t.Fatalf("fish-fed zero-grain-stock forecast falsely warns: %s", rec.Body.String())
	}
}
