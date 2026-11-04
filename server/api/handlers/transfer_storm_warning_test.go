package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/events"
)

// A transfer by sea warns (preview and receipt) when the freighting ship's own
// hull would not survive a storm; a sound hull gets no warning.
func TestTransferBySeaStormWarning(t *testing.T) {
	for _, hull := range []int{5, 2} {
		t.Run(fmt.Sprintf("hull_%d", hull), func(t *testing.T) {
			f := setupTradeNavalFixture(t)
			origin, prov := f.settlement(t, "Origin", 0, true, true)
			dest, _ := f.settlement(t, "Destination", 5, true, false)
			for q := 1; q < 5; q++ {
				f.mapTile(t, q, 0, "coastal_sea")
			}
			ship := f.ship(t, origin, "merchantman")
			if _, err := f.pool.Exec(context.Background(), `UPDATE units SET hull=$2 WHERE id=$1`, ship, hull); err != nil {
				t.Fatal(err)
			}
			h := NewProvinceHandler(f.pool, f.scheduler, f.clk, events.NewStore(f.pool), nil)
			f.router.Get("/worlds/{worldID}/provinces/{provinceID}/trade/preview", h.TradePreview)
			req := httptest.NewRequest("GET", fmt.Sprintf("/worlds/%s/provinces/%s/trade/preview?destination_id=%s&quantity=10&by=sea", f.worldID, prov, dest), nil)
			req.Header.Set("Authorization", "Bearer "+f.token)
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("preview %d %s", rec.Code, rec.Body.String())
			}
			var prev map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &prev)
			code, body := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, prov), map[string]any{"destination_id": dest, "good_key": "grain", "quantity": 10, "by": "sea"})
			if code != 201 {
				t.Fatalf("dispatch %d %+v", code, body)
			}
			for name, v := range map[string]any{"preview": prev["storm_warning"], "receipt": body["storm_warning"]} {
				got, _ := v.(string)
				if hull == 2 && !strings.Contains(got, "will sink it") {
					t.Errorf("%s: damaged hull got no warning: %v", name, v)
				}
				if hull == 5 && got != "" {
					t.Errorf("%s: sound hull warned: %q", name, got)
				}
			}
		})
	}
}
