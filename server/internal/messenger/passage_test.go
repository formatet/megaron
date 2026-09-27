package messenger

// DB integration tests for megaron_plan_budet_liftar.md (slice 3a) — the core
// mechanics R2 (boarding), R3 (disembark scheduling), R4 (carrier lost →
// sealed → promoted) and R5 (reserve after N ticks with no carrier), proven
// directly against PassageScanHandler rather than through the full HTTP
// dispatch surface (that wiring — R1/R6 — is proven in
// api/handlers/messenger_passage_test.go).
//
// Fixture: two coastal settlements (origin q=0, dest q=5) separated by a sea
// lane (q=1..4) — same geography as messenger_trade_naval_test.go.

import (
	"context"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func passageTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type passageFixture struct {
	pool             *pgxpool.Pool
	worldID          uuid.UUID
	ownerID          uuid.UUID
	originID, destID uuid.UUID
	currentTick      int
}

func setupPassageFixture(t *testing.T) *passageFixture {
	t.Helper()
	pool := passageTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	f := &passageFixture{pool: pool, currentTick: 500}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"passage-"+uuid.New().String(), f.currentTick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"passage-owner-"+uuid.New().String(),
	).Scan(&f.ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	mkTile := func(q int, terrain string) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, $3)`,
			f.worldID, q, terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,0)=%s: %v", q, terrain, err)
		}
	}
	mkSettlement := func(q int, name string, coastal bool) uuid.UUID {
		mkTile(q, "plains")
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, $2, 0, 'plains', $3) RETURNING id`,
			f.worldID, q, coastal,
		).Scan(&prov); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
			f.worldID, prov, name, f.ownerID,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.originID = mkSettlement(0, "Passage-Origin-"+uuid.NewString(), true)
	for q := 1; q <= 4; q++ {
		mkTile(q, "coastal_sea")
	}
	f.destID = mkSettlement(5, "Passage-Dest-"+uuid.NewString(), true)
	return f
}

// ship inserts a garrisoned ship at settlementID.
func (f *passageFixture) ship(t *testing.T, settlementID uuid.UUID, shipType string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, name)
		 VALUES ($1, $2, $3, 'naval', 1, 10, 'garrison', $4, $5) RETURNING id`,
		f.worldID, f.ownerID, shipType, settlementID, "Test-"+shipType,
	).Scan(&id); err != nil {
		t.Fatalf("create ship: %v", err)
	}
	return id
}

// waitingMessenger inserts an 'awaiting_passage' outbound diplomatic
// messenger from f.originID to f.destID, standing at portID.
func (f *passageFixture) waitingMessenger(t *testing.T, portID uuid.UUID, sinceTick int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, arrives_at, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,$4,'hello','outbound','diplomatic',5,0,now(),'awaiting_passage',$5,$6) RETURNING id`,
		f.worldID, f.ownerID, f.originID, f.destID, portID, sinceTick,
	).Scan(&id); err != nil {
		t.Fatalf("create waiting messenger: %v", err)
	}
	return id
}

// waitingReturnMessenger inserts a 'returning' messenger (already delivered
// and replied/auto-returned) waiting for passage back home from portID — R6's
// case: the return leg's port is wherever the messenger currently stands,
// which may be a foreign city.
func (f *passageFixture) waitingReturnMessenger(t *testing.T, portID uuid.UUID, sinceTick int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, reply_text, status, kind,
		                          hex_q, hex_r, arrives_at, return_departs_at, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,$4,'hello','ok','returning','diplomatic',5,0,now(),now(),'awaiting_passage',$5,$6) RETURNING id`,
		f.worldID, f.ownerID, f.originID, f.destID, portID, sinceTick,
	).Scan(&id); err != nil {
		t.Fatalf("create waiting return messenger: %v", err)
	}
	return id
}

// departingTransport inserts an in-transit naval transport bound to shipID,
// departing portID toward f.destID, "just now" (within the scan's window).
func (f *passageFixture) departingTransport(t *testing.T, portID, shipID uuid.UUID, dueTick int) uuid.UUID {
	t.Helper()
	return f.departingTransportTo(t, portID, f.destID, 5, 0, shipID, dueTick)
}

// departingTransportTo is departingTransport with an explicit destination —
// used for R6's return leg, which departs a FOREIGN city toward the
// messenger's own home rather than toward f.destID.
func (f *passageFixture) departingTransportTo(t *testing.T, portID, destID uuid.UUID, destQ, destR int, shipID uuid.UUID, dueTick int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	now := time.Now()
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO transports (world_id, owner_id, kind, origin_id, dest_id, category,
		                          origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick,
		                          status, interceptable, ship_unit_id)
		 VALUES ($1,$2,'transfer',$3,$4,'naval',0,0,$5,$6,$7,$8,$9,'in_transit',true,$10) RETURNING id`,
		f.worldID, f.ownerID, portID, destID, destQ, destR, now, now.Add(time.Hour), dueTick, shipID,
	).Scan(&id); err != nil {
		t.Fatalf("create departing transport: %v", err)
	}
	return id
}

// marchingShip inserts a naval unit mid-march: q,r is its frozen departure
// hex (the sea hex adjacent to its port, per combat.StartMarch), target_q/r
// its destination hex, departing exactly at depart_tick and arriving at
// arriveTick.
func (f *passageFixture) marchingShip(t *testing.T, q, r, targetQ, targetR, departTick, arriveTick int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r,
		                    depart_tick, arrive_tick, departs_at, arrives_at, name)
		 VALUES ($1,$2,'merchantman','naval',1,10,'marching',$3,$4,$5,$6,$7,$8,now(),now()+interval '1 hour','Test-mission-ship')
		 RETURNING id`,
		f.worldID, f.ownerID, q, r, targetQ, targetR, departTick, arriveTick,
	).Scan(&id); err != nil {
		t.Fatalf("create marching ship: %v", err)
	}
	return id
}

func (f *passageFixture) setTick(t *testing.T, tick int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE worlds SET current_tick = $2 WHERE id = $1`, f.worldID, tick); err != nil {
		t.Fatalf("advance tick: %v", err)
	}
	f.currentTick = tick
}

func (f *passageFixture) messengerRow(t *testing.T, id uuid.UUID) (status string, passageStatus, carrierName *string, lostUntil *int) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, passage_status, carrier_name, passage_lost_until_tick FROM messengers WHERE id = $1`,
		id,
	).Scan(&status, &passageStatus, &carrierName, &lostUntil); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	return
}

// passageRow is messengerRow's fuller cousin — used by the tests that also
// need to see the carrier reference and the generation counter (R4's own
// review fix: a stale, superseded event must fail a generation check).
type passageRow struct {
	status                            string
	passageStatus                     *string
	carrierName                       *string
	carrierTransportID, carrierUnitID *uuid.UUID
	lostUntil                         *int
	generation                        int
}

func (f *passageFixture) fullRow(t *testing.T, id uuid.UUID) passageRow {
	t.Helper()
	var r passageRow
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, passage_status, carrier_name, carrier_transport_id, carrier_unit_id, passage_lost_until_tick, passage_generation
		   FROM messengers WHERE id = $1`,
		id,
	).Scan(&r.status, &r.passageStatus, &r.carrierName, &r.carrierTransportID, &r.carrierUnitID, &r.lostUntil, &r.generation); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	return r
}

func (f *passageFixture) countScheduled(t *testing.T, eventType string, messengerID uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_events WHERE event_type = $1 AND (payload->>'messenger_id') = $2`,
		eventType, messengerID.String(),
	).Scan(&n); err != nil {
		t.Fatalf("count scheduled %s: %v", eventType, err)
	}
	return n
}

// loadScheduledEvent loads the (single, expected) pending scheduled_events
// row of eventType for messengerID as an events.ScheduledEvent, so a test can
// fire it through the real handler directly — capturing it BEFORE a later
// scan supersedes it is how the R4 review-fix tests prove a stale event
// really is a no-op, not just that the row's own fields look right.
func (f *passageFixture) loadScheduledEvent(t *testing.T, eventType string, messengerID uuid.UUID) events.ScheduledEvent {
	t.Helper()
	var e events.ScheduledEvent
	var payload []byte
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id, world_id, event_type, payload, due_tick FROM scheduled_events
		  WHERE event_type = $1 AND (payload->>'messenger_id') = $2
		  ORDER BY id DESC LIMIT 1`,
		eventType, messengerID.String(),
	).Scan(&e.ID, &e.WorldID, &e.EventType, &payload, &e.DueTick); err != nil {
		t.Fatalf("load scheduled %s event: %v", eventType, err)
	}
	e.Payload = payload
	return e
}

func (f *passageFixture) handler() *PassageScanHandler {
	clk := clock.NewTestClock(time.Now())
	return NewPassageScanHandler(f.pool, events.NewScheduler(f.pool, clk), nil, clk)
}

// TestPassageScan_BoardsWaitingMessengerAndSchedulesDisembark is acceptance
// criterion 2: a messenger waiting at its own port boards a real transport
// that departs it, and the eventual delivery is scheduled for the carrier's
// arrival plus the (here, zero-distance) landward leg from the carrier's own
// destination.
func TestPassageScan_BoardsWaitingMessengerAndSchedulesDisembark(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick+1)
	shipID := f.ship(t, f.originID, "merchantman")
	transportDueTick := f.currentTick + 3
	f.departingTransport(t, f.originID, shipID, transportDueTick)

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	status, passageStatus, carrierName, _ := f.messengerRow(t, messengerID)
	if status != "outbound" {
		t.Errorf("status = %q, want still outbound (not delivered yet)", status)
	}
	// R4 review fix: 'aboard' must PERSIST for the whole voyage (not clear the
	// instant boarding happens) — otherwise a lost carrier mid-voyage can
	// never be found by detectLostCarriers.
	if passageStatus == nil || *passageStatus != "aboard" {
		t.Errorf("passage_status = %v, want aboard (still at sea, only cleared on actual delivery)", passageStatus)
	}
	if carrierName == nil || *carrierName == "" {
		t.Error("carrier_name not set — boarding did not record which ship carried it")
	}

	if n := f.countScheduled(t, "MessengerArrival", messengerID); n != 1 {
		t.Errorf("scheduled MessengerArrival count = %d, want 1", n)
	}
	var dueTick int
	if err := f.pool.QueryRow(ctx,
		`SELECT due_tick FROM scheduled_events WHERE event_type = 'MessengerArrival' AND (payload->>'messenger_id') = $1`,
		messengerID.String(),
	).Scan(&dueTick); err != nil {
		t.Fatalf("load scheduled due_tick: %v", err)
	}
	if dueTick < transportDueTick {
		t.Errorf("disembark due_tick = %d, want >= carrier's own due_tick %d", dueTick, transportDueTick)
	}
}

// TestPassageScan_BoardsShipMission is R2's other carrier kind
// (megaron_plan_skeppsuppdrag_landsatt.md): a ship starting an ordinary
// port-to-port march this exact tick is also an eligible carrier — its
// frozen origin hex (q,r) resolves to the port via
// province.NearestSettlementNeighbor, and its target hex resolves to the
// disembark point the same way.
func TestPassageScan_BoardsShipMission(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick+1)
	arriveTick := f.currentTick + 4
	// (1,0) neighbours the origin settlement (q=0); (4,0) neighbours the
	// destination settlement (q=5) — same axial layout the fixture's own
	// sea lane uses.
	f.marchingShip(t, 1, 0, 4, 0, f.currentTick, arriveTick)

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	_, passageStatus, carrierName, _ := f.messengerRow(t, messengerID)
	if passageStatus == nil || *passageStatus != "aboard" {
		t.Errorf("passage_status = %v, want aboard (still en route, cleared only on delivery)", passageStatus)
	}
	if carrierName == nil || *carrierName != "Test-mission-ship" {
		t.Errorf("carrier_name = %v, want Test-mission-ship", carrierName)
	}
	if n := f.countScheduled(t, "MessengerArrival", messengerID); n != 1 {
		t.Errorf("scheduled MessengerArrival count = %d, want 1", n)
	}
}

// TestPassageScan_BoardsReturnLegFromForeignCity is acceptance criterion 5
// (R6): a reply's return leg waits at wherever the messenger stands — which
// may be a foreign city the sender does not own — and boards any of the
// SENDER's own ships that happen to depart from there, e.g. slice 1b's own
// trade-ship leg 2 home. Boarding never checks who owns the port, only who
// owns the ship.
func TestPassageScan_BoardsReturnLegFromForeignCity(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	messengerID := f.waitingReturnMessenger(t, f.destID, f.currentTick+1)
	shipID := f.ship(t, f.destID, "merchantman") // the sender's own ship, docked in the (foreign) city the bud stands in
	transportDueTick := f.currentTick + 3
	// Home leg: destID (foreign, where the bud waits) -> originID (home).
	f.departingTransportTo(t, f.destID, f.originID, 0, 0, shipID, transportDueTick)

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	_, passageStatus, carrierName, _ := f.messengerRow(t, messengerID)
	if passageStatus == nil || *passageStatus != "aboard" {
		t.Errorf("passage_status = %v, want aboard (still en route, cleared only on delivery)", passageStatus)
	}
	if carrierName == nil || *carrierName == "" {
		t.Error("carrier_name not set on the return leg")
	}
	if n := f.countScheduled(t, "MessengerReturn", messengerID); n != 1 {
		t.Errorf("scheduled MessengerReturn count = %d, want 1", n)
	}
	if n := f.countScheduled(t, "MessengerArrival", messengerID); n != 0 {
		t.Errorf("scheduled MessengerArrival count = %d, want 0 (this is the return leg, not a fresh outbound)", n)
	}
}

// TestPassageScan_ReserveAfterWait is acceptance criterion 3: no eligible
// carrier ever departs, so after PassageReserveWaitTicks the messenger takes
// the old abstract crossing from its port.
func TestPassageScan_ReserveAfterWait(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	sinceTick := f.currentTick
	messengerID := f.waitingMessenger(t, f.originID, sinceTick)

	// Not yet due.
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (too early): %v", err)
	}
	_, passageStatus, _, _ := f.messengerRow(t, messengerID)
	if passageStatus == nil || *passageStatus != "awaiting_passage" {
		t.Fatalf("passage_status before the wait elapsed = %v, want still awaiting_passage", passageStatus)
	}

	f.setTick(t, sinceTick+PassageReserveWaitTicks)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (reserve due): %v", err)
	}
	_, passageStatus, _, _ = f.messengerRow(t, messengerID)
	if passageStatus != nil {
		t.Errorf("passage_status after reserve = %v, want NULL", *passageStatus)
	}
	if n := f.countScheduled(t, "MessengerArrival", messengerID); n != 1 {
		t.Errorf("scheduled MessengerArrival count = %d, want 1 (took the reserve crossing)", n)
	}
}

// TestPassageScan_LostCarrierSealsThenPromotes is acceptance criterion 4: a
// boarded messenger's transport is captured/limped/sunk (transport.seize
// flips transports.status to 'intercepted') — the messenger is sealed at its
// (unchanged) port for PassageLostDelayTicks, then reappears
// 'awaiting_passage', its contents never touched.
// TestPassageScan_LostCarrierSealsThenPromotes is the review's own reproduction
// of the R4 bug and its fix, run through the REAL flow end to end (not a
// hand-crafted 'aboard' fixture, which the original version of this test used
// and which the real code never actually produced — passage_status was
// cleared the instant boarding happened, making detectLostCarriers dead code
// in practice):
//
//  1. bud väntar → skepp avgår → scan bordar det (passage_status stays 'aboard')
//  2. bäraren kapas (transports.status='intercepted', as transport.seize does)
//  3. scan upptäcker förlusten → sealed, generation bumped
//  4. den GAMLA, nu inaktuella schemalagda händelsen fyrar → måste vara en no-op
//  5. efter fördröjningen → awaiting_passage igen
//  6. ett nytt skepp bordas → ny generation, ny händelse
//  7. den NYA händelsen fyrar → levererar verkligen, exakt en gång, innehållet orört
func TestPassageScan_LostCarrierSealsThenPromotes(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	arrivalH := NewArrivalHandler(f.pool, events.NewScheduler(f.pool, clock.NewTestClock(time.Now())), events.NewStore(f.pool), nil)

	// 1. Bordning via det riktiga svepet.
	messengerID := f.waitingMessenger(t, f.originID, f.currentTick+1)
	shipID := f.ship(t, f.originID, "merchantman")
	transportID := f.departingTransport(t, f.originID, shipID, f.currentTick+3)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (board): %v", err)
	}
	boarded := f.fullRow(t, messengerID)
	if boarded.passageStatus == nil || *boarded.passageStatus != "aboard" {
		t.Fatalf("passage_status after boarding = %v, want aboard", boarded.passageStatus)
	}
	if boarded.generation != 1 {
		t.Fatalf("generation after boarding = %d, want 1", boarded.generation)
	}
	staleEvt := f.loadScheduledEvent(t, "MessengerArrival", messengerID)

	// 2. Bäraren kapas (transport.seize's own flip, R5 of megaron_plan_sjohandel_kraver_skepp.md).
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET status = 'intercepted' WHERE id = $1`, transportID); err != nil {
		t.Fatalf("simulate seizure: %v", err)
	}

	// 3. Scan upptäcker förlusten och förseglar.
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (detect loss): %v", err)
	}
	sealed := f.fullRow(t, messengerID)
	if sealed.status != "outbound" {
		t.Errorf("status after loss = %q, want still outbound (never delivered, never lost)", sealed.status)
	}
	if sealed.passageStatus == nil || *sealed.passageStatus != "returning_sealed" {
		t.Fatalf("passage_status after loss = %v, want returning_sealed", sealed.passageStatus)
	}
	if sealed.carrierTransportID != nil {
		t.Errorf("carrier_transport_id after loss = %v, want cleared", *sealed.carrierTransportID)
	}
	if sealed.lostUntil == nil || *sealed.lostUntil != f.currentTick+PassageLostDelayTicks {
		t.Errorf("passage_lost_until_tick = %v, want %d", sealed.lostUntil, f.currentTick+PassageLostDelayTicks)
	}
	if sealed.generation != 2 {
		t.Fatalf("generation after seal = %d, want 2 (bumped so the stale event below recognizes itself)", sealed.generation)
	}

	// 4. THE BUG THIS TEST REPRODUCES: the OLD scheduled event (from step 1,
	// generation 1) is still sitting in the queue at its original due_tick.
	// Firing it now must be a no-op — before the review fix it would have
	// delivered the messenger on schedule regardless of the lost ship.
	if err := arrivalH.Handle(ctx, staleEvt); err != nil {
		t.Fatalf("stale event Handle: %v", err)
	}
	afterStale := f.fullRow(t, messengerID)
	if afterStale.status == "delivered" {
		t.Fatal("the STALE (superseded) event delivered the messenger anyway — R4 is dead code again")
	}
	if afterStale.passageStatus == nil || *afterStale.passageStatus != "returning_sealed" {
		t.Errorf("passage_status after firing the stale event = %v, want unchanged (returning_sealed)", afterStale.passageStatus)
	}
	var messageText string
	if err := f.pool.QueryRow(ctx, `SELECT message_text FROM messengers WHERE id = $1`, messengerID).Scan(&messageText); err != nil {
		t.Fatalf("load message text: %v", err)
	}
	if messageText != "hello" {
		t.Errorf("message_text = %q, want unchanged (never read/altered by the loss or the stale firing)", messageText)
	}

	// Not yet promoted before the delay elapses.
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (still sealed): %v", err)
	}
	stillSealed := f.fullRow(t, messengerID)
	if stillSealed.passageStatus == nil || *stillSealed.passageStatus != "returning_sealed" {
		t.Fatalf("passage_status before the delay elapsed = %v, want still returning_sealed", stillSealed.passageStatus)
	}

	// 5. Efter fördröjningen: promoteSealed släpper ut det igen.
	f.setTick(t, f.currentTick+PassageLostDelayTicks)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (promote): %v", err)
	}
	promoted := f.fullRow(t, messengerID)
	if promoted.passageStatus == nil || *promoted.passageStatus != "awaiting_passage" {
		t.Fatalf("passage_status after the delay = %v, want awaiting_passage again", promoted.passageStatus)
	}

	// 6. Ett nytt skepp avgår — bordar igen, ny generation, ny händelse.
	shipID2 := f.ship(t, f.originID, "galley")
	f.departingTransport(t, f.originID, shipID2, f.currentTick+3)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (reboard): %v", err)
	}
	reboarded := f.fullRow(t, messengerID)
	if reboarded.generation != 3 {
		t.Fatalf("generation after reboard = %d, want 3", reboarded.generation)
	}

	// 7. Den NYA händelsen fyrar — ska verkligen leverera budet, exakt en gång.
	realEvt := f.loadScheduledEvent(t, "MessengerArrival", messengerID)
	if err := arrivalH.Handle(ctx, realEvt); err != nil {
		t.Fatalf("real event Handle: %v", err)
	}
	final := f.fullRow(t, messengerID)
	if final.status != "delivered" {
		t.Fatalf("status after the real event fired = %q, want delivered", final.status)
	}
	if final.passageStatus != nil {
		t.Errorf("passage_status after delivery = %v, want NULL (cleared on real arrival)", final.passageStatus)
	}
	// Replaying the same (now legitimately-consumed) event again must still be
	// the ordinary, pre-existing idempotency no-op (status != 'outbound').
	if err := arrivalH.Handle(ctx, realEvt); err != nil {
		t.Fatalf("replay of the real event: %v", err)
	}
}

// TestPassageScan_LostShipMissionCarrierSeals is R4's other carrier kind
// (megaron_plan_skeppsuppdrag_landsatt.md): a ship mission sunk in combat
// (status flips to 'disbanded', the same terminal state ship_hull.go's own
// sinking path uses) seals its boarded messenger exactly like a captured
// sjötransport does.
func TestPassageScan_LostShipMissionCarrierSeals(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()

	messengerID := f.waitingMessenger(t, f.originID, f.currentTick+1)
	arriveTick := f.currentTick + 4
	shipID := f.marchingShip(t, 1, 0, 4, 0, f.currentTick, arriveTick)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (board): %v", err)
	}
	boarded := f.fullRow(t, messengerID)
	if boarded.carrierUnitID == nil || *boarded.carrierUnitID != shipID {
		t.Fatalf("carrier_unit_id after boarding = %v, want %s", boarded.carrierUnitID, shipID)
	}

	// Sunk in combat mid-voyage.
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status = 'disbanded' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("simulate sinking: %v", err)
	}

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (detect loss): %v", err)
	}
	sealed := f.fullRow(t, messengerID)
	if sealed.passageStatus == nil || *sealed.passageStatus != "returning_sealed" {
		t.Fatalf("passage_status after the ship sank = %v, want returning_sealed", sealed.passageStatus)
	}
	if sealed.carrierUnitID != nil {
		t.Errorf("carrier_unit_id after loss = %v, want cleared", *sealed.carrierUnitID)
	}
	if sealed.generation != 2 {
		t.Errorf("generation after seal = %d, want 2", sealed.generation)
	}
}
