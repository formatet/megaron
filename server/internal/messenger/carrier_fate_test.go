package messenger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"flag"
	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"os"
	"strings"
)

var carrierFixtureDir = flag.String("t2-fixture-dir", "", "export actual notification payloads for four-surface proof")

type carrierStormDice struct{}

func (carrierStormDice) Float64() float64 { return 0 }
func (carrierStormDice) Intn(int) int     { return 0 }

func TestCarrierFate_StormTransportLosesPassenger(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	ship := f.ship(t, f.originID, "galley")
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='freighting', hull=1 WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	transportID := f.departingTransport(t, f.originID, ship, 505)
	journey := province.TradeJourney{Category: "naval", TravelTicks: 5, Distance: 5}
	for q := 0; q <= 5; q++ {
		journey.Path = append(journey.Path, province.MapPosition{Q: q, R: 0})
		if q > 0 {
			journey.StepCosts = append(journey.StepCosts, 667)
		}
	}
	if err := journey.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(journey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET journey=$2, departed_tick=500 WHERE id=$1`, transportID, raw); err != nil {
		t.Fatal(err)
	}
	messengerID := f.waitingMessenger(t, f.originID, 500)
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(f.pool, clk)
	scan := NewPassageScanHandler(f.pool, sched, nil, clk, nil)
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 500}); err != nil {
		t.Fatal(err)
	}
	var boarded string
	if err := f.pool.QueryRow(ctx, `SELECT passage_status FROM messengers WHERE id=$1 AND carrier_transport_id=$2 AND carrier_unit_id IS NULL`, messengerID, transportID).Scan(&boarded); err != nil || boarded != "aboard" {
		t.Fatalf("real transport boarding: status=%q err=%v", boarded, err)
	}
	stale := f.loadScheduledEvent(t, string(events.ScheduledMessengerArrival), messengerID)
	storm := combat.NewSeaStormScanHandler(f.pool, sched, events.NewStore(f.pool), nil)
	storm.Dice = carrierStormDice{}
	f.setTick(t, 501)
	if err := storm.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	var carrierStatus string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, transportID).Scan(&carrierStatus); err != nil || carrierStatus != "foundered" {
		t.Fatalf("forced storm must founder transport: status=%q err=%v", carrierStatus, err)
	}
	// The old timer fires BEFORE the passage projection. Pending witness
	// must block delivery; changing the live row must not change the notice.
	if err := NewArrivalHandler(f.pool, sched, events.NewStore(f.pool), nil).Handle(ctx, stale); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, messengerID).Scan(&before); err != nil || before != "outbound" {
		t.Fatalf("pending loss must stop old timer: %s %v", before, err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET message_text='changed AFTER loss' WHERE id=$1`, messengerID); err != nil {
		t.Fatal(err)
	}
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	status, passage, _, _ := f.messengerRow(t, messengerID)
	if status != "lost" || passage != nil {
		t.Fatalf("storm passenger must be lost with no passage remaining: status=%q passage=%v", status, passage)
	}
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	var count int
	var body []byte
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='MessengerLostAtSea' AND body_json->>'messenger_id'=$2`, f.ownerID, messengerID.String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("double scan must archive one loss: count=%d err=%v", count, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT body_json FROM notifications WHERE kind='MessengerLostAtSea' AND body_json->>'messenger_id'=$1`, messengerID.String()).Scan(&body); err != nil {
		t.Fatal(err)
	}
	var privateCheck map[string]json.RawMessage
	if err := json.Unmarshal(body, &privateCheck); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ship_id", "rescue_ship_id", "q", "r", "previous_witness_id"} {
		if _, ok := privateCheck[key]; ok {
			t.Fatalf("instant loss carries other physical news %s: %s", key, body)
		}
	}
	var witness carrier.Witness
	if err := json.Unmarshal(body, &witness); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(witness.Envelope, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["message_text"] != "hello" || envelope["sent_at"] == nil || envelope["origin"] == nil || envelope["destination"] == nil || witness.SenderID != f.ownerID || witness.Reason != "storm" {
		t.Fatalf("loss must freeze WHOLE letter/endpoints/time and sender: %s", body)
	}
	if path := *carrierFixtureDir; path != "" {
		if err := os.WriteFile(path+"/messenger_lost_at_sea.json", body, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCarrierFate_StormMissionLosesPassenger(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	ship := f.marchingShip(t, 0, 0, 4, 0, 500, 504)
	route := combat.StoredRoute{StartTick: 500, EndTick: 504, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}, Costs: []int64{1000, 1000, 1000, 1000}}
	raw, _ := json.Marshal(route)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET hull=1, march_route=$2 WHERE id=$1`, ship, raw); err != nil {
		t.Fatal(err)
	}
	runner := f.waitingMessenger(t, f.originID, 500)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard', carrier_unit_id=$2 WHERE id=$1`, runner, ship); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Now())
	storm := combat.NewSeaStormScanHandler(f.pool, events.NewScheduler(f.pool, clk), events.NewStore(f.pool), nil)
	storm.Dice = carrierStormDice{}
	f.setTick(t, 501)
	if err := storm.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	status, passage, _, _ := f.messengerRow(t, runner)
	if status != "lost" || passage != nil {
		t.Fatalf("storm mission passenger must be lost, not sealed: status=%q passage=%v", status, passage)
	}
}

func TestCarrierFate_BattleRescuesPassenger(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	ship := f.ship(t, f.originID, "galley")
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='positioned', settlement_id=NULL,q=2,r=0 WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	runner := f.waitingMessenger(t, f.originID, 500)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',carrier_unit_id=$2 WHERE id=$1`, runner, ship); err != nil {
		t.Fatal(err)
	}
	var enemy, battle uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players (username,password_hash) VALUES ($1,'x') RETURNING id`, "rescue-"+uuid.NewString()).Scan(&enemy); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO battles(world_id,q,r,started_tick,current_tick,seed) VALUES($1,2,0,500,499,424243) RETURNING id`, f.worldID).Scan(&battle); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO battle_participants(battle_id,unit_id,owner_id,side,joined_tick,initial_size,current_size,standing_orders) VALUES($1,$2,$3,'attacker',500,1,1,'{"hold_to_last_man":true}')`, battle, ship, f.ownerID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		var rescuer uuid.UUID
		if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES($1,$2,'galley','naval',1,20,'positioned',2,0) RETURNING id`, f.worldID, enemy).Scan(&rescuer); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO battle_participants(battle_id,unit_id,owner_id,side,joined_tick,initial_size,current_size) VALUES($1,$2,$3,'defender',500,1,1)`, battle, rescuer, enemy); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Now())
	h := combat.NewBattleTickHandler(f.pool, events.NewStore(f.pool), events.NewScheduler(f.pool, clk), nil, clk)
	raw, _ := json.Marshal(map[string]any{"battle_id": battle})
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 500, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	var shipStatus string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, ship).Scan(&shipStatus); err != nil || shipStatus != "disbanded" {
		t.Fatalf("battle must sink carrier: status=%q err=%v", shipStatus, err)
	}
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 500}); err != nil {
		t.Fatal(err)
	}
	var rescued bool
	if err := f.pool.QueryRow(ctx, `SELECT m.passage_status='aboard' AND u.owner_id=$2 AND u.status!='disbanded' FROM messengers m LEFT JOIN units u ON u.id=m.carrier_unit_id WHERE m.id=$1`, runner, enemy).Scan(&rescued); err != nil || !rescued {
		t.Fatalf("battle passenger must be aboard a surviving enemy ship: rescued=%v err=%v", rescued, err)
	}
}

func TestCarrierFate_PendingRescueCanLandBeforeProjectionAndReportOnlyAtHome(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(f.pool, clk)
	store := events.NewStore(f.pool)
	carrierID := f.ship(t, f.originID, "galley")
	runner := f.waitingMessenger(t, f.originID, 500)
	var enemy uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "rescuer-"+uuid.NewString()).Scan(&enemy); err != nil {
		t.Fatal(err)
	}
	var rescuer uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,name) VALUES($1,$2,'galley','naval',1,20,'positioned',2,0,'Sacred Dolphin') RETURNING id`, f.worldID, enemy).Scan(&rescuer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',carrier_unit_id=$2 WHERE id=$1`, runner, carrierID); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE units SET status='disbanded',size=0 WHERE id=$1`, carrierID); err != nil {
		t.Fatal(err)
	}
	if _, err := carrier.OutcomeTx(ctx, tx, store, f.worldID, carrierID, &rescuer, "battle", 2, 0, 500); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// The rescuer reaches its OWN foreign port before a passage scan sees the
	// rescue. Its arrival transaction must still find this physical passenger.
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, enemy); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='marching',q=2,r=0,target_q=4,target_r=0,home_settlement_id=$2,march_intent='captured_return',depart_tick=500,arrive_tick=502 WHERE id=$1`, rescuer, f.destID); err != nil {
		t.Fatal(err)
	}
	f.setTick(t, 502)
	raw, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: rescuer, WorldID: f.worldID})
	arrivals := combat.NewUnitArrivalHandler(f.pool, store, nil, sched, clk, economy.SitosConfig{})
	if err := arrivals.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 502, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	// It sails again before messenger consumes the landing. The immutable
	// port witness, not the ship's current garrison, decides where we leave.
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='marching',settlement_id=NULL,target_q=1,target_r=0,march_intent='explore',name='renamed AFTER rescue' WHERE id=$1`, rescuer); err != nil {
		t.Fatal(err)
	}
	scan := NewPassageScanHandler(f.pool, sched, nil, clk, nil)
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 502}); err != nil {
		t.Fatal(err)
	}
	var physical *uuid.UUID
	var dq, dr *int
	if err := f.pool.QueryRow(ctx, `SELECT carrier_unit_id,disembark_q,disembark_r FROM messengers WHERE id=$1`, runner).Scan(&physical, &dq, &dr); err != nil || physical != nil || dq == nil || *dq != 5 || dr == nil || *dr != 0 {
		t.Fatalf("must land at actual next port after ship has left: carrier=%v q=%v r=%v err=%v", physical, dq, dr, err)
	}
	assertNoRescueNotice(t, f, runner)
	outbound := f.loadScheduledEvent(t, string(events.ScheduledMessengerArrival), runner)
	f.setTick(t, outbound.DueTick)
	if err := NewArrivalHandler(f.pool, sched, store, nil).Handle(ctx, outbound); err != nil {
		t.Fatal(err)
	}
	reply := "I received the sealed letter."
	result, err := StartReturnLeg(ctx, f.pool, sched, f.worldID, runner, clk.Now(), f.currentTick, &reply)
	if err != nil || !result.Started || !result.PassageAwaiting {
		t.Fatalf("real reply must need physical return passage: %+v %v", result, err)
	}
	assertNoRescueNotice(t, f, runner)
	homeShip := f.ship(t, f.destID, "galley")
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='freighting' WHERE id=$1`, homeShip); err != nil {
		t.Fatal(err)
	}
	homeLeg := f.departingTransportTo(t, f.destID, f.originID, 0, 0, homeShip, f.currentTick+5)
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatal(err)
	}
	f.setTick(t, f.currentTick+5)
	legRaw, _ := json.Marshal(map[string]any{"transport_id": homeLeg})
	if err := transport.NewArrivalHandler(f.pool, nil).Handle(ctx, events.ScheduledEvent{ID: 919191, WorldID: f.worldID, DueTick: f.currentTick, Payload: legRaw}); err != nil {
		t.Fatal(err)
	}
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatal(err)
	}
	assertNoRescueNotice(t, f, runner)
	returning := f.loadScheduledEvent(t, string(events.ScheduledMessengerReturn), runner)
	h := NewReturnHandler(f.pool, store, nil)
	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, returning); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var report []byte
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE kind='MessengerRescuedAtSea' AND body_json->>'messenger_id'=$1`, runner.String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("home return report exactly once: %d %v", count, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT body_json FROM notifications WHERE kind='MessengerRescuedAtSea' AND body_json->>'messenger_id'=$1`, runner.String()).Scan(&report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), "Sacred Dolphin") || strings.Contains(string(report), "renamed AFTER") {
		t.Fatalf("report must contain frozen rescue and port: %s", report)
	}
	if path := *carrierFixtureDir; path != "" {
		if err := os.WriteFile(path+"/messenger_rescued_at_sea.json", report, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertNoRescueNotice(t *testing.T, f *passageFixture, runner uuid.UUID) {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE kind='MessengerRescuedAtSea' AND player_id=$1 AND body_json->>'messenger_id'=$2`, f.ownerID, runner.String()).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no rescue news before physical home return: %d %v", n, err)
	}
}

func TestCarrierFate_LossFreezesTradeAndUnitOrder(t *testing.T) {
	for _, kind := range []string{"trade", "order"} {
		t.Run(kind, func(t *testing.T) {
			f := setupPassageFixture(t)
			ctx := context.Background()
			clk := clock.NewTestClock(time.Now())
			sched := events.NewScheduler(f.pool, clk)
			ship := f.marchingShip(t, 0, 0, 4, 0, 500, 504)
			route := combat.StoredRoute{StartTick: 500, EndTick: 504, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}, Costs: []int64{1000, 1000, 1000, 1000}}
			raw, _ := json.Marshal(route)
			if _, err := f.pool.Exec(ctx, `UPDATE units SET hull=1,march_route=$2 WHERE id=$1`, ship, raw); err != nil {
				t.Fatal(err)
			}
			runner := f.waitingMessenger(t, f.originID, 500)
			var offer, order []byte
			rowKind := "message"
			if kind == "trade" {
				offer = []byte(`{"kind":"sell","status":"pending","offer_good":"bronze","offer_qty":31,"want_silver":145.7}`)
			} else {
				rowKind = "order"
				order = []byte(`{"verb":"march","unit_id":"00000000-0000-0000-0000-000000000009","q":9,"r":4,"standing_orders":{"hold_to_last_man":true}}`)
			}
			letter := "<script>untrusted text</script>\n" + strings.Repeat("whole sealed contents ", 150)
			if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',carrier_unit_id=$2,kind=$3,trade_offer=$4,order_payload=$5,message_text=$6 WHERE id=$1`, runner, ship, rowKind, offer, order, letter); err != nil {
				t.Fatal(err)
			}
			storm := combat.NewSeaStormScanHandler(f.pool, sched, events.NewStore(f.pool), nil)
			storm.Dice = carrierStormDice{}
			f.setTick(t, 501)
			if err := storm.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `UPDATE messengers SET message_text='AFTER',trade_offer=NULL,order_payload=NULL WHERE id=$1`, runner); err != nil {
				t.Fatal(err)
			}
			if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
				t.Fatal(err)
			}
			var body []byte
			if err := f.pool.QueryRow(ctx, `SELECT body_json FROM notifications WHERE kind='MessengerLostAtSea' AND body_json->>'messenger_id'=$1`, runner.String()).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var w carrier.Witness
			if err := json.Unmarshal(body, &w); err != nil {
				t.Fatal(err)
			}
			var e struct {
				Message      string          `json:"message_text"`
				Offer, Order json.RawMessage `json:"-"`
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(w.Envelope, &envelope); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(envelope["message_text"], &e.Message); err != nil || e.Message != letter {
				t.Fatalf("whole letter frozen at loss: len=%d %v", len(e.Message), err)
			}
			key := "trade_offer"
			expected := offer
			if kind == "order" {
				key = "order_payload"
				expected = order
			}
			var actual, want any
			if err := json.Unmarshal(envelope[key], &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(expected, &want); err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(want)
			if string(a) != string(b) {
				t.Fatalf("all %s terms frozen: %s want %s", kind, a, b)
			}
			if *carrierFixtureDir != "" {
				if err := os.WriteFile(*carrierFixtureDir+"/messenger_lost_"+kind+".json", body, 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestCarrierFate_PendingRescueCanBeLostAgainAndAtomicWitnessFailure(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(f.pool, clk)
	store := events.NewStore(f.pool)
	original := f.ship(t, f.originID, "galley")
	rescuer := f.marchingShip(t, 0, 0, 4, 0, 500, 504)
	runner := f.waitingMessenger(t, f.originID, 500)
	route := combat.StoredRoute{StartTick: 500, EndTick: 504, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}, Costs: []int64{1000, 1000, 1000, 1000}}
	raw, _ := json.Marshal(route)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET hull=1,march_route=$2 WHERE id=$1`, rescuer, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',carrier_unit_id=$2 WHERE id=$1`, runner, original); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := carrier.OutcomeTx(ctx, tx, store, f.worldID, original, &rescuer, "battle", 0, 0, 500); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// A database failure specifically at the new witness insert must roll back
	// ship death, storm progress and all companion changes.
	function := "reject_carrier_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sql := `CREATE FUNCTION ` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='CarrierPassengerLostV1' AND NEW.world_id='` + f.worldID.String() + `' THEN RAISE EXCEPTION 'T2 witness failure'; END IF; RETURN NEW; END $$`
	if _, err := f.pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `CREATE TRIGGER `+function+` BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION `+function+`() `); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+function+` ON events`)
		_, _ = f.pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS `+function+`()`)
	})
	storm := combat.NewSeaStormScanHandler(f.pool, sched, store, nil)
	storm.Dice = carrierStormDice{}
	f.setTick(t, 501)
	if err := storm.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	var status string
	var hull int
	if err := f.pool.QueryRow(ctx, `SELECT status,hull FROM units WHERE id=$1`, rescuer).Scan(&status, &hull); err != nil || status != "marching" || hull != 1 {
		t.Fatalf("failed witness must roll back ship death: %s hull=%d %v", status, hull, err)
	}
	if _, err := f.pool.Exec(ctx, `DROP TRIGGER `+function+` ON events`); err != nil {
		t.Fatal(err)
	}
	if err := storm.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	status, passage, _, _ := f.messengerRow(t, runner)
	if status != "lost" || passage != nil {
		t.Fatalf("physical rescue followed by storm BEFORE projection must lose runner: %s %v", status, passage)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='CarrierPassengerLostV1'`, runner).Scan(&n); err != nil || n != 1 {
		t.Fatalf("atomic retry exactly one loss: %d %v", n, err)
	}
}

func TestCarrierFate_StaleBoardingCannotBoardDeadCarrier(t *testing.T) {
	for _, kind := range []string{"mission", "transport"} {
		t.Run(kind, func(t *testing.T) {
			f := setupPassageFixture(t)
			ctx := context.Background()
			ship := f.marchingShip(t, 0, 0, 4, 0, 500, 504)
			runner := f.waitingMessenger(t, f.originID, 500)
			c := boardedCarrier{unitID: &ship, name: "Stale ship"}
			if kind == "transport" {
				id := f.departingTransport(t, f.originID, ship, 504)
				c = boardedCarrier{transportID: &id, name: "Stale transport"}
				if _, err := f.pool.Exec(ctx, `UPDATE transports SET status='foundered' WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.pool.Exec(ctx, `UPDATE units SET status='disbanded',size=0 WHERE id=$1`, ship); err != nil {
				t.Fatal(err)
			}
			if err := f.handler().boardOne(ctx, f.worldID, runner, province.MapPosition{Q: 5, R: 0}, 5, 0, 504, time.Now().Add(time.Hour), c); err != nil {
				t.Fatal(err)
			}
			row := f.fullRow(t, runner)
			if row.passageStatus == nil || *row.passageStatus != "awaiting_passage" {
				t.Fatalf("carrier selected before death must not board after death: %+v", row)
			}
		})
	}
}

func TestCarrierFate_RescuedWaitingDoesNotSendRemotePassageNews(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	store := events.NewStore(f.pool)
	var enemy uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "remote-"+uuid.NewString()).Scan(&enemy); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, enemy); err != nil {
		t.Fatal(err)
	}
	original := f.ship(t, f.originID, "galley")
	rescuer := f.ship(t, f.destID, "galley")
	if _, err := f.pool.Exec(ctx, `UPDATE units SET owner_id=$2 WHERE id=$1`, rescuer, enemy); err != nil {
		t.Fatal(err)
	}
	runner := f.waitingMessenger(t, f.originID, 500)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',carrier_unit_id=$2,hex_q=0 WHERE id=$1`, runner, original); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := carrier.OutcomeTx(ctx, tx, store, f.worldID, original, &rescuer, "battle", 3, 0, 500); err != nil {
		t.Fatal(err)
	}
	if _, err := carrier.PortTx(ctx, tx, store, f.worldID, rescuer, f.destID, 500); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	hub := &fakeRecallBroadcaster{}
	scan := NewPassageScanHandler(f.pool, events.NewScheduler(f.pool, clk), hub, clk, nil)
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 500}); err != nil {
		t.Fatal(err)
	}
	f.setTick(t, 504)
	if err := scan.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 504}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range hub.notified {
		if kind == "PassageStalled" {
			t.Fatalf("foreign rescue port must not send news without physical carrier: %+v", hub.notified)
		}
	}
}
