package handlers

import (
	"net/http"
	"strings"
	"testing"
)

// A stone caravan has no destination CITY (dest_id NULL). The map's trade layer
// must still list it for its owner — and not fail on the NULL.
func TestBuildRoad_StoneCaravanListedOnTradesLayer(t *testing.T) {
	a := newRoadAPI(t, 100)
	rec := a.do(t, http.MethodPost, a.buildURL(), a.token, map[string]int{"from_q": 5, "from_r": 0, "to_q": 6, "to_r": 0})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("build-road = %d %s", rec.Code, rec.Body.String())
	}
	rec = a.do(t, http.MethodGet, "/worlds/"+a.world.String()+"/trades", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /trades = %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"kind":"road_stone"`) || !strings.Contains(body, `"dest_q":5`) {
		t.Fatalf("the stone caravan is missing from the trade layer: %s", body)
	}
}
