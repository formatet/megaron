package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecruitQuoteMatchesPopulationAndExplainsShortfall(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET population=20 WHERE id=$1`, f.settlementID); err != nil {
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
			Rows []struct {
				Unit    string   `json:"unit"`
				Can     bool     `json:"can_recruit"`
				Reasons []string `json:"reasons"`
			} `json:"can_recruit"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, row := range body.Settlement.Rows {
		if row.Unit == "galley" {
			if row.Can || len(row.Reasons) == 0 {
				t.Fatalf("20 citizens cannot crew a20-person ship while leaving a resident; quote=%+v", row)
			}
			return
		}
	}
	t.Fatal("galley quote missing")
}

func TestRecruitCedarReceiptAndCancellationPreservePaidGoods(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1;`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'cedar',749,0,10000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=749,rate=0`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	rec, body := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	paid, ok := body["paid_costs"].(map[string]any)
	if !ok || paid["cedar"] != float64(30) {
		t.Fatalf("receipt must name actual cedar payment: %+v", body)
	}
	id := body["unit_id"].(string)
	rec, _ = f.post(t, "/worlds/"+f.worldID.String()+"/units/"+id+"/cancel-training", map[string]any{})
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	state := snapshotRecruitState(t, f)
	if state.Goods["cedar"] != 749 || state.Goods["timber"] != 0 {
		t.Fatalf("cancellation converted cedar into timber: %+v", state.Goods)
	}
}

func TestRecruitQuoteNamesEveryBlockingGate(t *testing.T) {
	for _, tc := range []struct {
		name, setup, want string
		can               bool
	}{
		{"goods", `UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 10 ELSE 0 END,rate=0 WHERE settlement_id=$1`, "needs 30 timber (or cedar) (have 10)", false},
		{"building", `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='shipyard'`, "needs a shipyard", false},
		{"cedar", `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'cedar',749,0,10000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=749`, "", true},
		{"queue", `INSERT INTO units(world_id,owner_id,settlement_id,type,category,size,crew,status,shipyard_work_remaining) SELECT s.world_id,s.owner_id,s.id,'galley','naval',1,20,'forming',3 FROM settlements s CROSS JOIN generate_series(1,10) WHERE s.id=$1`, "shipyard queue full", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			if tc.name == "cedar" {
				if _, err := f.pool.Exec(context.Background(), `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`, f.settlementID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.pool.Exec(context.Background(), tc.setup, f.settlementID); err != nil {
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
					Rows []struct {
						Unit    string   `json:"unit"`
						Can     bool     `json:"can_recruit"`
						Reasons []string `json:"reasons"`
					} `json:"can_recruit"`
				} `json:"settlement"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, r := range body.Settlement.Rows {
				if r.Unit == "galley" {
					found = true
					if r.Can != tc.can || !strings.Contains(strings.Join(r.Reasons, "; "), tc.want) {
						t.Fatalf("quote=%+v want can%v reason%q", r, tc.can, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("missing galley")
			}
			posted, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
			if tc.can && posted.Code != 201 || !tc.can && posted.Code != 422 {
				t.Fatalf("actual Recruit disagrees with quote: %d %s", posted.Code, posted.Body.String())
			}
		})
	}
}
