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

func TestBuildQueueCarriesCityPlayerNameAndHex(t *testing.T) {
	f := setupRecruitShipFixture(t)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO build_queue(settlement_id,world_id,building_type,complete_at,hex_q,hex_r) VALUES($1,$2,'lumbermill',now(),0,7)`, f.settlementID, f.worldID); err != nil {
		t.Fatal(err)
	}
	h := NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), nil, nil)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body struct {
		Settlement struct {
			Name  string `json:"name"`
			Queue []struct {
				Name, SettlementName string
				HexQ, HexR           *int
			} `json:"-"`
			Raw []map[string]any `json:"build_queue"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Settlement.Raw) != 1 {
		t.Fatal(body.Settlement.Raw)
	}
	q := body.Settlement.Raw[0]
	if q["name"] != "Lumbermill" || q["settlement_name"] != body.Settlement.Name || q["hex_q"] != float64(0) || q["hex_r"] != float64(7) {
		t.Fatalf("queue lost place:%+v", q)
	}
}
