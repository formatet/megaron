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

// departingTransport inserts an in-transit naval transport bound to shipID,
// departing portID toward f.destID, "just now" (within the scan's window).
func (f *passageFixture) departingTransport(t *testing.T, portID, shipID uuid.UUID, dueTick int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	now := time.Now()
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO transports (world_id, owner_id, kind, origin_id, dest_id, category,
		                          origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick,
		                          status, interceptable, ship_unit_id)
		 VALUES ($1,$2,'transfer',$3,$4,'naval',0,0,5,0,$5,$6,$7,'in_transit',true,$8) RETURNING id`,
		f.worldID, f.ownerID, portID, f.destID, now, now.Add(time.Hour), dueTick, shipID,
	).Scan(&id); err != nil {
		t.Fatalf("create departing transport: %v", err)
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
	if passageStatus != nil {
		t.Errorf("passage_status = %v, want NULL (boarded and disembark already scheduled)", *passageStatus)
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
func TestPassageScan_LostCarrierSealsThenPromotes(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	shipID := f.ship(t, f.originID, "merchantman")
	transportID := f.departingTransport(t, f.originID, shipID, f.currentTick+5)

	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, arrives_at, passage_status, passage_port_id, carrier_transport_id, carrier_name)
		 VALUES ($1,$2,$3,$4,'sealed cargo','outbound','diplomatic',5,0,now(),'aboard',$5,$6,'Test-merchantman') RETURNING id`,
		f.worldID, f.ownerID, f.originID, f.destID, f.originID, transportID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("create aboard messenger: %v", err)
	}

	if _, err := f.pool.Exec(ctx, `UPDATE transports SET status = 'intercepted' WHERE id = $1`, transportID); err != nil {
		t.Fatalf("simulate seizure: %v", err)
	}

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (detect loss): %v", err)
	}
	status, passageStatus, carrierName, lostUntil := f.messengerRow(t, messengerID)
	if status != "outbound" {
		t.Errorf("status after loss = %q, want still outbound (never delivered, never lost)", status)
	}
	if passageStatus == nil || *passageStatus != "returning_sealed" {
		t.Fatalf("passage_status after loss = %v, want returning_sealed", passageStatus)
	}
	if carrierName != nil {
		t.Errorf("carrier_name after loss = %v, want cleared", *carrierName)
	}
	if lostUntil == nil || *lostUntil != f.currentTick+PassageLostDelayTicks {
		t.Errorf("passage_lost_until_tick = %v, want %d", lostUntil, f.currentTick+PassageLostDelayTicks)
	}
	var messageText string
	if err := f.pool.QueryRow(ctx, `SELECT message_text FROM messengers WHERE id = $1`, messengerID).Scan(&messageText); err != nil {
		t.Fatalf("load message text: %v", err)
	}
	if messageText != "sealed cargo" {
		t.Errorf("message_text = %q, want unchanged (never read/altered by the loss)", messageText)
	}

	// Not yet promoted before the delay elapses.
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (still sealed): %v", err)
	}
	_, passageStatus, _, _ = f.messengerRow(t, messengerID)
	if passageStatus == nil || *passageStatus != "returning_sealed" {
		t.Fatalf("passage_status before the delay elapsed = %v, want still returning_sealed", passageStatus)
	}

	f.setTick(t, f.currentTick+PassageLostDelayTicks)
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatalf("Handle (promote): %v", err)
	}
	_, passageStatus, _, _ = f.messengerRow(t, messengerID)
	if passageStatus == nil || *passageStatus != "awaiting_passage" {
		t.Fatalf("passage_status after the delay = %v, want awaiting_passage again", passageStatus)
	}
}
