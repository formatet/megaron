package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestStandingOrderChoiceCreateListAndUnavailableReject(t *testing.T) {
	for _, c := range []struct {
		by         string
		ship, land bool
		code       int
	}{{"land", true, true, 201}, {"sea", true, true, 201}, {"sea", false, true, 422}, {"land", true, false, 422}, {"sky", true, true, 400}} {
		t.Run(fmt.Sprintf("by_%s_ship_%v_land_%v", c.by, c.ship, c.land), func(t *testing.T) {
			f := setupTradeNavalFixture(t)
			ctx := context.Background()
			origin, _ := f.settlement(t, "Origin", 0, true, true)
			dest, _ := f.settlement(t, "Destination", 5, true, false)
			for q := 1; q < 5; q++ {
				f.mapTile(t, q, 0, "coastal_sea")
			}
			if c.land {
				for q := 0; q <= 5; q++ {
					f.mapTile(t, q, 1, "plains")
				}
			}
			if c.ship {
				f.ship(t, origin, "merchantman")
			}
			h := NewStandingOrderHandler(f.pool)
			f.router.Post("/worlds/{worldID}/standing-orders", h.Create)
			f.router.Get("/worlds/{worldID}/standing-orders", h.List)
			path := fmt.Sprintf("/worlds/%s/standing-orders", f.worldID)
			code, body := f.post(t, path, map[string]any{"from_settlement_id": origin, "to_settlement_id": dest, "crewed_by_settlement_id": origin, "outbound": []map[string]any{{"good_key": "grain", "threshold": 100}}, "by": c.by})
			if code != c.code {
				t.Fatalf("create %d want%d %+v", code, c.code, body)
			}
			if code != 201 {
				var n int
				_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM standing_orders WHERE world_id=$1`, f.worldID).Scan(&n)
				if n != 0 {
					t.Fatalf("rejected choice saved %d routes", n)
				}
				return
			}
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", "Bearer "+f.token)
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			var got []standingOrderOut
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].By != c.by || body["by"] != c.by {
				t.Fatalf("choice not exposed %+v response%v", got, body)
			}
			var saved string
			if err := f.pool.QueryRow(ctx, `SELECT route_mode FROM standing_orders WHERE id=$1`, got[0].ID).Scan(&saved); err != nil || saved != c.by {
				t.Fatalf("saved%q err%v", saved, err)
			}
		})
	}
}
