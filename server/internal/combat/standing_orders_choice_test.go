package combat

import (
	"context"
	"fmt"
	"testing"
)

func TestStandingOrderExplicitChoiceControlsSweepWithoutFallback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, c := range []struct {
		by         string
		ship, land bool
		want       string
	}{{"land", true, true, "land"}, {"sea", true, true, "naval"}, {"sea", false, true, "paused"}, {"land", true, false, "paused"}, {"", true, true, "naval"}, {"", false, true, "land"}} {
		t.Run(fmt.Sprintf("by_%s_ship_%v_land_%v", c.by, c.ship, c.land), func(t *testing.T) {
			f := newCoastalStandingOrderFixture(t, pool, "explicit-choice")
			seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
			if c.land {
				for q := 0; q <= 5; q++ {
					if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,1,'plains')`, f.worldID, q); err != nil {
						t.Fatal(err)
					}
				}
			}
			if c.ship {
				shipAt(t, pool, f.worldID, f.owner, f.capitalID, "merchantman")
			}
			order := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
			addOutbound(t, pool, order, "grain", 200)
			if _, err := pool.Exec(ctx, `UPDATE standing_orders SET route_mode=$1 WHERE id=$2`, c.by, order); err != nil {
				t.Fatal(err)
			}
			runStandingOrderTick(t, pool, f)
			if c.want == "paused" {
				status, reason := orderStatus(t, pool, order)
				var n int
				_ = pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE standing_order_id=$1`, order).Scan(&n)
				if status != "paused" || reason == nil || n != 0 {
					t.Fatalf("explicit fallback status%q reason%v movers%d", status, reason, n)
				}
				return
			}
			if cat := transportCategoryForOrder(t, pool, order); cat != c.want {
				t.Fatalf("category%q want%q", cat, c.want)
			}
			var saved string
			if err := pool.QueryRow(ctx, `SELECT route_mode FROM transports WHERE standing_order_id=$1`, order).Scan(&saved); err != nil || saved != c.by {
				t.Fatalf("saved mode%q want%q err%v", saved, c.by, err)
			}
			if _, err := pool.Exec(ctx, `UPDATE transports SET status='delivered' WHERE standing_order_id=$1`, order); err != nil {
				t.Fatal(err)
			}
			runStandingOrderTick(t, pool, f)
			_, kind, status := latestTransportForOrder(t, pool, order)
			if kind != "standing_order_return" || status != "in_transit" {
				t.Fatalf("return not dispatched kind%q status%q", kind, status)
			}
			var returnBy, returnCategory string
			if err := pool.QueryRow(ctx, `SELECT route_mode,category FROM transports WHERE standing_order_id=$1 AND kind='standing_order_return'`, order).Scan(&returnBy, &returnCategory); err != nil {
				t.Fatal(err)
			}
			if returnBy != c.by || returnCategory != c.want {
				t.Fatalf("return ignored choice by%q category%q", returnBy, returnCategory)
			}

		})
	}
}
