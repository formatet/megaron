package combat

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// Hiketeia (Timothy 2026-10-24): attacking a fallen Wanax's risen Nomadic
// Host before it has founded a city is allowed, and costs the attacker 70
// kharis when the battle actually starts. A host that has founded, or a
// newcomer's host, costs nothing.
func TestHiketeia_AttackingARisenHostCostsKharis(t *testing.T) {
	for _, tc := range []struct {
		name             string
		refugee, active  bool
		kharis, wantLeft float64
	}{
		{"risen host", true, true, 100, 30},
		{"floor holds", true, true, 50, 1},
		{"newcomer host", false, true, 100, 100},
		{"already founded", true, false, 100, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
				t.Fatal(err)
			}
			var worldID uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES($1,'active') RETURNING id`, "hiketeia-"+uuid.NewString()).Scan(&worldID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, worldID) })
			mk := func(tag string) uuid.UUID {
				var id uuid.UUID
				if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, tag+"-"+uuid.NewString()).Scan(&id); err != nil {
					t.Fatal(err)
				}
				return id
			}
			attacker, fallen := mk("attacker"), mk("fallen")
			if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,0,0)`, attacker, worldID, tc.kharis); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,1,0,'plains')`, worldID); err != nil {
				t.Fatal(err)
			}
			var host uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES($1,$2,'nomadic_host','land',1,0,'positioned',1,0) RETURNING id`, worldID, fallen).Scan(&host); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES($1,$2,'spearman','land',50,0,'positioned',1,0)`, worldID, fallen); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO founder_phase(world_id,owner_id,host_unit_id,population,grain_amount,grain_rate,silver_amount,silver_rate,calc_tick,active,is_refugee) VALUES($1,$2,$3,500,0,0,60,0,0,$4,$5)`, worldID, fallen, host, tc.active, tc.refugee); err != nil {
				t.Fatal(err)
			}
			var unitID uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,target_q,target_r,capture_mode) VALUES($1,$2,'spearman','land',100,0,'marching',0,0,1,0,'sack') RETURNING id`, worldID, attacker).Scan(&unitID); err != nil {
				t.Fatal(err)
			}
			h := &UnitArrivalHandler{pool: pool, eventStore: events.NewStore(pool), hub: &fakeBroadcaster{},
				scheduler: events.NewScheduler(pool, clock.NewTestClock(time.Now())), clk: clock.NewTestClock(time.Now())}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.resolve(ctx, tx, unitID, worldID); err != nil {
				tx.Rollback(ctx)
				t.Fatalf("resolve: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			// A second attacking unit joining the same battle is the same breach.
			var second uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,target_q,target_r,capture_mode) VALUES($1,$2,'spearman','land',100,0,'marching',0,0,1,0,'sack') RETURNING id`, worldID, attacker).Scan(&second); err != nil {
				t.Fatal(err)
			}
			tx, _ = pool.Begin(ctx)
			if err := h.resolve(ctx, tx, second, worldID); err != nil {
				tx.Rollback(ctx)
				t.Fatalf("resolve second: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var left float64
			var broken int
			if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, attacker, worldID).Scan(&left); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='HiketeiaBroken'`, worldID).Scan(&broken); err != nil {
				t.Fatal(err)
			}
			if left != tc.wantLeft {
				t.Fatalf("attacker kharis %v, want %v", left, tc.wantLeft)
			}
			if want := map[bool]int{true: 1, false: 0}[tc.wantLeft != tc.kharis]; broken != want {
				t.Fatalf("%d HiketeiaBroken events, want %d", broken, want)
			}
		})
	}
}
