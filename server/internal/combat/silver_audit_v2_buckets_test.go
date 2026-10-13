package combat

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
)

func TestSilverAuditV2AllBucketsAndDeferredHandoff(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	world, mother, pouch := purseFixture(t, pool, ctx, 10, 30)
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM units WHERE id=$1`, pouch).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO founder_phase(world_id,owner_id,population,grain_amount,grain_rate,silver_amount,silver_rate) VALUES($1,$2,1000,1200,-1,20,0)`, world, owner)
	exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap) VALUES($1,'grain',0,0,1000)`, mother)
	exec(`INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,0,0,'plains')`, world)
	for _, offer := range []string{`{"status":"pending","kind":"buy","offer_silver":40}`, `{"status":"pending","kind":"sell","offer_good":"silver","offer_qty":50}`, `{"status":"accepted","kind":"buy","offer_silver":999}`} {
		exec(`INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,hex_q,hex_r,arrives_at,trade_offer) VALUES($1,$2,$3,$3,'offer',0,0,now(),$4::jsonb)`, world, owner, mother, offer)
	}
	clk := clock.NewTestClock(time.Now())
	store := events.NewStore(pool)
	sched := events.NewScheduler(pool, clk)
	makeLeg := func(good string, qty float64) uuid.UUID {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		journey := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0, R: 0}}, TravelTicks: 1}
		id, err := transport.CreateShadow(ctx, tx, transport.DispatchParams{WorldID: world, OwnerID: owner, OriginID: mother, DestID: mother, Kind: "trade", Category: "land", DepartsAt: clk.Now(), ArrivesAt: clk.Now().Add(time.Hour), DueTick: 1, Manifest: transport.Manifest{good: qty}, Journey: &journey})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return id
	}
	makeLeg("silver", 60)
	dead := makeLeg("silver", 999)
	exec(`UPDATE transports SET status='delivered' WHERE id=$1`, dead)
	parent := makeLeg("grain", 5)
	journey := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0, R: 0}}, TravelTicks: 1}
	payload := map[string]any{"destination_id": mother, "transport_id": parent, "good_key": "grain", "quantity": 5, "then_return": map[string]any{"destination_id": mother.String(), "good_key": "silver", "quantity": 70, "owner_id": owner.String(), "journey": journey, "travel_ticks": 1}}
	if err := sched.EnqueueTick(ctx, world, events.ScheduledTradeDelivery, payload, 1); err != nil {
		t.Fatal(err)
	}
	var tradeID int64
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT id,payload FROM scheduled_events WHERE world_id=$1 AND event_type='TradeDelivery' ORDER BY id DESC LIMIT 1`, world).Scan(&tradeID, &raw); err != nil {
		t.Fatal(err)
	}
	// Dead copies are not stock; a neutral active city is stock, attributed to nobody.
	exec(`INSERT INTO units(world_id,owner_id,type,category,size,status,carried_silver) VALUES($1,$2,'spearman','land',0,'disbanded',999)`, world, owner)
	var neutral uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type) SELECT $1,id,'Neutral','achaean',NULL,'neutral' FROM provinces WHERE world_id=$1 AND map_q=10 AND map_r=10 RETURNING id`, world).Scan(&neutral); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap) VALUES($1,'silver',8,0,100)`, neutral)
	h := NewUpkeepHandler(pool, sched, store, nil, nil)
	audit := func() SilverAuditV2Payload {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `SELECT nextval('scheduled_events_id_seq')`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if err := h.emitSilverAuditV2(ctx, events.ScheduledEvent{WorldID: world, ID: id}); err != nil {
			t.Fatal(err)
		}
		var raw []byte
		if err := pool.QueryRow(ctx, `SELECT payload FROM events WHERE world_id=$1 AND event_type='SilverAuditV2' ORDER BY id DESC LIMIT 1`, world).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var p SilverAuditV2Payload
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := audit()
	if p.TotalSilver != 288 || p.CitySilver != 18 || p.BuyEscrowSilver != 40 || p.SellEscrowSilver != 50 || p.FounderSilver != 20 || p.CarriedSilver != 30 || p.TransportSilver != 60 || p.DeferredTradeSilver != 70 || p.UnownedSilver != 8 || p.WanaxDistribution.TotalSilver != 280 {
		t.Fatalf("wrong stock %+v", p)
	}
	if err := economy.NewDeliveryHandler(pool, store, nil, sched).Handle(ctx, events.ScheduledEvent{ID: tradeID, WorldID: world, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	p = audit()
	if p.TotalSilver != 288 || p.DeferredTradeSilver != 0 || p.TransportSilver != 130 || p.NetDelta != 0 {
		t.Fatalf("return handoff double counted or lost silver: %+v", p)
	}
	// Worker has not marked TradeDelivery processed yet. Transport status, not
	// the later queue update, is the atomic marker for handing off reservation.
	var processed bool
	if err := pool.QueryRow(ctx, `SELECT processed_at IS NOT NULL FROM scheduled_events WHERE id=$1`, tradeID).Scan(&processed); err != nil {
		t.Fatal(err)
	}
	if processed {
		t.Fatal("fixture did not exercise commit-before-worker-mark interval")
	}
}

func TestSilverMiningCreditRollbackAndRateChange(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	world, mother, _ := purseFixture(t, pool, ctx, 90, 0)
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=10,cap=100 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=100,calc_tick=2 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var counter float64
	if err := pool.QueryRow(ctx, `SELECT (SELECT COALESCE(SUM(credited_silver),0) FROM silver_mining_totals WHERE world_id=$1)`, world).Scan(&counter); err != nil {
		t.Fatal(err)
	}
	if counter != 0 {
		t.Fatal("rolled-back mining was credited")
	}
	// A rate change settles the OLD rate; future ticks then use the new one.
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=100,rate=2,calc_tick=2 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=80 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=84,calc_tick=4 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(credited_silver),0) FROM silver_mining_totals WHERE world_id=$1`, world).Scan(&counter); err != nil {
		t.Fatal(err)
	}
	if math.Abs(counter-14) > 1e-8 {
		t.Fatalf("credited=%g want old clipped10+new4", counter)
	}
}

func TestSilverAuditV2AppendFailureCanRetry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	world, mother, _ := purseFixture(t, pool, ctx, 90, 0)
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET rate=10,cap=100 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=1 WHERE id=$1`, world); err != nil {
		t.Fatal(err)
	}
	// Fail at the LAST write, after the audit would have realized production.
	constraint := `ALTER TABLE events ADD CONSTRAINT silver_audit_v2_test_reject CHECK (world_id<>'` + world.String() + `'::uuid OR event_type<>'SilverAuditV2')`
	if _, err := pool.Exec(ctx, constraint); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `ALTER TABLE events DROP CONSTRAINT IF EXISTS silver_audit_v2_test_reject`)
	})
	clk := clock.NewTestClock(time.Now())
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, nil)
	var eventID int64
	if err := pool.QueryRow(ctx, `SELECT nextval('scheduled_events_id_seq')`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	e := events.ScheduledEvent{ID: eventID, WorldID: world}
	if err := h.emitSilverAuditV2(ctx, e); err == nil {
		t.Fatal("forced append failure accepted")
	}
	var amount, credited float64
	var calc, claims int
	if err := pool.QueryRow(ctx, `SELECT amount,calc_tick FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, mother).Scan(&amount, &calc); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT (SELECT COALESCE(SUM(credited_silver),0) FROM silver_mining_totals WHERE world_id=$1)`, world).Scan(&credited); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_tick_claims WHERE event_id=$1 AND scope_id=$2`, eventID, uuid.NewSHA1(world, []byte("silver_audit_v2"))).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if amount != 90 || calc != 0 || credited != 0 || claims != 0 {
		t.Fatalf("failed audit committed partial state: %g/%d credit%g claims%d", amount, calc, credited, claims)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE events DROP CONSTRAINT silver_audit_v2_test_reject`); err != nil {
		t.Fatal(err)
	}
	if err := h.emitSilverAuditV2(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := h.emitSilverAuditV2(ctx, e); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='SilverAuditV2'`, world).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(credited_silver),0) FROM silver_mining_totals WHERE world_id=$1`, world).Scan(&credited); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || credited != 10 {
		t.Fatalf("retry duplicate/lost audit: audits%d credit%g", audits, credited)
	}
}
