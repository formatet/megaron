package combat

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/carrier"
	"github.com/google/uuid"
)

func TestUpkeepCarrierFate_HarbourAndSea(t *testing.T) {
	for _, cause := range []string{"grain", "silver"} {
		for _, rescued := range []bool{false, true} {
			for _, place := range []string{"port", "adjacent", "sea", "marching", "freighting"} {
				label := "own/"
				if rescued {
					label = "rescued/"
				}
				t.Run(label+cause+"/"+place, func(t *testing.T) {
					pool := testPool(t)
					f := newStarvationFixture(t, pool, "t2-upkeep")
					ctx := context.Background()
					ship := mkStarvingShip(t, pool, f.worldID, f.ownerID, f.capitalID, f.capitalID)
					q, r := 3, 0
					status := "positioned"
					var settlement *uuid.UUID
					if place == "port" {
						q = 0
						status = "garrison"
						settlement = &f.capitalID
					}
					if place == "adjacent" {
						q = 1
					}
					if place == "marching" {
						q = 0
						status = "marching"
					}
					if place == "freighting" {
						q = 0
						status = "freighting"
						settlement = &f.capitalID
					}
					if _, err := pool.Exec(ctx, `UPDATE units SET crew=2,status=$2,q=$3,r=0,settlement_id=$4 WHERE id=$1`, ship, status, q, settlement); err != nil {
						t.Fatal(err)
					}
					if place == "marching" {
						route := StoredRoute{StartTick: 0, EndTick: 3, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}}, Costs: []int64{1000, 1000, 1000}}
						raw, _ := json.Marshal(route)
						if _, err := pool.Exec(ctx, `UPDATE units SET march_route=$2,depart_tick=0,arrive_tick=3,target_q=3,target_r=0 WHERE id=$1`, ship, raw); err != nil {
							t.Fatal(err)
						}
					}
					if place == "freighting" {
						raw := []byte(`{"category":"naval","travel_ticks":3,"distance":3,"path":[{"q":0,"r":0},{"q":1,"r":0},{"q":2,"r":0},{"q":3,"r":0}],"step_costs":[667,667,667]}`)
						if _, err := pool.Exec(ctx, `INSERT INTO transports(world_id,owner_id,kind,origin_id,dest_id,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,departed_tick,ship_unit_id,journey) VALUES($1,$2,'gift',$3,$3,'naval',0,0,3,0,now(),now(),3,0,$4,$5)`, f.worldID, f.ownerID, f.capitalID, ship, raw); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=3 WHERE id=$1`, f.worldID); err != nil {
						t.Fatal(err)
					}
					var runner uuid.UUID
					if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,kind,status,hex_q,hex_r,arrives_at,passage_status,carrier_unit_id) VALUES($1,$2,$3,$3,'sealed upkeep letter','message','outbound',0,0,now(),'aboard',$4) RETURNING id`, f.worldID, f.ownerID, f.capitalID, ship).Scan(&runner); err != nil {
						t.Fatal(err)
					}
					if rescued {
						oldShip := mkStarvingShip(t, pool, f.worldID, f.ownerID, f.capitalID, f.capitalID)
						if _, err := pool.Exec(ctx, `UPDATE messengers SET carrier_unit_id=$2 WHERE id=$1`, runner, oldShip); err != nil {
							t.Fatal(err)
						}
						tx, err := pool.Begin(ctx)
						if err != nil {
							t.Fatal(err)
						}
						defer tx.Rollback(ctx)
						if _, err := carrier.OutcomeTx(ctx, tx, nil, f.worldID, oldShip, &ship, "battle", 3, 0, 2); err != nil {
							t.Fatal(err)
						}
						if err := tx.Commit(ctx); err != nil {
							t.Fatal(err)
						}
					}
					h := newStarvationUpkeepHandler(pool, nil)
					u := upkeepUnitRow{id: ship, ownerID: f.ownerID, unitType: "galley", category: "naval", size: 1, crew: 2, status: status, settlementID: settlement, q: &q, r: &r, unpaidPeriods: upkeepDesertionTicks - 1}
					if cause == "grain" {
						if !h.applyAttrition(ctx, u, 0, f.worldID, uuid.Nil) {
							t.Fatal("fixture must disband crewless ship")
						}
					} else {
						h.recordUnpaid(ctx, u, f.worldID, 0, uuid.Nil, 1)
					}
					var dead bool
					if err := pool.QueryRow(ctx, `SELECT status='disbanded' FROM units WHERE id=$1`, ship).Scan(&dead); err != nil || !dead {
						t.Fatalf("fixture must kill carrier: %v %v", dead, err)
					}
					expected := carrier.Lost
					if place == "port" || place == "adjacent" {
						expected = carrier.Landed
					}
					var raw []byte
					if err := pool.QueryRow(ctx, `SELECT payload FROM events WHERE stream_id=$1 AND event_type=$2`, runner, expected).Scan(&raw); err != nil {
						t.Fatalf("upkeep %s/%s must write %s in ship death TX: %v", cause, place, expected, err)
					}
					var w carrier.Witness
					if err := json.Unmarshal(raw, &w); err != nil {
						t.Fatal(err)
					}
					if expected == carrier.Landed && (w.Q != 0 || w.R != 0) {
						t.Fatalf("actual settlement landing: %s", raw)
					}
					if expected == carrier.Lost && w.Q != 3 {
						t.Fatalf("death must use actual route position, not frozen departure/settlement_id: %s", raw)
					}
				})
			}
		}
	}
}
