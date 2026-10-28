package handlers

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"testing"
)

func TestTransferExplicitLandDoesNotChooseAvailableSea(t *testing.T) {
	f := setupTradeNavalFixture(t)
	ctx := context.Background()
	origin, prov := f.settlement(t, "Choice origin", 0, true, true)
	dest, _ := f.settlement(t, "Choice destination", 5, true, false)
	for q := 1; q < 5; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	for q := 0; q <= 5; q++ {
		f.mapTile(t, q, 1, "plains")
	}
	ship := f.ship(t, origin, "merchantman")
	code, body := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, prov), map[string]any{"destination_id": dest, "good_key": "grain", "quantity": 10, "by": "land"})
	if code != 201 {
		t.Fatalf("dispatch %d %+v", code, body)
	}
	var category, status string
	if err := f.pool.QueryRow(ctx, `SELECT category FROM transports WHERE world_id=$1`, f.worldID).Scan(&category); err != nil {
		t.Fatal(err)
	}
	if category != "land" {
		t.Fatalf("explicit land silently chose %s", category)
	}
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, ship).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "garrison" {
		t.Fatalf("land choice bound ship: %s", status)
	}
}

func TestTransferChoiceStrictnessAndPersistence(t *testing.T) {
	for _, c := range []struct {
		by         string
		ship, land bool
		code       int
		category   string
	}{{"land", true, true, 201, "land"}, {"sea", true, true, 201, "naval"}, {"", true, true, 201, "naval"}, {"", false, true, 201, "land"}, {"sea", false, true, 422, ""}, {"land", true, false, 422, ""}, {"sky", true, true, 400, ""}} {
		t.Run(fmt.Sprintf("by_%s_ship_%v_land_%v", c.by, c.ship, c.land), func(t *testing.T) {
			f := setupTradeNavalFixture(t)
			ctx := context.Background()
			origin, prov := f.settlement(t, "Origin", 0, true, true)
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
			code, body := f.post(t, fmt.Sprintf("/worlds/%s/provinces/%s/trade", f.worldID, prov), map[string]any{"destination_id": dest, "good_key": "grain", "quantity": 10, "by": c.by})
			if code != c.code {
				t.Fatalf("dispatch %d want%d %+v", code, c.code, body)
			}
			if code != 201 {
				var n int
				if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&n); err != nil || n != 0 {
					t.Fatalf("rejected choice dispatched %d %v", n, err)
				}
				var grain float64
				_ = f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, origin).Scan(&grain)
				if grain != 100000 {
					t.Fatalf("rejected choice debited %g", grain)
				}
				return
			}
			var cat, by, routeBy string
			if err := f.pool.QueryRow(ctx, `SELECT category,route_mode FROM transports WHERE world_id=$1`, f.worldID).Scan(&cat, &by); err != nil {
				t.Fatal(err)
			}
			if err := f.pool.QueryRow(ctx, `SELECT route_mode FROM trade_routes WHERE world_id=$1`, f.worldID).Scan(&routeBy); err != nil {
				t.Fatal(err)
			}
			if cat != c.category || by != c.by || routeBy != c.by || body["by"] != c.by {
				t.Fatalf("choice not persisted category%q by%q route%q response%v", cat, by, routeBy, body)
			}
			if c.by == "sea" {
				handler := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
				if err := handler.Handle(ctx, f.lastTradeDeliveryEvent(t)); err != nil {
					t.Fatal(err)
				}
				var retBy string
				if err := f.pool.QueryRow(ctx, `SELECT route_mode FROM transports WHERE world_id=$1 AND kind='ship_return'`, f.worldID).Scan(&retBy); err != nil || retBy != "sea" {
					t.Fatalf("empty hull return lost selectedsea %q err%v", retBy, err)
				}
			}

		})
	}
}
