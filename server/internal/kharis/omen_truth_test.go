package kharis

import (
 "context"
 "testing"
 "formatet/megaron/server/internal/economy"
 "formatet/megaron/server/internal/events"
)

func TestOmenNoFactsMeansSilence(t *testing.T){
 pool,wid,sid:=newGrowthFixture(t,[6]string{"plains","plains","plains","plains","plains","plains"},1000);ctx:=context.Background()
 if _,err:=pool.Exec(ctx,`UPDATE settlement_goods SET rate=CASE WHEN good_key='grain' THEN $2 ELSE 0 END WHERE settlement_id=$1`,sid,economy.GrainConsumptionPerTick(1000));err!=nil{t.Fatal(err)}
 if _,err:=pool.Exec(ctx,`INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_cap,kharis_calc_tick) SELECT owner_id,world_id,40,100,0 FROM settlements WHERE id=$1 ON CONFLICT DO NOTHING`,sid);err!=nil{t.Fatal(err)}
 h:=NewTickHandler(pool,events.NewScheduler(pool,nil),events.NewStore(pool),nil)
 h.generateOmen(ctx,sid,wid)
 var n int;if err:=pool.QueryRow(ctx,`SELECT count(*) FROM gossip_events WHERE world_id=$1 AND category='omen'`,wid).Scan(&n);err!=nil{t.Fatal(err)}
 if n!=0{t.Fatalf("no true omen candidate, must remain silent; got %d invented omens",n)}
}
