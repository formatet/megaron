package handlers

// End-to-end DB integration tests for megaron_plan_ordna_passage.md, slice
// 3b-3 ("Ordna passage"). Every acceptance scenario is driven through the
// REAL flow: the HTTP endpoint (Send/Arrange/Reply) → combat.StartMarch →
// the scheduled arrival events, fired through the SAME handlers the worker
// would call (never a fixture that hand-sets the end state) → internal/
// combat.UnitArrivalHandler → messenger.PassageScanHandler → the delivery
// handlers. Geography: navalPlayerFixture's two coastal settlements (q=0
// initiator, q=5 counterparty) joined by a sea lane (q=1..4) — same as
// messenger_passage_test.go's own acceptance criteria 2/3.
//
// §5 acceptance covered here:
//  1. Outbound message to a foreign coastal city: the runner boards, is
//     delivered, the recipient replies, the SAME ship carries the reply
//     home — both legs verified aboard (carrier_unit_id).
//  2. As 1, unanswered: the ship still carries it home once the stay ends.
//  3. An order to the initiator's own unit on the far shore: delivered, ship
//     goes straight home (no waiting).
//  4. Pickup: a runner already waiting in a foreign port is fetched by a
//     ship dispatched from a DIFFERENT own port.
//  5. Rejections (wrong port, war galley, ship on a mission).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
)

type passageArrangeFixture struct {
	*navalPlayerFixture
	scheduler      *events.Scheduler
	eventStore     *events.Store
	unitArrivalH   *combat.UnitArrivalHandler
	passageScanH   *messenger.PassageScanHandler
	msgArrivalH    *messenger.ArrivalHandler
	msgReturnH     *messenger.ReturnHandler
	stayEndH       *messenger.StayEndHandler
	orderDeliveryH *messenger.OrderDeliveryHandler
}

func setupPassageArrangeFixture(t *testing.T) *passageArrangeFixture {
	t.Helper()
	f := setupNavalPlayerFixture(t)
	scheduler := events.NewScheduler(f.pool, f.clk)
	eventStore := events.NewStore(f.pool)
	unitArrivalH := combat.NewUnitArrivalHandler(f.pool, eventStore, nil, scheduler, f.clk, economy.SitosConfig{})

	// Seed the self-perpetuating ScheduledPassageScan event this world would
	// otherwise only get from main.go's seedDailyTicks at server startup.
	if err := scheduler.EnqueueTick(context.Background(), f.worldID, events.ScheduledPassageScan, struct{}{}, 0); err != nil {
		t.Fatalf("seed passage scan: %v", err)
	}

	return &passageArrangeFixture{
		navalPlayerFixture: f,
		scheduler:          scheduler,
		eventStore:         eventStore,
		unitArrivalH:       unitArrivalH,
		passageScanH:       messenger.NewPassageScanHandler(f.pool, scheduler, nil, f.clk, unitArrivalH),
		msgArrivalH:        messenger.NewArrivalHandler(f.pool, scheduler, eventStore, nil),
		msgReturnH:         messenger.NewReturnHandler(f.pool, eventStore, nil),
		stayEndH:           messenger.NewStayEndHandler(f.pool, scheduler, f.clk),
		orderDeliveryH:     messenger.NewOrderDeliveryHandler(f.pool, scheduler, eventStore, nil, f.clk),
	}
}

// settlementAt is navalPlayerFixture.settlement generalised to an arbitrary
// (q,r) — that helper always places its settlement at r=0, which the pickup
// acceptance test (criterion 4) can't use: its second own port needs to sit
// off that row, land-adjacent to the origin, while still fronting the SAME
// sea lane one hex further along.
func settlementAt(t *testing.T, f *navalPlayerFixture, name string, q, r int, ownerID uuid.UUID, coastal bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
		f.worldID, q, r,
	); err != nil {
		t.Fatalf("insert map tile (%d,%d): %v", q, r, err)
	}
	var provinceID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, $2, $3, 'plains', $4) RETURNING id`,
		f.worldID, q, r, coastal,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province %s: %v", name, err)
	}
	var settlementID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
		f.worldID, provinceID, name, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement %s: %v", name, err)
	}
	return settlementID
}

func (f *passageArrangeFixture) setTick(t *testing.T, tick int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE worlds SET current_tick = $2 WHERE id = $1`, f.worldID, tick); err != nil {
		t.Fatalf("advance tick: %v", err)
	}
}

// loadPendingEventAny loads the single pending (unprocessed) scheduled_events
// row of eventType for this world — used for PassageScan, which carries no
// filterable payload (it is one self-perpetuating row per world).
func (f *passageArrangeFixture) loadPendingEventAny(t *testing.T, eventType string) events.ScheduledEvent {
	t.Helper()
	var e events.ScheduledEvent
	var payload []byte
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id, world_id, event_type, payload, due_tick FROM scheduled_events
		  WHERE world_id = $1 AND event_type = $2 AND processed_at IS NULL
		  ORDER BY due_tick ASC LIMIT 1`,
		f.worldID, eventType,
	).Scan(&e.ID, &e.WorldID, &e.EventType, &payload, &e.DueTick); err != nil {
		t.Fatalf("load pending %s event: %v", eventType, err)
	}
	e.Payload = payload
	return e
}

// loadPendingEventBy is loadPendingEventAny filtered by one payload key —
// used for UnitArrival (unit_id) and the messenger events (messenger_id).
func (f *passageArrangeFixture) loadPendingEventBy(t *testing.T, eventType, jsonKey string, jsonVal uuid.UUID) events.ScheduledEvent {
	t.Helper()
	var e events.ScheduledEvent
	var payload []byte
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id, world_id, event_type, payload, due_tick FROM scheduled_events
		  WHERE world_id = $1 AND event_type = $2 AND processed_at IS NULL AND (payload->>$3) = $4
		  AND ($3 <> 'messenger_id' OR event_type NOT IN ('MessengerArrival','MessengerReturn','OrderDelivery')
               OR COALESCE((payload->>'passage_generation')::int,0)=(SELECT passage_generation FROM messengers WHERE id=$4::uuid))
          ORDER BY due_tick ASC LIMIT 1`,
		f.worldID, eventType, jsonKey, jsonVal.String(),
	).Scan(&e.ID, &e.WorldID, &e.EventType, &payload, &e.DueTick); err != nil {
		t.Fatalf("load pending %s event (%s=%s): %v", eventType, jsonKey, jsonVal, err)
	}
	e.Payload = payload
	return e
}

func (f *passageArrangeFixture) markProcessed(t *testing.T, id int64) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE scheduled_events SET processed_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("mark event processed: %v", err)
	}
}

// runPassageScan fires the world's one PassageScan event through the real
// handler — the SAME code boardShipMissions/releasePassageWait run in
// production, driven by hand instead of the worker's poll loop.
func (f *passageArrangeFixture) runPassageScan(t *testing.T) {
	t.Helper()
	ev := f.loadPendingEventAny(t, string(events.ScheduledPassageScan))
	if err := f.passageScanH.Handle(context.Background(), ev); err != nil {
		t.Fatalf("passage scan: %v", err)
	}
	f.markProcessed(t, ev.ID)
}

// runUnitArrival advances the world to atTick and fires unitID's real
// ScheduledUnitArrival through combat.UnitArrivalHandler.
func (f *passageArrangeFixture) runUnitArrival(t *testing.T, atTick int, unitID uuid.UUID) {
	t.Helper()
	f.setTick(t, atTick)
	ev := f.loadPendingEventBy(t, string(events.ScheduledUnitArrival), "unit_id", unitID)
	if err := f.unitArrivalH.Handle(context.Background(), ev); err != nil {
		t.Fatalf("unit arrival: %v", err)
	}
	f.markProcessed(t, ev.ID)
}

// T2: a physical arrival freezes a witness. The real PassageScan consumes it
// and supersedes the boarding timer. Acceptance drivers must execute that phase
// and then the current generation's actual scheduled completion, just as workers do.
func (f *passageArrangeFixture) projectCarrierWitnesses(t *testing.T) {
	t.Helper()
	var pending bool
	if err := f.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM events e JOIN messengers m ON m.id=e.stream_id WHERE m.world_id=$1 AND e.id>m.carrier_witness_id AND e.event_type LIKE 'CarrierPassenger%V1')`, f.worldID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending {
		f.runPassageScan(t)
	}
}

func (f *passageArrangeFixture) runMessengerArrival(t *testing.T, atTick int, messengerID uuid.UUID) {
	t.Helper()
	f.setTick(t, atTick)
	f.projectCarrierWitnesses(t)
	ev := f.loadPendingEventBy(t, string(events.ScheduledMessengerArrival), "messenger_id", messengerID)
	f.setTick(t, ev.DueTick)
	if err := f.msgArrivalH.Handle(context.Background(), ev); err != nil {
		t.Fatalf("messenger arrival: %v", err)
	}
	f.markProcessed(t, ev.ID)
}

func (f *passageArrangeFixture) runMessengerReturn(t *testing.T, atTick int, messengerID uuid.UUID) {
	t.Helper()
	f.setTick(t, atTick)
	f.projectCarrierWitnesses(t)
	ev := f.loadPendingEventBy(t, string(events.ScheduledMessengerReturn), "messenger_id", messengerID)
	f.setTick(t, ev.DueTick)
	if err := f.msgReturnH.Handle(context.Background(), ev); err != nil {
		t.Fatalf("messenger return: %v", err)
	}
	f.markProcessed(t, ev.ID)
}

func (f *passageArrangeFixture) runStayEnd(t *testing.T, atTick int, messengerID uuid.UUID) {
	t.Helper()
	f.setTick(t, atTick)
	ev := f.loadPendingEventBy(t, string(events.ScheduledMessengerStayEnd), "messenger_id", messengerID)
	if err := f.stayEndH.Handle(context.Background(), ev); err != nil {
		t.Fatalf("stay end: %v", err)
	}
	f.markProcessed(t, ev.ID)
}

func (f *passageArrangeFixture) runOrderDelivery(t *testing.T, atTick int, messengerID uuid.UUID) {
	t.Helper()
	f.setTick(t, atTick)
	f.projectCarrierWitnesses(t)
	ev := f.loadPendingEventBy(t, string(events.ScheduledOrderDelivery), "messenger_id", messengerID)
	f.setTick(t, ev.DueTick)
	if err := f.orderDeliveryH.Handle(context.Background(), ev); err != nil {
		t.Fatalf("order delivery: %v", err)
	}
	f.markProcessed(t, ev.ID)
}

type passageMessengerRow struct {
	status        string
	passageStatus *string
	carrierUnitID *uuid.UUID
	passagePortID *uuid.UUID
}

func (f *passageArrangeFixture) messengerRow(t *testing.T, id uuid.UUID) passageMessengerRow {
	t.Helper()
	var r passageMessengerRow
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, passage_status, carrier_unit_id, passage_port_id FROM messengers WHERE id = $1`, id,
	).Scan(&r.status, &r.passageStatus, &r.carrierUnitID, &r.passagePortID); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	return r
}

type passageShipRow struct {
	status       string
	marchIntent  *string
	settlementID *uuid.UUID
}

func (f *passageArrangeFixture) shipRow(t *testing.T, id uuid.UUID) passageShipRow {
	t.Helper()
	var r passageShipRow
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, march_intent, settlement_id FROM units WHERE id = $1`, id,
	).Scan(&r.status, &r.marchIntent, &r.settlementID); err != nil {
		t.Fatalf("load ship: %v", err)
	}
	return r
}

// arrangePassage POSTs the Arrange endpoint as token and returns the ship's
// own arrival tick (the response's arrival_tick field), fatal-ing the test on
// any non-202.
func (f *passageArrangeFixture) arrangePassage(t *testing.T, token string, messengerID, shipID uuid.UUID) int {
	t.Helper()
	code, resp := f.post(t, token,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/passage",
		map[string]any{"ship_id": shipID.String()})
	if code != 202 {
		t.Fatalf("Arrange status = %d, want 202 (%v)", code, resp)
	}
	tickF, ok := resp["arrival_tick"].(float64)
	if !ok {
		t.Fatalf("Arrange response missing arrival_tick: %v", resp)
	}
	return int(tickF)
}

// ---------------------------------------------------------------------------
// Acceptance 1: outbound message, boarded, delivered, replied, boarded home.
// ---------------------------------------------------------------------------

func TestArrangePassage_OutboundRoundTrip_ShipCarriesBothLegs(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "Passage-Home", 0, f.initiatorID, true)
	foreignID := f.settlement(t, "Passage-Foreign", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	shipID := f.ship(t, homeID, f.initiatorID, "merchantman")

	// 1. Send over sea, no ship anywhere yet — awaiting_passage at home.
	code, sendResp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+homeID.String()+"/messengers",
		map[string]any{"destination_id": foreignID.String(), "message": "greetings across the water"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, sendResp)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM messengers WHERE origin_id = $1 AND destination_id = $2`, homeID, foreignID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load sent messenger: %v", err)
	}

	// 2. Arrange passage on the home ship.
	shipArriveTick := f.arrangePassage(t, f.initiatorToken, messengerID, shipID)

	// 3. Same-tick board: boardShipMissions must pick the runner up off the
	// SAME code the real scan runs, not a hand-set 'aboard' row.
	f.runPassageScan(t)
	boarded := f.messengerRow(t, messengerID)
	if boarded.passageStatus == nil || *boarded.passageStatus != "aboard" {
		t.Fatalf("outbound leg: passage_status = %v, want \"aboard\"", boarded.passageStatus)
	}
	if boarded.carrierUnitID == nil || *boarded.carrierUnitID != shipID {
		t.Fatalf("outbound leg: carrier_unit_id = %v, want the ship %v", boarded.carrierUnitID, shipID)
	}

	// 4. Ship reaches the foreign shore: delivery and the ship's own wait.
	f.runUnitArrival(t, shipArriveTick, shipID)
	f.runMessengerArrival(t, shipArriveTick, messengerID)

	waitingShip := f.shipRow(t, shipID)
	if waitingShip.status != "positioned" || waitingShip.marchIntent == nil || *waitingShip.marchIntent != "passage_wait" {
		t.Errorf("ship after landing = (%s, %v), want (\"positioned\", \"passage_wait\")", waitingShip.status, waitingShip.marchIntent)
	}
	delivered := f.messengerRow(t, messengerID)
	if delivered.status != "delivered" {
		t.Errorf("messenger status = %q, want \"delivered\"", delivered.status)
	}

	// 5. The counterparty replies.
	code, replyResp := f.post(t, f.counterpartyToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/reply",
		map[string]any{"reply": "safe travels"})
	if code != 200 {
		t.Fatalf("Reply status = %d, want 200 (%v)", code, replyResp)
	}

	// 6. The waiting ship is released and boards the reply in the SAME scan.
	f.runPassageScan(t)
	boardedHome := f.messengerRow(t, messengerID)
	if boardedHome.passageStatus == nil || *boardedHome.passageStatus != "aboard" {
		t.Fatalf("return leg: passage_status = %v, want \"aboard\"", boardedHome.passageStatus)
	}
	if boardedHome.carrierUnitID == nil || *boardedHome.carrierUnitID != shipID {
		t.Fatalf("return leg: carrier_unit_id = %v, want the SAME ship %v", boardedHome.carrierUnitID, shipID)
	}
	releasedShip := f.shipRow(t, shipID)
	if releasedShip.status != "marching" {
		t.Errorf("ship after release = %q, want \"marching\" (home)", releasedShip.status)
	}

	// 7. Both arrive home.
	var shipHomeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, shipID).Scan(&shipHomeTick); err != nil {
		t.Fatalf("load ship home arrival tick: %v", err)
	}
	f.runUnitArrival(t, shipHomeTick, shipID)
	f.runMessengerReturn(t, shipHomeTick, messengerID)

	finalShip := f.shipRow(t, shipID)
	if finalShip.status != "garrison" || finalShip.settlementID == nil || *finalShip.settlementID != homeID {
		t.Errorf("ship final state = (%s, %v), want (\"garrison\", %v)", finalShip.status, finalShip.settlementID, homeID)
	}
	finalMsg := f.messengerRow(t, messengerID)
	if finalMsg.status != "arrived" {
		t.Errorf("messenger final status = %q, want \"arrived\"", finalMsg.status)
	}
}

// ---------------------------------------------------------------------------
// Acceptance 2: as 1, unanswered — the stay ends and the ship still carries
// the runner home.
// ---------------------------------------------------------------------------

func TestArrangePassage_Unanswered_ShipCarriesHomeViaStayEnd(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "Passage-Home2", 0, f.initiatorID, true)
	foreignID := f.settlement(t, "Passage-Foreign2", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	shipID := f.ship(t, homeID, f.initiatorID, "galley")

	code, _ := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+homeID.String()+"/messengers",
		map[string]any{"destination_id": foreignID.String(), "message": "no reply needed"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201", code)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM messengers WHERE origin_id = $1 AND destination_id = $2`, homeID, foreignID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load sent messenger: %v", err)
	}

	shipArriveTick := f.arrangePassage(t, f.initiatorToken, messengerID, shipID)
	f.runPassageScan(t)
	f.runUnitArrival(t, shipArriveTick, shipID)
	f.runMessengerArrival(t, shipArriveTick, messengerID)

	waiting := f.shipRow(t, shipID)
	if waiting.status != "positioned" || waiting.marchIntent == nil || *waiting.marchIntent != "passage_wait" {
		t.Fatalf("ship after landing = (%s, %v), want (\"positioned\", \"passage_wait\")", waiting.status, waiting.marchIntent)
	}

	// The stay ends with no reply — StartReturnLeg runs with replyText=nil.
	stayEndTick := shipArriveTick + messenger.ReplyStayTicks
	f.runStayEnd(t, stayEndTick, messengerID)

	f.runPassageScan(t)
	homeward := f.messengerRow(t, messengerID)
	if homeward.passageStatus == nil || *homeward.passageStatus != "aboard" || homeward.carrierUnitID == nil || *homeward.carrierUnitID != shipID {
		t.Fatalf("unanswered runner not boarded for the ride home: %+v", homeward)
	}

	var shipHomeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, shipID).Scan(&shipHomeTick); err != nil {
		t.Fatalf("load ship home arrival tick: %v", err)
	}
	f.runUnitArrival(t, shipHomeTick, shipID)
	f.runMessengerReturn(t, shipHomeTick, messengerID)

	finalShip := f.shipRow(t, shipID)
	if finalShip.status != "garrison" {
		t.Errorf("ship final status = %q, want \"garrison\"", finalShip.status)
	}
	finalMsg := f.messengerRow(t, messengerID)
	if finalMsg.status != "arrived" {
		t.Errorf("messenger final status = %q, want \"arrived\"", finalMsg.status)
	}
	if finalMsg.status == "arrived" {
		var replyText *string
		if err := f.pool.QueryRow(context.Background(), `SELECT reply_text FROM messengers WHERE id = $1`, messengerID).Scan(&replyText); err != nil {
			t.Fatalf("load reply text: %v", err)
		}
		if replyText != nil {
			t.Errorf("reply_text = %q, want nil — nobody answered", *replyText)
		}
	}
}

// ---------------------------------------------------------------------------
// Acceptance 3: an order to the initiator's OWN unit on the far shore —
// delivered, ship goes straight home, no waiting.
// ---------------------------------------------------------------------------

func TestArrangePassage_OrderToOwnUnit_ShipGoesHomeImmediately(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "Passage-Home3", 0, f.initiatorID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	// Planner review 2026-09-27: NO settlement anywhere near the target — a
	// landmass with no city at all. R2's disembark search must still find an
	// EMPTY coastal hex (5,0) with a land route onward, never depending on a
	// settlement existing to find its footing.
	f.mapTile(t, 5, 0, "plains")
	farQ, farR := 6, 0
	f.mapTile(t, farQ, farR, "plains")
	shipID := f.ship(t, homeID, f.initiatorID, "galley")

	var targetUnitID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', $3, $4) RETURNING id`,
		f.worldID, f.initiatorID, farQ, farR,
	).Scan(&targetUnitID); err != nil {
		t.Fatalf("create field target unit: %v", err)
	}

	// Seed an order runner exactly as api/handlers.UnitHandler.sendOrderCourier
	// would (kind='order', dest_q/r = the target unit's position), already
	// awaiting_passage at the initiator's own port — dispatching it through
	// the full March→courier HTTP path is covered elsewhere (temenos_
	// orderlopare_plan.md); this test's own subject is the passage mechanism
	// at the arrival/release end, not the order courier's own dispatch.
	stanceOrder := combat.StanceOrder{WorldID: f.worldID, PlayerID: f.initiatorID, UnitID: targetUnitID, Stance: "fortify"}
	payload := messenger.OrderDeliveryPayload{
		WorldID: f.worldID, PlayerID: f.initiatorID, UnitID: targetUnitID,
		Verb: "stance", Stance: &stanceOrder,
	}
	payloadJSON := mustJSON(payload)
	var orderMsgID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, dest_q, dest_r, arrives_at, order_payload, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,NULL,'Runner — stance order.','outbound','order',0,0,$4,$5,now(),$6,'awaiting_passage',$3,0)
		 RETURNING id`,
		f.worldID, f.initiatorID, homeID, farQ, farR, payloadJSON,
	).Scan(&orderMsgID); err != nil {
		t.Fatalf("seed order runner: %v", err)
	}
	payload.MessengerID = orderMsgID
	if _, err := f.pool.Exec(context.Background(), `UPDATE messengers SET order_payload = $1 WHERE id = $2`, mustJSON(payload), orderMsgID); err != nil {
		t.Fatalf("update order payload with messenger id: %v", err)
	}

	shipArriveTick := f.arrangePassage(t, f.initiatorToken, orderMsgID, shipID)
	f.runPassageScan(t)

	boarded := f.messengerRow(t, orderMsgID)
	if boarded.passageStatus == nil || *boarded.passageStatus != "aboard" || boarded.carrierUnitID == nil || *boarded.carrierUnitID != shipID {
		t.Fatalf("order runner not boarded: %+v", boarded)
	}

	f.runUnitArrival(t, shipArriveTick, shipID)
	f.runOrderDelivery(t, shipArriveTick, orderMsgID)

	// The ship never waits for an order — it turns for home the instant it lands.
	ship := f.shipRow(t, shipID)
	if ship.status != "marching" || ship.marchIntent == nil || *ship.marchIntent != "explore_return" {
		t.Errorf("ship after landing an order = (%s, %v), want (\"marching\", \"explore_return\") — no waiting for an order", ship.status, ship.marchIntent)
	}

	msg := f.messengerRow(t, orderMsgID)
	if msg.status != "arrived" {
		t.Errorf("order messenger status = %q, want \"arrived\"", msg.status)
	}
	var targetStance *string
	if err := f.pool.QueryRow(context.Background(), `SELECT stance FROM units WHERE id = $1`, targetUnitID).Scan(&targetStance); err != nil {
		t.Fatalf("load target unit stance: %v", err)
	}
	if targetStance == nil || *targetStance != "fortify" {
		t.Errorf("target unit stance = %v, want \"fortify\" — the order was actually delivered", targetStance)
	}

	var shipHomeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, shipID).Scan(&shipHomeTick); err != nil {
		t.Fatalf("load ship home arrival tick: %v", err)
	}
	f.runUnitArrival(t, shipHomeTick, shipID)
	finalShip := f.shipRow(t, shipID)
	if finalShip.status != "garrison" {
		t.Errorf("ship final status = %q, want \"garrison\"", finalShip.status)
	}
}

// ---------------------------------------------------------------------------
// Acceptance 4: pickup — a runner already waiting in a FOREIGN port, fetched
// by a ship dispatched from a DIFFERENT own port.
// ---------------------------------------------------------------------------

func TestArrangePassage_Pickup_ShipFetchesFromDifferentOwnPort(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	// Two of the initiator's own ports on the SAME landmass: originID(0,0) —
	// the one the runner originally sailed from — and secondPortID(0,1), a
	// land neighbour of originID that fronts the SAME sea lane one hex further
	// along (a neighbour of the sea tile at (1,0) too) — so "any own port will
	// do" is a real choice: a different city, reachable home by land from the
	// runner's true origin, not a second landmass of its own.
	originID := f.settlement(t, "Passage-Origin4", 0, f.initiatorID, true)
	secondPortID := settlementAt(t, f.navalPlayerFixture, "Passage-SecondPort4", 0, 1, f.initiatorID, true)
	foreignID := f.settlement(t, "Passage-Foreign4", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	fetchShipID := f.ship(t, secondPortID, f.initiatorID, "merchantman")

	// The runner already delivered its message and is standing 'returning',
	// awaiting_passage, in the FOREIGN port — the state a normal Reply/
	// stay-end leaves it in (proven in criteria 1/2 above); seeded directly
	// here since this test's own subject is the pickup arrangement itself.
	var runnerID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, reply_text, status, kind,
		                          hex_q, hex_r, arrives_at, return_departs_at, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,$4,'hello','safe travels','returning','diplomatic',5,0,now(),now(),'awaiting_passage',$4,0)
		 RETURNING id`,
		f.worldID, f.initiatorID, originID, foreignID,
	).Scan(&runnerID); err != nil {
		t.Fatalf("seed waiting-in-foreign-port runner: %v", err)
	}

	shipArriveTick := f.arrangePassage(t, f.initiatorToken, runnerID, fetchShipID)

	// The fetching ship sails to the foreign port itself (R2's pickup rule) —
	// no boarding happens at ITS departure (the runner isn't waiting there).
	pending := f.messengerRow(t, runnerID)
	if pending.passageStatus == nil || *pending.passageStatus != "awaiting_passage" {
		t.Fatalf("runner should still be awaiting_passage before the ship arrives: %+v", pending)
	}

	f.runUnitArrival(t, shipArriveTick, fetchShipID)
	waitingShip := f.shipRow(t, fetchShipID)
	if waitingShip.status != "positioned" || waitingShip.marchIntent == nil || *waitingShip.marchIntent != "passage_wait" {
		t.Fatalf("fetching ship after arrival = (%s, %v), want (\"positioned\", \"passage_wait\")", waitingShip.status, waitingShip.marchIntent)
	}

	// The release phase finds the runner ready and departs — boarding it in
	// the SAME scan.
	f.runPassageScan(t)
	boarded := f.messengerRow(t, runnerID)
	if boarded.passageStatus == nil || *boarded.passageStatus != "aboard" || boarded.carrierUnitID == nil || *boarded.carrierUnitID != fetchShipID {
		t.Fatalf("runner not boarded by the fetching ship: %+v", boarded)
	}
	released := f.shipRow(t, fetchShipID)
	if released.status != "marching" {
		t.Errorf("fetching ship after release = %q, want \"marching\"", released.status)
	}

	var shipHomeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, fetchShipID).Scan(&shipHomeTick); err != nil {
		t.Fatalf("load fetching ship's home arrival tick: %v", err)
	}
	f.runUnitArrival(t, shipHomeTick, fetchShipID)
	f.runMessengerReturn(t, shipHomeTick, runnerID)

	finalShip := f.shipRow(t, fetchShipID)
	if finalShip.status != "garrison" || finalShip.settlementID == nil || *finalShip.settlementID != secondPortID {
		t.Errorf("fetching ship final state = (%s, %v), want (\"garrison\", %v) — its OWN port, not the runner's original one", finalShip.status, finalShip.settlementID, secondPortID)
	}
	finalMsg := f.messengerRow(t, runnerID)
	if finalMsg.status != "arrived" {
		t.Errorf("runner final status = %q, want \"arrived\"", finalMsg.status)
	}
}

// ---------------------------------------------------------------------------
// Planner review (2026-09-27): an unseen foreign coastal city nearer the
// target than any empty coastal hex must never be chosen, and must never
// leak into the response — resolveOutboundDisembark's candidates are raw
// coastline, never settlements (bar the target's own, already-known hex).
// ---------------------------------------------------------------------------

func TestArrangePassage_OrderToUnseenLandmass_PicksEmptyShoreNotForeignCity(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "Passage-Home6", 0, f.initiatorID, true)
	for q := 1; q <= 5; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	// A foreign, UNSEEN coastal city at (6,0) — ring distance 2 from the
	// target (8,0), with its own valid land route there via (7,0). Nothing
	// scouts it: no player_scouted_tiles row is ever written for it, exactly
	// as an unvisited foreign city would sit in a live world.
	f.settlement(t, "Passage-UnseenCity6", 6, f.counterpartyID, true)
	f.mapTile(t, 7, 0, "plains")
	f.mapTile(t, 8, 0, "plains") // the target unit's own hex
	// A farther, EMPTY coastal hex at (6,-1) — ring distance 3 from the
	// target, reachable by land via (7,-1)→(8,-1)→(8,0) and by sea via the
	// same lane (adjacent to (5,0)). The only valid candidate once the
	// nearer city is correctly excluded.
	f.mapTile(t, 6, -1, "plains")
	f.mapTile(t, 7, -1, "plains")
	f.mapTile(t, 8, -1, "plains")
	shipID := f.ship(t, homeID, f.initiatorID, "galley")

	farQ, farR := 8, 0
	var targetUnitID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', $3, $4) RETURNING id`,
		f.worldID, f.initiatorID, farQ, farR,
	).Scan(&targetUnitID); err != nil {
		t.Fatalf("create field target unit: %v", err)
	}

	stanceOrder := combat.StanceOrder{WorldID: f.worldID, PlayerID: f.initiatorID, UnitID: targetUnitID, Stance: "fortify"}
	payload := messenger.OrderDeliveryPayload{
		WorldID: f.worldID, PlayerID: f.initiatorID, UnitID: targetUnitID,
		Verb: "stance", Stance: &stanceOrder,
	}
	var orderMsgID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, dest_q, dest_r, arrives_at, order_payload, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,NULL,'Runner — stance order.','outbound','order',0,0,$4,$5,now(),$6,'awaiting_passage',$3,0)
		 RETURNING id`,
		f.worldID, f.initiatorID, homeID, farQ, farR, mustJSON(payload),
	).Scan(&orderMsgID); err != nil {
		t.Fatalf("seed order runner: %v", err)
	}
	payload.MessengerID = orderMsgID
	if _, err := f.pool.Exec(context.Background(), `UPDATE messengers SET order_payload = $1 WHERE id = $2`, mustJSON(payload), orderMsgID); err != nil {
		t.Fatalf("update order payload with messenger id: %v", err)
	}

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+orderMsgID.String()+"/passage",
		map[string]any{"ship_id": shipID.String()})
	if code != 202 {
		t.Fatalf("Arrange status = %d, want 202 (%v)", code, resp)
	}

	disembarkQ, _ := resp["disembark_q"].(float64)
	disembarkR, _ := resp["disembark_r"].(float64)
	if int(disembarkQ) == 6 && int(disembarkR) == 0 {
		t.Fatalf("disembark hex = (6,0) — the foreign city itself, want the empty shore instead")
	}
	settled, err := hexIsSettled(context.Background(), f.pool, f.worldID, int(disembarkQ), int(disembarkR))
	if err != nil {
		t.Fatalf("check disembark hex settled: %v", err)
	}
	if settled {
		t.Errorf("disembark hex (%v,%v) is settled — the search must never depend on a city existing there", disembarkQ, disembarkR)
	}

	// The response must not name the foreign city anywhere — the whole point
	// of the fix is that arranging passage never reveals it.
	respJSON, _ := json.Marshal(resp)
	if strings.Contains(string(respJSON), "Passage-UnseenCity6") {
		t.Errorf("response leaks the unseen foreign city's name: %s", respJSON)
	}
}

// ---------------------------------------------------------------------------
// Acceptance 5: rejections.
// ---------------------------------------------------------------------------

func TestArrangePassage_Rejections(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "Passage-Home5", 0, f.initiatorID, true)
	otherOwnID := f.settlement(t, "Passage-Other5", 10, f.initiatorID, true)
	foreignID := f.settlement(t, "Passage-Foreign5", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	for q := 6; q <= 9; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}

	code, _ := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+homeID.String()+"/messengers",
		map[string]any{"destination_id": foreignID.String(), "message": "hello"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201", code)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM messengers WHERE origin_id = $1 AND destination_id = $2`, homeID, foreignID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load sent messenger: %v", err)
	}

	t.Run("ship in wrong own port", func(t *testing.T) {
		wrongPortShip := f.ship(t, otherOwnID, f.initiatorID, "merchantman")
		code, resp := f.post(t, f.initiatorToken,
			"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/passage",
			map[string]any{"ship_id": wrongPortShip.String()})
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("war galley refused", func(t *testing.T) {
		warGalley := f.ship(t, homeID, f.initiatorID, "war_galley")
		code, resp := f.post(t, f.initiatorToken,
			"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/passage",
			map[string]any{"ship_id": warGalley.String()})
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("ship already on a mission", func(t *testing.T) {
		busyShip := f.ship(t, homeID, f.initiatorID, "merchantman")
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE units SET status = 'marching', settlement_id = NULL, q = 1, r = 0, target_q = 2, target_r = 0 WHERE id = $1`,
			busyShip,
		); err != nil {
			t.Fatalf("send ship on a mission: %v", err)
		}
		code, resp := f.post(t, f.initiatorToken,
			"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/passage",
			map[string]any{"ship_id": busyShip.String()})
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("not your ship", func(t *testing.T) {
		theirShip := f.ship(t, foreignID, f.counterpartyID, "merchantman")
		code, resp := f.post(t, f.initiatorToken,
			"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/passage",
			map[string]any{"ship_id": theirShip.String()})
		if code != 403 {
			t.Errorf("status = %d, want 403 (%v)", code, resp)
		}
	})
}

// ---------------------------------------------------------------------------
// R0 (megaron_plan_hamta_hem.md, slice 2b): a passage ship arranged MID-TICK
// must have its runner aboard once the world moves on — not one scan later.
//
// boardShipMissions only ever boards a ship whose depart_tick equals the
// tick currently being scanned. A Wanax who arranges passage between two
// scans of the SAME tick T gets a ship with depart_tick=T — but the scan for
// T has already run by then, and the NEXT scan (at T+1) filters on
// depart_tick=T+1 and never finds it. Without R0's fix
// (messenger.BoardDispatchedShipRunner, called from Arrange right after
// combat.StartMarch succeeds) the runner sails "without" the ship it was
// just arranged onto.
// ---------------------------------------------------------------------------

func TestArrangePassage_DispatchedMidTick_RunnerBoardsAtDispatchNotNextScan(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "MidTick-Home", 0, f.initiatorID, true)
	foreignID := f.settlement(t, "MidTick-Foreign", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	shipID := f.ship(t, homeID, f.initiatorID, "merchantman")

	code, sendResp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+homeID.String()+"/messengers",
		map[string]any{"destination_id": foreignID.String(), "message": "mid-tick dispatch"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, sendResp)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM messengers WHERE origin_id = $1 AND destination_id = $2`, homeID, foreignID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load sent messenger: %v", err)
	}

	// Tick T's own scan has ALREADY run — nothing to board yet, the ship
	// hasn't been dispatched.
	f.runPassageScan(t)

	// The Wanax arranges passage mid-tick T: the world has not advanced since
	// the scan above ran, so the ship's depart_tick = T, the SAME tick that
	// scan already covered.
	shipArriveTick := f.arrangePassage(t, f.initiatorToken, messengerID, shipID)

	// Prove the fix fired synchronously inside Arrange — before any further
	// scan runs at all.
	boardedAtDispatch := f.messengerRow(t, messengerID)
	if boardedAtDispatch.passageStatus == nil || *boardedAtDispatch.passageStatus != "aboard" {
		t.Fatalf("mid-tick dispatch: passage_status = %v immediately after Arrange, want \"aboard\" (R0: boarded at dispatch, not by a later scan)", boardedAtDispatch.passageStatus)
	}
	if boardedAtDispatch.carrierUnitID == nil || *boardedAtDispatch.carrierUnitID != shipID {
		t.Fatalf("mid-tick dispatch: carrier_unit_id = %v, want the ship %v", boardedAtDispatch.carrierUnitID, shipID)
	}

	// The world advances to T+1 and that tick's own scan runs — this must be
	// a harmless no-op (boardOne's own awaiting_passage guard), not a double
	// boarding or an error.
	f.setTick(t, 1)
	f.runPassageScan(t)
	stillBoarded := f.messengerRow(t, messengerID)
	if stillBoarded.passageStatus == nil || *stillBoarded.passageStatus != "aboard" || stillBoarded.carrierUnitID == nil || *stillBoarded.carrierUnitID != shipID {
		t.Fatalf("mid-tick dispatch: a later scan disturbed the already-boarded runner: %+v", stillBoarded)
	}

	// Sanity: the rest of the voyage still plays out normally from here.
	f.runUnitArrival(t, shipArriveTick, shipID)
	f.runMessengerArrival(t, shipArriveTick, messengerID)
	delivered := f.messengerRow(t, messengerID)
	if delivered.status != "delivered" {
		t.Errorf("messenger status = %q, want \"delivered\"", delivered.status)
	}
}
