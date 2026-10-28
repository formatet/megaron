package transport

import (
	"context"
	"errors"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/jackc/pgx/v5"
	"math"
	"strings"
	"testing"
	"time"
)

func TestFreightProvisionsDispatchAndShadowLifecycle(t *testing.T) {
	for _, kind := range []string{"transfer", "gift", "trade", "standing_order_out"} {
		t.Run(kind, func(t *testing.T) {
			pool := testPool(t)
			f := newNavalFixture(t, pool)
			ctx := context.Background()
			ship := mkShip(t, pool, f, f.sourceID, "galley")
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',10,0,1000,0)`, f.sourceID); err != nil {
				t.Fatal(err)
			}
			clk := clock.NewTestClock(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
			sched := events.NewScheduler(pool, clk)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if err := BindShip(ctx, tx, ship); err != nil {
				t.Fatal(err)
			}
			p := DispatchParams{WorldID: f.worldID, OwnerID: f.owner, Kind: kind, OriginID: f.sourceID, DestID: f.destID, Category: "naval", DestQ: 3, DepartsAt: clk.Now(), ShipUnitID: &ship}
			var loaded float64
			if kind == "trade" || kind == "gift" {
				_, err = CreateShadow(ctx, tx, p)
			} else {
				_, err = Dispatch(ctx, tx, sched, p)
			}
			if err != nil {
				t.Fatal(err)
			}
			var goods map[string]float64
			if err := tx.QueryRow(ctx, `SELECT provisions,provision_goods FROM units WHERE id=$1`, ship).Scan(&loaded, &goods); err != nil {
				t.Fatal(err)
			}
			if math.Abs(loaded-0.20) > 1e-9 || math.Abs(goods["fish"]-loaded) > 1e-9 {
				t.Fatalf("provisions=%g goods=%v; want round-trip fish", loaded, goods)
			}
			var city float64
			if err := tx.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'`, f.sourceID).Scan(&city); err != nil {
				t.Fatal(err)
			}
			if math.Abs(city+loaded-10) > 1e-9 {
				t.Fatalf("food was created or double-charged: city=%g aboard=%g", city, loaded)
			}
			// The same upkeep meal owner consumes only onboard food.
			fed, err := economy.EatProvisions(ctx, tx, ship, economy.VoyageRation("galley", 1, "", 0, 10))
			if err != nil || !fed {
				t.Fatalf("fed=%v err=%v", fed, err)
			}
			if _, err := tx.Exec(ctx, `UPDATE transports SET status='delivered' WHERE ship_unit_id=$1`, ship); err != nil {
				t.Fatal(err)
			}
			// Return must not remotely debit home, even when OriginID misleadingly
			// names it (the damaged-return writer uses this shape).
			p.Kind = "damaged_return"
			p.OriginQ = 3
			p.DestQ = 0
			p.DestID = f.sourceID
			p.Manifest = nil
			p.Journey = nil
			if _, err := Dispatch(ctx, tx, sched, p); err != nil {
				t.Fatal(err)
			}
			var after float64
			if err := tx.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'`, f.sourceID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != city {
				t.Fatal("return teleported port food")
			}
			for i := 0; i < 2; i++ {
				if err := ReleaseShip(ctx, tx, ship, f.sourceID); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.QueryRow(ctx, `SELECT provisions FROM units WHERE id=$1`, ship).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != 0 {
				t.Fatalf("release left %g aboard", after)
			}
			if err := tx.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='fish'`, f.sourceID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if math.Abs(after-9.90) > 1e-9 {
				t.Fatalf("returned fish=%g want9.90 exactly once", after)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFreightProvisionsShortageRollsBackBindingAndTransport(t *testing.T) {
	pool := testPool(t)
	f := newNavalFixture(t, pool)
	ctx := context.Background()
	ship := mkShip(t, pool, f, f.sourceID, "merchantman")
	func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := BindShip(ctx, tx, ship); err != nil {
			t.Fatal(err)
		}
		_, err = CreateShadow(ctx, tx, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, Kind: "trade", OriginID: f.sourceID, DestID: f.destID, Category: "naval", DestQ: 3, DepartsAt: time.Now(), ShipUnitID: &ship})
		if err == nil || !strings.Contains(err.Error(), "not enough food") {
			t.Fatalf("want shortage, got%v", err)
		}
	}()
	status, _ := unitStatus(t, pool, ship)
	if status != "garrison" {
		t.Fatal(status)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE ship_unit_id=$1`, ship).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("failed dispatch persisted mover")
	}
}

func TestFreightPortFollowsPhysicalLegNotHomeReference(t *testing.T) {
	pool := testPool(t)
	f := newNavalFixture(t, pool)
	ctx := context.Background()
	ship := mkShip(t, pool, f, f.sourceID, "merchantman")
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',10,0,1000,0)`, f.sourceID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := BindShip(ctx, tx, ship); err != nil {
		t.Fatal(err)
	}
	port, err := economy.FreightPort(ctx, tx, ship)
	if err != nil || port == nil || *port != f.sourceID {
		t.Fatalf("bound at source port=%v err=%v", port, err)
	}
	_, err = CreateShadow(ctx, tx, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, Kind: "trade", OriginID: f.sourceID, DestID: f.destID, Category: "naval", DestQ: 3, DepartsAt: time.Now(), ShipUnitID: &ship})
	if err != nil {
		t.Fatal(err)
	}
	port, err = economy.FreightPort(ctx, tx, ship)
	if err != nil || port != nil {
		t.Fatalf("at sea port=%v err=%v", port, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE transports SET status='delivered' WHERE ship_unit_id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	port, err = economy.FreightPort(ctx, tx, ship)
	if err != nil || port == nil || *port != f.destID {
		t.Fatalf("landed port=%v err=%v", port, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE transports SET status='lost' WHERE ship_unit_id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	port, err = economy.FreightPort(ctx, tx, ship)
	if err != nil || port != nil {
		t.Fatalf("lost leg port=%v err=%v", port, err)
	}
}

// Live 2026-10-28 (Horns of Crete): a ship released after a delivered leg
// and then moved as a unit to another own city must sail from where it now
// docks; the older transport is not location evidence after a new binding.
func TestFreightPortAfterReleaseAndMarchUsesUnitDock(t *testing.T) {
	pool := testPool(t)
	f := newNavalFixture(t, pool)
	ctx := context.Background()
	ship := mkShip(t, pool, f, f.sourceID, "merchantman")
	for _, s := range []any{f.sourceID, f.destID} {
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',10,0,1000,0)`, s); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(pool, clk)
	inTx := func(fn func(tx pgx.Tx) error) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	// Leg 1 source -> dest, delivered; the ship is released at dest.
	if err := inTx(func(tx pgx.Tx) error {
		if err := BindShip(ctx, tx, ship); err != nil {
			return err
		}
		_, err := Dispatch(ctx, tx, sched, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, Kind: "transfer", OriginID: f.sourceID, DestID: f.destID, Category: "naval", DestQ: 3, DepartsAt: clk.Now(), ShipUnitID: &ship})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := inTx(func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE transports SET status='delivered' WHERE ship_unit_id=$1`, ship); err != nil {
			return err
		}
		return ReleaseShip(ctx, tx, ship, f.destID)
	}); err != nil {
		t.Fatal(err)
	}
	// The ship marches back to source as a unit (unit arrival owns this row).
	if _, err := pool.Exec(ctx, `UPDATE units SET settlement_id=$2 WHERE id=$1`, ship, f.sourceID); err != nil {
		t.Fatal(err)
	}
	if err := inTx(func(tx pgx.Tx) error {
		if err := BindShip(ctx, tx, ship); err != nil {
			return err
		}
		port, err := economy.FreightPort(ctx, tx, ship)
		if err != nil || port == nil || *port != f.sourceID {
			t.Fatalf("rebound port=%v err=%v; want source", port, err)
		}
		_, err = Dispatch(ctx, tx, sched, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, Kind: "transfer", OriginID: f.sourceID, DestID: f.destID, Category: "naval", DestQ: 3, DepartsAt: clk.Now(), ShipUnitID: &ship})
		return err
	}); err != nil {
		t.Fatalf("dispatch from the city the ship marched to: %v", err)
	}
}

func TestDispatchShipRuleIsPlayerRefusal(t *testing.T) {
	pool := testPool(t)
	f := newNavalFixture(t, pool)
	ctx := context.Background()
	ship := mkShip(t, pool, f, f.sourceID, "merchantman")
	if _, err := pool.Exec(ctx, `UPDATE units SET name='Horns of Crete' WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := BindShip(ctx, tx, ship); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	// Ordered from dest, but the ship docks at source.
	_, err = Dispatch(ctx, tx, events.NewScheduler(pool, clk), DispatchParams{WorldID: f.worldID, OwnerID: f.owner, Kind: "transfer", OriginID: f.destID, DestID: f.sourceID, Category: "naval", OriginQ: 3, DepartsAt: clk.Now(), ShipUnitID: &ship})
	var refusal *DispatchRefusal
	if !errors.As(err, &refusal) || !strings.HasPrefix(refusal.Text, "Horns of Crete is not in harbour at ") {
		t.Fatalf("err=%v; want a player refusal naming the ship", err)
	}
}
