package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
			// Low stores still suffice for the nearer destination city. Dock is
			// a food stop, not a patrol that must abandon its mission early.
			ration := VoyageRation("merchantman", 1, "spearman", len(ids)*100, 10)
			clk.Set(result.ArrivesAt.Add(-time.Duration(tick.TickSeconds) * time.Second))
			if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, world, result.ArrivalTick-1, clk.Now()); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET provisions=$2,provision_goods=jsonb_build_object('grain',$2::float8) WHERE id=$1`, ship, 2*ration); err != nil {
				t.Fatal(err)
			}
			arrivals := NewUnitArrivalHandler(pool, store, nil, sched, clk)
			upkeep := NewUpkeepHandler(pool, sched, store, nil, arrivals, clk)
			if err := upkeep.returnForFood(ctx, upkeepUnitRow{id: ship, category: "naval", status: "marching"}, world); err != nil {
				t.Fatal(err)
			}
			var activeIntent string
			if err := pool.QueryRow(ctx, `SELECT march_intent FROM units WHERE id=$1`, ship).Scan(&activeIntent); err != nil {
				t.Fatal(err)
			}
			if activeIntent != "dock" {
				t.Fatalf("nearby city food stop must preserve docking mission, got %s", activeIntent)
			}
			clk.Set(result.ArrivesAt)
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
			// Departures use the physical city, never the unchanged support anchor.
			var before, after float64
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, home).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET provisions=0,provision_goods='{}' WHERE id=$1`, ship); err != nil {
				t.Fatal(err)
			}
			// Arrival returned some food here; remove it to prove the rich remote
			// support city cannot fund a departure from this empty port.
			if _, err := pool.Exec(ctx, `DELETE FROM settlement_goods WHERE settlement_id=$1`, city); err != nil {
				t.Fatal(err)
			}
			order := MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 0, TargetR: 0}
			for _, preview := range []bool{true, false} {
				var err error
				if preview {
					_, err = PreviewMarch(ctx, pool, clk, order, nil)
				} else {
					_, err = StartMarch(ctx, pool, sched, store, clk, order, nil)
				}
				if err == nil {
					t.Fatalf("empty physical departure port must reject (preview=%v) despite rich support city", preview)
				}
			}
			var status string
			var onboard float64
			if err := pool.QueryRow(ctx, `SELECT status,provisions FROM units WHERE id=$1`, ship).Scan(&status, &onboard); err != nil {
				t.Fatal(err)
			}
			if status != "garrison" || onboard != 0 {
				t.Fatalf("rejection changed ship: %s food=%v", status, onboard)
			}
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, home).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("failed departure consumed remote food: %v -> %v", before, after)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',10000,0,20000,0)`, city); err != nil {
				t.Fatal(err)
			}
			// An actual dock is sufficient without any support anchor.
			if intent == "" {
				if _, err := pool.Exec(ctx, `UPDATE units SET support_settlement_id=NULL WHERE id=$1`, ship); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := PreviewMarch(ctx, pool, clk, order, nil); err != nil {
				t.Fatalf("physical port preview: %v", err)
			}
			if _, err := StartMarch(ctx, pool, sched, store, clk, order, nil); err != nil {
				t.Fatalf("next voyage must load from physical city: %v", err)
			}
			var physical float64
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, city).Scan(&physical); err != nil {
				t.Fatal(err)
			}
			if physical >= 10000 {
				t.Fatalf("physical departure city did not provision next voyage: %v", physical)
			}
			if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, home).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("departure changed remote support stock: %v -> %v", before, after)
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

// A database settlement reference is a docking witness only while its owner,
// world and position still match. No invalid reference falls back to support.
func TestShipDeparturePortRequiresPhysicalOwnedActiveCity(t *testing.T) {
	pool, _, _, ship, _ := setupLandMission(t)
	ctx := context.Background()
	var home uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT settlement_id FROM units WHERE id=$1`, ship).Scan(&home); err != nil {
		t.Fatal(err)
	}
	check := func(want bool) {
		t.Helper()
		port, err := economy.ShipDeparturePort(ctx, pool, ship)
		if err != nil {
			t.Fatal(err)
		}
		if (port != nil) != want || (port != nil && *port != home) {
			t.Fatalf("physical port=%v want access=%v", port, want)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x')`, "port-foreign-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	check(true) // Legacy NULL q/r is valid: settlement_id is the docking witness.
	for _, change := range []string{
		"UPDATE units SET q=9,r=0 WHERE id=$1",
		"UPDATE units SET status='positioned',q=1,r=0 WHERE id=$1",
		"UPDATE units SET owner_id=(SELECT id FROM players WHERE id<>units.owner_id LIMIT 1) WHERE id=$1",
	} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, change, ship); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		port, err := economy.ShipDeparturePort(ctx, tx, ship)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if port != nil {
			t.Fatalf("invalid docking witness received food access after %s", change)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET support_settlement_id=$2,settlement_id=NULL,status='positioned',q=1,r=0 WHERE id=$1`, ship, home); err != nil {
		t.Fatal(err)
	}
	check(false)
}

func TestShipAtSeaOrderNeverLoadsRemoteSupportFood(t *testing.T) {
	pool, world, owner, ship, _ := setupLandMission(t)
	ctx := context.Background()
	var home uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT settlement_id FROM units WHERE id=$1`, ship).Scan(&home); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET support_settlement_id=$2,settlement_id=NULL,status='positioned',q=5,r=0,provisions=2,provision_goods='{"grain":2}' WHERE id=$1`, ship, home); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	order := MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 0, TargetR: 0}
	if _, err := PreviewMarch(ctx, pool, clk, order, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := StartMarch(ctx, pool, sched, store, clk, order, nil); err != nil {
		t.Fatal(err)
	}
	var food, onboard float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, home).Scan(&food); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT provisions FROM units WHERE id=$1`, ship).Scan(&onboard); err != nil {
		t.Fatal(err)
	}
	if food != 10000 || onboard != 2 {
		t.Fatalf("at-sea order teleported food: support=%v aboard=%v", food, onboard)
	}
}

func seedDepartureFood(t *testing.T, pool *pgxpool.Pool, city uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',10000,0,20000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0`, city); err != nil {
		t.Fatal(err)
	}
}
func seedDepartureFoodForShip(t *testing.T, pool *pgxpool.Pool, ship uuid.UUID) {
	t.Helper()
	var city uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT settlement_id FROM units WHERE id=$1`, ship).Scan(&city); err != nil {
		t.Fatal(err)
	}
	seedDepartureFood(t, pool, city)
}
