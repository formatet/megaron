package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTransportOptionsBothModesAnchoredAndReadOnly(t *testing.T) {
	f := setupTradeNavalFixture(t)
	ctx := context.Background()
	origin, prov := f.settlement(t, "Origin", 0, true, true)
	dest, _ := f.settlement(t, "Destination", 5, true, false)
	for q := 1; q < 5; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	for q := 0; q <= 5; q++ {
		f.mapTile(t, q, 1, "plains")
	}
	ship := f.ship(t, origin, "merchantman")
	if _, err := f.pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_land_departure')`, origin, f.playerID); err != nil {
		t.Fatal(err)
	}
	h := NewProvinceHandler(f.pool, f.scheduler, f.clk, events.NewStore(f.pool), nil)
	f.router.Get("/worlds/{worldID}/settlements/{settlementID}/transport-options", h.TransportOptions)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/trade/preview", h.TradePreview)
	for _, path := range []string{fmt.Sprintf("/worlds/%s/settlements/%s/transport-options?destination_id=%s&quantity=10", f.worldID, origin, dest), fmt.Sprintf("/worlds/%s/provinces/%s/trade/preview?destination_id=%s&quantity=10", f.worldID, prov, dest)} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+f.token)
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("preview %d %s", rec.Code, rec.Body.String())
		}
		var out struct {
			Selected string `json:"selected_by"`
			Options  []struct {
				By           string    `json:"by"`
				Available    bool      `json:"available"`
				Ticks        int       `json:"travel_ticks"`
				Seconds      float64   `json:"travel_seconds"`
				At           time.Time `json:"arrives_at"`
				Requirements []string  `json:"requirements"`
				Risk         string    `json:"risk"`
			} `json:"options"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Options) != 2 || out.Selected != "sea" {
			t.Fatalf("missing alternatives %s", rec.Body.String())
		}
		for _, o := range out.Options {
			if !o.Available || o.Ticks <= 0 || o.Seconds <= 0 || len(o.Requirements) == 0 || o.Risk == "" {
				t.Fatalf("incomplete option %+v", o)
			}
			want := f.clk.Now().Add(tick.RealUntil(o.Ticks, 0))
			if o.At.Sub(want) > time.Millisecond || want.Sub(o.At) > time.Millisecond {
				t.Fatalf("preview clock drift %+v want%v", o, want)
			}
		}
	}
	if pending, err := religion.DeparturePending(ctx, f.pool, origin, f.playerID, religion.EffectNextLandDeparture); err != nil || !pending {
		t.Fatalf("preview consumed prayer pending%v err%v", pending, err)
	}
	var status string
	_ = f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, ship).Scan(&status)
	if status != "garrison" {
		t.Fatalf("preview bound ship %s", status)
	}
	var movers int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&movers)
	if movers != 0 {
		t.Fatalf("preview dispatched %d", movers)
	}
}
