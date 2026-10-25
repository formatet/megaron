package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestCityLandingAllPassengersStayAndSupportUnchanged(t *testing.T) {
	for _, intent := range []string{"land", "", "empty"} {
		t.Run("intent="+intent, func(t *testing.T) {
			pool, world, owner, ship, cargo := setupLandMission(t)
			ctx := context.Background()
			var home uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT settlement_id FROM units WHERE id=$1`, ship).Scan(&home); err != nil {
				t.Fatal(err)
			}
			var prov, city uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES($1,10,0,'plains',true) RETURNING id`, world).Scan(&prov); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,population) VALUES($1,$2,'Destination','achaean',$3,'colony',500) RETURNING id`, world, prov, owner).Scan(&city); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET type='merchantman',crew=10,support_settlement_id=$2 WHERE id=$1`, ship, home); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET support_settlement_id=$2 WHERE id=$1`, cargo, home); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',10000,0,20000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0`, home); err != nil {
				t.Fatal(err)
			}
			ids := []uuid.UUID{cargo}
			for i := 0; i < 2; i++ {
				var id uuid.UUID
				if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,carried_by_ship_id,support_settlement_id) VALUES($1,$2,'spearman','land',100,'embarked',$3,$4) RETURNING id`, world, owner, ship, home).Scan(&id); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, id)
			}
			if intent == "empty" {
				intent = ""
				if _, err := pool.Exec(ctx, `UPDATE units SET status='garrison',carried_by_ship_id=NULL,settlement_id=$2,q=0,r=0 WHERE carried_by_ship_id=$1`, ship, home); err != nil {
					t.Fatal(err)
				}
				ids = nil
			}
			clk := clock.NewTestClock(time.Now())
			sched := events.NewScheduler(pool, clk)
			store := events.NewStore(pool)
			result, err := StartMarch(ctx, pool, sched, store, clk, MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 10, TargetR: 0, Intent: intent}, nil)
			if err != nil {
				t.Fatalf("own city must accept ship and passengers: %v", err)
			}
			if result.TargetQ != 9 || result.TargetR != 0 {
				t.Fatalf("must sail to the saved coastal approach of the selected city, got %+v", result)
			}
			markUnitArrivalProcessed(t, pool, world, ship)
			h := newArrivalHandler(pool, nil)
			runFieldArrival(t, pool, h, world, ship)
			runFieldArrival(t, pool, h, world, ship)
			for _, id := range append(ids, ship) {
				var status string
				var sid, support, carrier *uuid.UUID
				var q, r int
				if err := pool.QueryRow(ctx, `SELECT status,settlement_id,support_settlement_id,carried_by_ship_id,q,r FROM units WHERE id=$1`, id).Scan(&status, &sid, &support, &carrier, &q, &r); err != nil {
					t.Fatal(err)
				}
				if status != "garrison" || sid == nil || *sid != city || support == nil || *support != home || carrier != nil || q != 10 || r != 0 {
					t.Fatalf("arrival id%s status%s city%v support%v carrier%v pos%d,%d", id, status, sid, support, carrier, q, r)
				}
			}
			var returns int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND payload->>'unit_id'=$2 AND processed_at IS NULL`, world, ship.String()).Scan(&returns); err != nil {
				t.Fatal(err)
			}
			if returns != 0 {
				t.Fatalf("ship queued return: %d", returns)
			}
			// A subsequent player-given voyage is provisioned by the old support
			// city, per Timothy's explicit A decision, even while docked here.
			var before, after float64
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, home).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET provisions=0,provision_goods='{}' WHERE id=$1`, ship); err != nil {
				t.Fatal(err)
			}
			clk.Set(result.ArrivesAt)
			if _, err := StartMarch(ctx, pool, sched, store, clk, MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 0, TargetR: 0}, nil); err != nil {
				t.Fatalf("next voyage must load from original support city: %v", err)
			}
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, home).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after >= before {
				t.Fatalf("original support city did not provision next voyage: before%v after%v", before, after)
			}

		})
	}
}

func TestCityLandingRechecksOwnershipAndKeepsPassengers(t *testing.T) {
	pool, world, owner, ship, cargo := setupLandMission(t)
	ctx := context.Background()
	var prov, city uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES($1,10,0,'plains',true) RETURNING id`, world).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type) VALUES($1,$2,'Changing port','achaean',$3,'colony') RETURNING id`, world, prov, owner).Scan(&city); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	if _, err := StartMarch(ctx, pool, sched, store, clk, MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 10, Intent: "land"}, nil); err != nil {
		t.Fatal(err)
	}
	var foreign uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "new-owner-"+uuid.NewString()).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, city, foreign); err != nil {
		t.Fatal(err)
	}
	markUnitArrivalProcessed(t, pool, world, ship)
	runFieldArrival(t, pool, newArrivalHandler(pool, nil), world, ship)
	var shipStatus, intent, cargoStatus string
	var carrier *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT status,march_intent FROM units WHERE id=$1`, ship).Scan(&shipStatus, &intent); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,carried_by_ship_id FROM units WHERE id=$1`, cargo).Scan(&cargoStatus, &carrier); err != nil {
		t.Fatal(err)
	}
	if shipStatus != "marching" || intent != "explore_return" || cargoStatus != "embarked" || carrier == nil || *carrier != ship {
		t.Fatalf("lost port must return with passengers, got ship%s/%s cargo%s carrier%v", shipStatus, intent, cargoStatus, carrier)
	}
}
