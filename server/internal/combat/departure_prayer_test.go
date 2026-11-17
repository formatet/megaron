package combat

import (
	"context"
	"encoding/json"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
)

func TestBetaStillnessDamageSnapshotAndReturn(t *testing.T) {
	for _, freight := range []bool{false, true} {
		t.Run(map[bool]string{false: "march", true: "freight"}[freight], func(t *testing.T) {
			pool, world, owner, route := seaStormWorld(t)
			ctx := context.Background()
			ship := uuid.Nil
			if freight {
				ship, _ = mkStormFreight(t, pool, world, owner, route)
			} else {
				ship, _ = mkStormFleet(t, pool, world, owner, route)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET stillness_depart_tick=0,stillness_arrive_tick=10,stillness_owner_id=$2 WHERE id=$1`, ship, owner); err != nil {
				t.Fatal(err)
			}
			stormsOverHexes(t, pool, world, 1, 2, 4, 5, 7)
			h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
			h.Dice = newStormDice(1)
			for day := 0; day <= 6; day++ {
				runStormDay(t, h, world, day)
			}
			if _, hull := shipState(t, pool, ship); hull != 5 {
				t.Fatalf("Stillness failed: hull%d", hull)
			}
			if countShipEvents(t, pool, ship, EventShipStormDamaged) != 0 {
				t.Fatal("protected voyage emitted damage")
			}
			// The same old stamp cannot protect a different voyage.
			if freight {
				_, err := pool.Exec(ctx, `INSERT INTO transports(world_id,owner_id,kind,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,status,interceptable,ship_unit_id,journey,departed_tick) SELECT world_id,owner_id,'ship_return',category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,11,'in_transit',true,ship_unit_id,journey,1 FROM transports WHERE ship_unit_id=$1`, ship)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = pool.Exec(ctx, `UPDATE transports SET status='delivered' WHERE ship_unit_id=$1 AND departed_tick=0`, ship)
			} else {
				route.StartTick = 1
				route.EndTick = 11
				raw, _ := json.Marshal(route)
				_, _ = pool.Exec(ctx, `UPDATE units SET depart_tick=1,arrive_tick=11,march_route=$2 WHERE id=$1`, ship, raw)
			}
			runStormDay(t, h, world, 1)
			if _, hull := shipState(t, pool, ship); hull != 3 {
				t.Fatalf("next leg inherited Stillness: hull%d", hull)
			}
		})
	}
}

func TestBetaStillnessDoesNotFollowShipOwner(t *testing.T) {
	pool, world, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, world, owner, route)
	ctx := context.Background()
	var other uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET stillness_depart_tick=0,stillness_arrive_tick=10,stillness_owner_id=$2,owner_id=$3 WHERE id=$1`, ship, owner, other); err != nil {
		t.Fatal(err)
	}
	stormsOverHexes(t, pool, world, 1, 2)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(1)
	runStormDay(t, h, world, 0)
	if _, hull := shipState(t, pool, ship); hull != 3 {
		t.Fatalf("captured owner inherited Stillness: hull%d", hull)
	}
}
