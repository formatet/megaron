package combat

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"testing"
	"time"
)

func storageCity(t *testing.T) (*pgxpool.Pool, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE worlds SET status='archived' WHERE status='active'`)
	wid, owner, pid, sid := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO worlds(id,name,status,current_tick) VALUES($1,$2,'active',10)`, wid, "storage-"+wid.String())
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `UPDATE worlds SET status='archived' WHERE id=$1`, wid) })
	exec(`INSERT INTO players(id,username,password_hash,agora_exempt) VALUES($1,$2,'x',true)`, owner, "storage-"+owner.String())
	exec(`INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type) VALUES($1,$2,0,0,'plains')`, pid, wid)
	exec(`INSERT INTO settlements(id,world_id,province_id,owner_id,name,culture_id,control_type,is_capital,state,population) VALUES($1,$2,$3,$4,'MalliaWest','achaean','colony',false,'active',1000)`, sid, wid, pid, owner)
	exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',0,0,60,10)`, sid)
	return pool, wid, sid, owner
}

func TestStorageKnownIncomingWarningDebounceAndActualDelivery(t *testing.T) {
	pool, wid, sid, owner := storageCity(t)
	ctx := context.Background()
	store := events.NewStore(pool)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	incoming := uuid.New()
	exec(`INSERT INTO transports(id,world_id,owner_id,kind,dest_id,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,status,interceptable) VALUES($1,$2,$3,'internal_transfer',$4,'land',0,0,0,0,now(),now(),30,'in_transit',true)`, incoming, wid, owner, sid)
	exec(`INSERT INTO transport_goods(transport_id,good_key,quantity) VALUES($1,'silver',150)`, incoming)
	// Unknown gifts must not reveal the sender's transport or cargo before arrival.
	stranger := uuid.New()
	gift := uuid.New()
	exec(`INSERT INTO players(id,username,password_hash,agora_exempt) VALUES($1,$2,'x',true)`, stranger, "stranger-"+stranger.String())
	exec(`INSERT INTO transports(id,world_id,owner_id,kind,dest_id,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,status,interceptable) VALUES($1,$2,$3,'gift',$4,'land',0,0,0,0,now(),now(),11,'in_transit',true)`, gift, wid, stranger, sid)
	exec(`INSERT INTO transport_goods(transport_id,good_key,quantity) VALUES($1,'silver',1000)`, gift)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	forecast, err := (StorageForecaster{}).StorageForecast(ctx, tx, sid)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(forecast) != 1 || forecast[0].Incoming != 150 || forecast[0].FullInTicks == nil || *forecast[0].FullInTicks != 20 || !forecast[0].Warning {
		t.Fatalf("known receipts forecast=%+v", forecast)
	}
	run := func(tick int) {
		t.Helper()
		exec(`UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick)
		// Reconstruct the observer every tick, then retry the same event: both must
		// use the durable transition context, including the four-clear-tick relapse.
		h := kharis.NewTickHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))), store, nil)
		h.SetStorageForecaster(StorageForecaster{})
		e := events.ScheduledEvent{ID: int64(299000 + tick), WorldID: wid, DueTick: tick}
		if err := h.Handle(ctx, e); err != nil {
			t.Fatal(err)
		}
		if err := h.Handle(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int) {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='StorageCapacityWarning'`, owner).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("storage warnings=%d want%d", n, want)
		}
	}
	for tick := 10; tick < 20; tick++ {
		run(tick)
	}
	count(1)
	exec(`UPDATE transports SET status='lost' WHERE id=$1`, incoming)
	for tick := 20; tick < 24; tick++ {
		run(tick)
	}
	exec(`UPDATE transports SET status='in_transit' WHERE id=$1`, incoming)
	run(24)
	count(1)
	exec(`UPDATE transports SET status='lost' WHERE id=$1`, incoming)
	for tick := 25; tick <= 29; tick++ {
		run(tick)
	}
	exec(`UPDATE transports SET status='in_transit',due_tick=50 WHERE id=$1`, incoming)
	run(30)
	count(2)
	// An already observed period is not recreated when the inbox is archived.
	exec(`DELETE FROM notifications WHERE player_id=$1 AND kind='StorageCapacityWarning'`, owner)
	run(30)
	count(0)
	raw, _ := json.Marshal(map[string]any{"transport_id": incoming})
	arrival := transport.NewArrivalHandler(pool, nil, store)
	e := events.ScheduledEvent{ID: 299100, WorldID: wid, DueTick: 30, Payload: raw}
	for i := 0; i < 2; i++ {
		if err := arrival.Handle(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var amount, lost float64
	var n int
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, sid).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(sum((body_json->>'lost_quantity')::float8),0) FROM notifications WHERE player_id=$1 AND kind='StorageOverflow'`, owner).Scan(&n, &lost); err != nil {
		t.Fatal(err)
	}
	if amount != 60 || n != 1 || lost != 90 {
		t.Fatalf("actual delivery stock%g notices%d lost%g", amount, n, lost)
	}
}

func TestStorageForecastUsesCurrentMealAndPayerSold(t *testing.T) {
	pool, wid, sid, owner := storageCity(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE settlement_goods SET rate=2 WHERE settlement_id=$1`, sid)
	exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',100,1,1000,10),($1,'fish',900,9,1000,10)`, sid)
	exec(`INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id) VALUES($1,$2,'galley','naval',1,20,'garrison',$3,$3)`, wid, owner, sid)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	fs, err := (StorageForecaster{}).StorageForecast(ctx, tx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if f.GoodKey == "silver" {
			want := 2 - UnitUpkeep("galley", "naval", 1, "garrison", 20).Silver*(1-UpkeepSoldShare())
			if math.Abs(f.NetPerTick-want) > 1e-9 {
				t.Fatalf("net%g want%g", f.NetPerTick, want)
			}
		}
		if f.GoodKey == "grain" && math.Abs(f.NetPerTick-.49) > 1e-9 {
			t.Fatalf("grain net%g must include proportional civilian+ship food", f.NetPerTick)
		}
	}
}
