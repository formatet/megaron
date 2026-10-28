package transport

import (
	"context"
	"strings"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestDispatchExplicitChoiceRejectsCategorySubstitution(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	for _, by := range []string{"sea", "sky"} {
		t.Run(by, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = CreateShadow(ctx, tx, DispatchParams{By: by, WorldID: f.worldID, OwnerID: f.owner, Kind: "trade_return", OriginID: f.sourceID, DestID: f.destID, Category: "land", OriginQ: 0, OriginR: 0, DestQ: 3, DestR: 0, DepartsAt: time.Now(), Manifest: Manifest{"grain": 1}})
			if err == nil {
				t.Fatalf("explicit %s silently used land", by)
			}
		})
	}
}

// Call the dispatcher itself: HTTP handlers already pick a matching category
// and therefore cannot exercise its final explicit-choice backstop.
func TestDispatchLandChoiceRejectsOtherwiseValidNavalTransport(t *testing.T) {
	for _, by := range []string{"land", "sea"} {
		t.Run(by, func(t *testing.T) {
			pool := testPool(t)
			f := newNavalFixture(t, pool)
			ctx := context.Background()
			ship := mkShip(t, pool, f, f.sourceID, "galley")
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',10,0,1000,0)`, f.sourceID); err != nil {
				t.Fatal(err)
			}
			clk := clock.NewTestClock(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if err := BindShip(ctx, tx, ship); err != nil {
				t.Fatal(err)
			}
			id, err := Dispatch(ctx, tx, events.NewScheduler(pool, clk), DispatchParams{
				By: by, WorldID: f.worldID, OwnerID: f.owner, Kind: "transfer", OriginID: f.sourceID, DestID: f.destID,
				Category: "naval", DestQ: 3, DepartsAt: clk.Now(), ShipUnitID: &ship, Manifest: Manifest{"grain": 1},
			})
			if by == "sea" {
				if err != nil {
					t.Fatalf("valid sea control refused: %v", err)
				}
				var category, mode string
				if err := tx.QueryRow(ctx, `SELECT category,route_mode FROM transports WHERE id=$1`, id).Scan(&category, &mode); err != nil {
					t.Fatal(err)
				}
				if category != "naval" || mode != "sea" {
					t.Fatalf("sea control stored %s/%s", category, mode)
				}
				return
			}
			if err == nil {
				t.Fatal("explicit land choice dispatched a naval transport")
			}
			if !strings.Contains(err.Error(), "selected land transport cannot use naval route") {
				t.Fatalf("land/naval refused for the wrong reason: %v", err)
			}
			if id != uuid.Nil {
				t.Fatalf("rejected dispatch returned transport %s", id)
			}
			var movers, manifests, arrivals int
			if err := tx.QueryRow(ctx, `SELECT
    (SELECT count(*) FROM transports WHERE world_id=$1),
    (SELECT count(*) FROM transport_goods g JOIN transports t ON t.id=g.transport_id WHERE t.world_id=$1),
    (SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type IN ('TransportArrival','TransportArrivalV2'))`, f.worldID).Scan(&movers, &manifests, &arrivals); err != nil {
				t.Fatal(err)
			}
			if movers != 0 || manifests != 0 || arrivals != 0 {
				t.Fatalf("refused dispatch wrote movers=%d manifests=%d arrivals=%d", movers, manifests, arrivals)
			}
			var food, aboard float64
			if err := tx.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'`, f.sourceID).Scan(&food); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRow(ctx, `SELECT provisions FROM units WHERE id=$1`, ship).Scan(&aboard); err != nil {
				t.Fatal(err)
			}
			if food != 10 || aboard != 0 {
				t.Fatalf("refused dispatch loaded food: port=%g aboard=%g", food, aboard)
			}
		})
	}
}
