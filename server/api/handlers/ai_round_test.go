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

func TestAIRound_ShortfallCarriesCurrentRate(t *testing.T) {
	f := buildHexFixture(t)
	if _, err := f.pool.Exec(context.Background(), `UPDATE settlement_goods SET amount=22,rate=2,calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key='stone'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	code, body := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "temple"})
	if code != 422 {
		t.Fatal(code, body)
	}
	rows, ok := body["missing"].([]any)
	if !ok {
		t.Fatal(body)
	}
	for _, row := range rows {
		m := row.(map[string]any)
		if m["good"] == "stone" {
			if m["rate_per_tick"] != float64(2) {
				t.Fatal(body)
			}
			return
		}
	}
	t.Fatal(body)
}
func TestAIRound_ForecastReportsFarmFacts(t *testing.T) {
	f := buildHexFixture(t)
	h := NewWorldHandler(f.pool, nil, clock.NewTestClock(time.Now()))
	f.router.Get("/worlds/{worldID}/colonize-preview", h.ColonizePreview)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/colonize-preview?q=0&r=0&starter_farm=1&pop=1000", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var b struct {
		Grain struct {
			Possible bool `json:"farm_possible"`
			Included bool `json:"starter_farm_included"`
		}
		Gifts []struct{ Key string } `json:"founding_gifts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if !b.Grain.Possible || !b.Grain.Included {
		t.Fatal(rec.Body.String())
	}
	found := false
	for _, g := range b.Gifts {
		if g.Key == "demeter_farm" {
			found = true
		}
	}
	if !found {
		t.Fatal(rec.Body.String())
	}
}
