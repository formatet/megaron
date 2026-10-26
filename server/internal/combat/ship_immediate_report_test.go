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
	"formatet/megaron/server/internal/unit"
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
	for _, offset := range []time.Duration{0, 44 * time.Second, 104 * time.Second} {
		name := "at_boundary"
		if offset > 0 {
			name = "reported_phase"
		}
		if offset > 60*time.Second {
			name = "delayed_world_worker"
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
			t.Run("claimed_legacy_queue_row_keeps_its_identity", func(t *testing.T) {
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				if _, err = tx.Exec(ctx, `DELETE FROM scheduled_events WHERE world_id=$1 AND event_type=$2`, wid, string(events.ScheduledUnitArrivalV2)); err != nil {
					t.Fatal(err)
				}
				p := unit.ScheduledUnitArrivalPayload{UnitID: ship, WorldID: wid, ArriveTick: &res.ArrivalTick}
				body, _ := json.Marshal(p)
				var id int64
				if err = tx.QueryRow(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,attempts) VALUES($1,$2,$3,$4,$5,1) RETURNING id`, wid, string(events.ScheduledUnitArrival), body, clk.Now(), res.ArrivalTick).Scan(&id); err != nil {
					t.Fatal(err)
				}
				if err = enqueueUnitArrivalTx(ctx, tx, events.NewScheduler(pool, clk), wid, p, res.ArrivalTick); err != nil {
					t.Fatal(err)
				}
				var typ string
				if err = tx.QueryRow(ctx, `SELECT event_type FROM scheduled_events WHERE id=$1`, id).Scan(&typ); err != nil {
					t.Fatal(err)
				}
				if typ != string(events.ScheduledUnitArrival) {
					t.Fatal("already claimed legacy row was rewritten; old worker could mark the replacement done")
				}
				var n int
				if err = tx.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type=$2`, wid, string(events.ScheduledUnitArrivalV2)).Scan(&n); err != nil || n != 1 {
					t.Fatalf("replacement arrival count%d err%v", n, err)
				}
			})

			t.Run("stale_claimed_legacy_arrival_cannot_finish_v2_course", func(t *testing.T) {
				body, _ := json.Marshal(map[string]any{"unit_id": ship, "world_id": wid, "arrive_tick": res.ArrivalTick})
				h := NewUnitArrivalHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clk), clk)
				if err := h.Handle(ctx, events.ScheduledEvent{EventType: events.ScheduledUnitArrival, WorldID: wid, DueTick: res.ArrivalTick, Payload: body}); err != nil {
					t.Fatal(err)
				}
				var same bool
				if err := pool.QueryRow(ctx, `SELECT march_route=$2::jsonb AND status='marching' FROM units WHERE id=$1`, ship, raw).Scan(&same); err != nil {
					t.Fatal(err)
				}
				if !same {
					t.Fatal("an already claimed legacy event changed a new version2 voyage")
				}
			})

			t.Run("live_position_while_world_tick_worker_lags", func(t *testing.T) {
				halfway := res.DepartsAt.Add(time.Duration(res.DurationTicks*tick.TickSeconds) * time.Second / 2)
				pos, err := RoutePositionAt(route, anchor.MilliAt(halfway), halfway)
				if err != nil || pos.Q != route.Hexes[1][0] || pos.R != route.Hexes[1][1] {
					t.Fatalf("lagged world froze physical voyage: pos%v err%v", pos, err)
				}
			})

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
						// Unentered route tiles may already be visible across open sea.
						if entered[i] <= at && !known {
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
