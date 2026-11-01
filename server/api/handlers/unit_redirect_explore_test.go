package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
)

// A redirect can carry explore (Timothy 2026-11-01): a march order to a land
// unit already on the road travels by Runner, and on delivery the unit turns
// onto an expedition from where it is — into land none of its men have seen.
func TestMarch_MarchingLandUnitTakesExploreByRunner(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupRecallCourierWorld(t)
	for q := 5; q <= 7; q++ { // unseen ground beyond the capital's sight
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains') ON CONFLICT DO NOTHING`, f.worldID, q); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	router.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	body, _ := json.Marshal(map[string]any{"target_q": 7, "target_r": 0, "intent": "explore"})
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/march", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("march to a marching unit = %d %s, want 202 order_dispatched", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status      string    `json:"status"`
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "order_dispatched" {
		t.Fatalf("not carried by a Runner: %s", rec.Body.String())
	}
	var tq int
	var intent *string
	if err := pool.QueryRow(ctx, `SELECT target_q, march_intent FROM units WHERE id=$1`, f.unitID).Scan(&tq, &intent); err != nil {
		t.Fatal(err)
	}
	if tq != 4 || intent != nil {
		t.Fatalf("command was instant: target %d intent %v", tq, intent)
	}
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	deliverFollowingCourier(t, pool, clk, odh, resp.MessengerID)
	var home *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT march_intent, home_settlement_id FROM units WHERE id=$1`, f.unitID).Scan(&intent, &home); err != nil {
		t.Fatal(err)
	}
	if intent == nil || *intent != "explore" || home == nil {
		t.Fatalf("delivered redirect did not become an expedition: intent %v home %v", intent, home)
	}
}
