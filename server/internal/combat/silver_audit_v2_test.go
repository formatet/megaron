package combat

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
)

// Conservation is checked at every change of custody, not just at endpoints.
// Financial genesis/escrow are explicit fixtures; payroll, physical dispatch,
// trade delivery, transfer delivery and colony use the real domain handlers.
func TestSilverAuditV2Conservation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	world, mother, colonist := purseFixture(t, pool, ctx, 0, 0)
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM units WHERE id=$1`, colonist).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE units SET status='forming' WHERE id=$1`, colonist)
	exec(`INSERT INTO player_world_records(player_id,world_id) VALUES($1,$2)`, owner, world)
	var zeroOwner uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&zeroOwner); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO player_world_records(player_id,world_id,status) VALUES($1,$2,'collapsed')`, zeroOwner, world)
	clk := clock.NewTestClock(time.Now())
	store := events.NewStore(pool)
	sched := events.NewScheduler(pool, clk)
	h := NewUpkeepHandler(pool, sched, store, nil, nil)
	h.soldShare = .7
	legacy, err := store.Append(ctx, world, events.StreamWorld, "SilverAudit", SilverAuditPayload{LiquidTotal: 999, FundTotal: 123, EscrowTotal: 7}, world, nil)
	if err != nil {
		t.Fatal(err)
	}
	nextID := func() int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `SELECT nextval('scheduled_events_id_seq')`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	read := func() SilverAuditV2Payload {
		t.Helper()
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
	audit := func() SilverAuditV2Payload {
		t.Helper()
		e := events.ScheduledEvent{WorldID: world, ID: nextID()}
		if err := h.emitSilverAuditV2(ctx, e); err != nil {
			t.Fatal(err)
		}
		p := read()
		if err := h.emitSilverAuditV2(ctx, e); err != nil {
			t.Fatal(err)
		}
		if got := read(); got.MiningCreditedTotal != p.MiningCreditedTotal || got.TotalSilver != p.TotalSilver {
			t.Fatal("replayed audit changed stock")
		}
		return p
	}
	near := func(got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 1e-8 {
			t.Fatalf("got %.9f, want %.9f", got, want)
		}
	}
	p := audit()
	near(p.TotalSilver, 0)
	near(p.NetDelta, 0)
	if p.PreviousAuditTick != nil {
		t.Fatal("V1 baseline used for V2")
	}
	// First newcomer purse, followed by its fixed metropolis endowment.
	exec(`INSERT INTO founder_phase(world_id,owner_id,population,grain_amount,grain_rate,silver_amount,silver_rate) VALUES($1,$2,1000,1200,-1,480,0)`, world, owner)
	p = audit()
	near(p.TotalSilver, 480)
	near(p.FounderSilver, 480)
	near(p.NetDelta, 480)
	amount, cap := economy.MetropolisSilver(false, 480, false)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE founder_phase SET active=false WHERE world_id=$1 AND owner_id=$2`, world, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=$2,cap=$3 WHERE settlement_id=$1 AND good_key='silver'`, mother, amount, cap); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	p = audit()
	near(p.TotalSilver, 730)
	near(p.NetDelta, 250)
	near(p.FounderSilver, 0)
	// Cap, partial fill and changing rates: full warehouses don't mine 10/tick.
	exec(`UPDATE settlement_goods SET rate=10,cap=735 WHERE settlement_id=$1 AND good_key='silver'`, mother)
	exec(`UPDATE worlds SET current_tick=1 WHERE id=$1`, world)
	p = audit()
	near(p.TotalSilver, 735)
	near(p.MinedSinceLast, 5)
	near(p.NetDelta, 5)
	exec(`UPDATE worlds SET current_tick=2 WHERE id=$1`, world)
	p = audit()
	near(p.TotalSilver, 735)
	near(p.MinedSinceLast, 0)
	exec(`UPDATE settlement_goods SET rate=0 WHERE settlement_id=$1 AND good_key='silver'`, mother)
	// Actual field payroll destroys 2 silver, paid from a fed city.
	exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',1000,0,1000,2)`, mother)
	exec(`UPDATE units SET type='elite_infantry',status='marching',size=100,support_settlement_id=$2 WHERE id=$1`, colonist, mother)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: world, ID: nextID(), DueTick: 2}); err != nil {
		t.Fatal(err)
	}
	p = read()
	near(p.TotalSilver, 733)
	near(p.NetDelta, -2)
	near(p.MinedSinceLast, 0)
	exec(`UPDATE units SET type='spearman',size=50,status='forming' WHERE id=$1`, colonist)
	exec(`UPDATE settlement_goods SET rate=10 WHERE settlement_id=$1 AND good_key='silver'`, mother)
	exec(`UPDATE worlds SET current_tick=3 WHERE id=$1`, world)
	p = audit()
	near(p.TotalSilver, 735)
	near(p.MinedSinceLast, 2)
	exec(`UPDATE settlement_goods SET rate=0 WHERE settlement_id=$1 AND good_key='silver'`, mother)
	// Trade reserve → physical first leg → delivered, with no minting.
	var offer uuid.UUID
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=amount-50 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,hex_q,hex_r,arrives_at,trade_offer) VALUES($1,$2,$3,$3,'trade',0,0,now(),'{"status":"pending","kind":"buy","offer_silver":50}') RETURNING id`, world, owner, mother).Scan(&offer); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	p = audit()
	near(p.TotalSilver, 735)
	near(p.BuyEscrowSilver, 50)
	near(p.NetDelta, 0)
	dispatch := func(kind string, qty float64, escrow bool) uuid.UUID {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if escrow {
			_, err = tx.Exec(ctx, `UPDATE messengers SET trade_offer=trade_offer||'{"status":"accepted"}' WHERE id=$1`, offer)
		} else {
			_, err = tx.Exec(ctx, `UPDATE settlement_goods SET amount=amount-$2 WHERE settlement_id=$1 AND good_key='silver'`, mother, qty)
		}
		if err != nil {
			t.Fatal(err)
		}
		journey := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0, R: 0}}, TravelTicks: 1}
		id, err := transport.Dispatch(ctx, tx, sched, transport.DispatchParams{WorldID: world, OwnerID: owner, OriginID: mother, DestID: mother, Kind: kind, Category: "land", DepartsAt: clk.Now(), ArrivesAt: clk.Now().Add(time.Hour), DepartedTick: 3, DueTick: 4, Manifest: transport.Manifest{"silver": qty}, Journey: &journey})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return id
	}
	leg := dispatch("trade", 50, true)
	p = audit()
	near(p.TotalSilver, 735)
	near(p.TransportSilver, 50)
	near(p.BuyEscrowSilver, 0)
	payload, _ := json.Marshal(map[string]any{"destination_id": mother, "transport_id": leg, "good_key": "silver", "quantity": 50})
	if err := economy.NewDeliveryHandler(pool, store, nil, sched).Handle(ctx, events.ScheduledEvent{ID: nextID(), WorldID: world, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	p = audit()
	near(p.TotalSilver, 735)
	near(p.TransportSilver, 0)
	near(p.NetDelta, 0)
	leg = dispatch("transfer", 25, false)
	p = audit()
	near(p.TotalSilver, 735)
	near(p.TransportSilver, 25)
	payload, _ = json.Marshal(map[string]any{"transport_id": leg})
	if err := transport.NewArrivalHandler(pool, nil, store).Handle(ctx, events.ScheduledEvent{ID: nextID(), WorldID: world, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	p = audit()
	near(p.TotalSilver, 735)
	near(p.TransportSilver, 0)
	// The unit's money remains real when carried; colony creation moves it.
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=amount-15 WHERE settlement_id=$1 AND good_key='silver'`, mother); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE units SET status='marching',carried_silver=15 WHERE id=$1`, colonist); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	p = audit()
	near(p.TotalSilver, 735)
	near(p.CarriedSilver, 15)
	near(p.NetDelta, 0)
	runColonizeArrival(t, pool, ctx, world, colonist)
	p = audit()
	near(p.TotalSilver, 735)
	near(p.CarriedSilver, 0)
	near(p.NetDelta, 0)
	near(p.WanaxDistribution.TotalSilver, 735)
	near(p.WanaxDistribution.MedianSilver, 367.5)
	near(p.WanaxDistribution.MaxSilver, 735)
	near(p.WanaxDistribution.ZeroShare, .5)
	if p.WanaxDistribution.Count != 2 || p.WanaxDistribution.ZeroCount != 1 {
		t.Fatal(p.WanaxDistribution)
	}
	var old []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM events WHERE id=$1`, legacy.ID).Scan(&old); err != nil {
		t.Fatal(err)
	}
	var historical SilverAuditPayload
	if err := json.Unmarshal(old, &historical); err != nil {
		t.Fatal(err)
	}
	near(historical.LiquidTotal, 999)
	near(historical.FundTotal, 123)
	// Down/up leaves wallet tuples and old payloads intact, and opens a new
	// mining epoch rather than deriving negative credits from the old counter.
	down, err := os.ReadFile("../../db/migrations/179_silver_mining_credits.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("../../db/migrations/179_silver_mining_credits.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	reset := audit()
	near(reset.TotalSilver, 735)
	near(reset.MinedSinceLast, 0)
	near(reset.NetDelta, 0)
	if reset.MiningEpoch == p.MiningEpoch || reset.PreviousAuditTick != nil {
		t.Fatal("down/up retained invalid mining watermark")
	}
	raw, _ := json.Marshal(reset)
	t.Log("AUDIT_V2_JSON=" + string(raw))
}
