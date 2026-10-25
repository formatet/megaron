package combat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

// Report 43640c42: Sea Palace departed at 20:10:14 during tick103,
// whose nominal boundary was20:09:30. Reproduce on an isolated fresh DB;
// no live IDs or state are used. Val A intentionally enters the FIRST route
// hex at departure; it must not consume time from before the order existed.
func TestShipReport43640c42_DeparturePhase(t *testing.T) {
	oldSeconds := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = oldSeconds })
	for _, offset := range []time.Duration{0, 44 * time.Second} {
		name := "at_boundary"
		if offset > 0 {
			name = "reported_phase"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			pool := testPool(t)
			if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
				t.Fatal(err)
			}
			anchor := tick.Anchor{Tick: 103, At: time.Date(2026, 10, 24, 18, 9, 30, 0, time.UTC)}
			var wid, owner, pid, sid, ship uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick,last_tick_at) VALUES($1,'active',103,$2) RETURNING id`, "reported-phase-"+uuid.NewString(), anchor.At).Scan(&wid); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
			if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "reported-sailor-"+uuid.NewString()).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES($1,61,27,'forest_cedar',true) RETURNING id`, wid).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES($1,$2,'Home Port','achaean',$3,'capital',true,'active',1000) RETURNING id`, wid, pid, owner).Scan(&sid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',1000,0,10000,103)`, sid); err != nil {
				t.Fatal(err)
			}
			for q := 58; q <= 61; q++ {
				for r := 26; r <= 27; r++ {
					terrain := "deep_sea"
					if q == 60 {
						terrain = "coastal_sea"
					}
					if q == 61 {
						terrain = "forest_cedar"
					}
					if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,$4)`, wid, q, r, terrain); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,status,q,r,support_settlement_id,provisions) VALUES($1,$2,'galley','naval',1,20,5,'positioned',60,27,$3,100) RETURNING id`, wid, owner, sid).Scan(&ship); err != nil {
				t.Fatal(err)
			}
			clk := clock.NewTestClock(anchor.At.Add(offset))
			res, err := StartMarch(ctx, pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, MarchOrder{WorldID: wid, PlayerID: owner, UnitID: ship, TargetQ: 58, TargetR: 26, Intent: "explore"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var raw []byte
			if err := pool.QueryRow(ctx, `SELECT march_route FROM units WHERE id=$1`, ship).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var route StoredRoute
			if err := json.Unmarshal(raw, &route); err != nil {
				t.Fatal(err)
			}
			t.Logf("duration=%d ticks route=%s announced=%s nominal_due=%s departure_milli=%d", res.DurationTicks, raw, res.ArrivesAt.Format(time.RFC3339), anchor.WallAt(int64(res.ArrivalTick)*1000).Format(time.RFC3339), anchor.MilliAt(clk.Now()))
			if res.DurationTicks != 1 || len(route.Hexes) != 4 {
				t.Fatalf("fixture no longer reproduces reported three-step one-tick voyage: %+v", res)
			}
			t.Run("prefix_memory", func(t *testing.T) {
				entered, err := RouteEnterMilli(route)
				if err != nil {
					t.Fatal(err)
				}
				for _, at := range []int64{entered[1] - 1, entered[1], entered[2], entered[3]} {
					tx, err := pool.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					_, err = recordPlaceEntriesTx(ctx, tx, events.NewStore(pool), wid, at)
					if err != nil {
						_ = tx.Rollback(ctx)
						t.Fatal(err)
					}
					if err = tx.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					for i := 1; i < len(entered); i++ {
						var known bool
						if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=$3 AND r=$4)`, wid, owner, route.Hexes[i][0], route.Hexes[i][1]).Scan(&known); err != nil {
							t.Fatal(err)
						}
						if known != (entered[i] <= at) {
							t.Fatalf("at %d hex%d known=%v entry=%d", at, i, known, entered[i])
						}
					}
				}
			})

			t.Run("start_hex_at_dispatch", func(t *testing.T) {
				pos, err := RoutePositionAt(route, anchor.MilliAt(clk.Now()))
				if err != nil {
					t.Fatal(err)
				}
				first := province.MapPosition{Q: route.Hexes[0][0], R: route.Hexes[0][1]}
				if pos != first {
					t.Fatalf("newly dispatched ship already at %+v, want first entered hex %+v; consumed elapsed phase before departure", pos, first)
				}
			})
			t.Run("announced_eta_matches_due_tick_boundary", func(t *testing.T) {
				dueAt := anchor.WallAt(int64(res.ArrivalTick)*1000 + route.OffsetMilli)
				if !res.ArrivesAt.Equal(dueAt) {
					t.Fatalf("announced ETA %s differs from due-tick boundary %s by %s", res.ArrivesAt, dueAt, res.ArrivesAt.Sub(dueAt))
				}
			})
		})
	}
}
