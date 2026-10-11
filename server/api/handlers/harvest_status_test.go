package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
)

func TestProvinceHarvestBlessingStatus(t *testing.T) {
	pool := armyDisplayTestPool(t)
	worldID, settID := riteHarvestFixture(t, pool, 100)
	ctx := context.Background()
	var provID string
	if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, settID).Scan(&provID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=100 WHERE id=$1`, worldID); err != nil {
		t.Fatal(err)
	}
	h := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/provinces/{provinceID}", h.Get)
	for _, tc := range []struct {
		name      string
		until     any
		remaining int
	}{
		{"active", 112, 12}, {"last tick", 101, 1}, {"end tick", 100, 0}, {"expired", 99, 0}, {"never blessed", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `UPDATE settlements SET harvest_blessing_until_tick=$2 WHERE id=$1`, settID, tc.until); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces/"+provID, nil))
			if rec.Code != 200 {
				t.Fatalf("GET %d: %s", rec.Code, rec.Body.String())
			}
			var resp struct {
				Settlement map[string]json.RawMessage `json:"settlement"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			raw := resp.Settlement["harvest_blessing"]
			t.Logf("settlement.harvest_blessing = %s", raw)
			if tc.remaining == 0 {
				if len(raw) > 0 && string(raw) != "null" {
					t.Fatalf("ended harvest blessing leaked active status: %s", raw)
				}
				return
			}
			var b struct {
				UntilTick      int      `json:"until_tick"`
				RemainingTicks int      `json:"remaining_ticks"`
				YieldBonus     float64  `json:"yield_bonus"`
				Goods          []string `json:"goods"`
			}
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatalf("active harvest blessing missing from settlement response: %v", err)
			}
			if b.RemainingTicks != tc.remaining || b.UntilTick != tc.until || b.YieldBonus != economy.HarvestBlessingYieldBonus || !reflect.DeepEqual(b.Goods, economy.HarvestBlessedGoods()) {
				t.Fatalf("harvest status contradicts economy/column: %+v", b)
			}
		})
	}
}
