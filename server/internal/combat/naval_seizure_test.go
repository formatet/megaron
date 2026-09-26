package combat

// R5 (megaron_plan_sjohandel_kraver_skepp.md): NavalSeizureOutcomeHandler
// processes the "captured" branch of a naval interception — transport.
// InterceptScanHandler.seize rolled the outcome and credited the cargo
// (transport package, G1), this handler does the piece that needs combat's
// march machinery: change the ship's owner and send it home to the captor's
// nearest own port, reusing the exact same march/re-garrison path a battle-
// damaged ship's "damaged_return" already takes.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type navalCaptureFixture struct {
	worldID      uuid.UUID
	victim       uuid.UUID
	captor       uuid.UUID
	captorHomeID uuid.UUID
	captorHomeQ  int
	captorHomeR  int
	shipID       uuid.UUID
}

func newNavalCaptureFixture(t *testing.T, pool *pgxpool.Pool) navalCaptureFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f navalCaptureFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"victim-"+uuid.New().String(),
	).Scan(&f.victim); err != nil {
		t.Fatalf("create victim: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"captor-"+uuid.New().String(),
	).Scan(&f.captor); err != nil {
		t.Fatalf("create captor: %v", err)
	}

	f.captorHomeQ, f.captorHomeR = 9, 9
	var captorProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		f.worldID, f.captorHomeQ, f.captorHomeR,
	).Scan(&captorProv); err != nil {
		t.Fatalf("create captor province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Captorhaven', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`,
		f.worldID, captorProv, f.captor,
	).Scan(&f.captorHomeID); err != nil {
		t.Fatalf("create captor home: %v", err)
	}

	// A ship bound to a route/transfer, sitting freighting (R2: no live q/r —
	// the transport, not the ship, carries the map position) at the moment
	// it's captured.
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 10, 'freighting') RETURNING id`,
		f.worldID, f.victim,
	).Scan(&f.shipID); err != nil {
		t.Fatalf("create ship: %v", err)
	}

	return f
}

func TestNavalSeizureOutcome_CapturedChangesOwnerAndMarchesHome(t *testing.T) {
	pool := testPool(t)
	f := newNavalCaptureFixture(t, pool)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)

	h := NewNavalSeizureOutcomeHandler(pool, sched, clk, nil)
	payload := NavalSeizureOutcomePayload{
		TransportID: uuid.New(), ShipUnitID: f.shipID, Outcome: "captured",
		CaptorID: f.captor, Q: 5, R: 5,
	}
	raw, _ := json.Marshal(payload)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 100, Payload: raw}); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var ownerID uuid.UUID
	var status string
	var marchIntent *string
	var homeSettlementID *uuid.UUID
	var targetQ, targetR *int
	if err := pool.QueryRow(ctx,
		`SELECT owner_id, status, march_intent, home_settlement_id, target_q, target_r FROM units WHERE id = $1`,
		f.shipID,
	).Scan(&ownerID, &status, &marchIntent, &homeSettlementID, &targetQ, &targetR); err != nil {
		t.Fatalf("read ship: %v", err)
	}
	if ownerID != f.captor {
		t.Errorf("owner_id = %s, want captor %s", ownerID, f.captor)
	}
	if status != "marching" {
		t.Errorf("status = %q, want marching", status)
	}
	if marchIntent == nil || *marchIntent != "captured_return" {
		t.Errorf("march_intent = %v, want captured_return", marchIntent)
	}
	if homeSettlementID == nil || *homeSettlementID != f.captorHomeID {
		t.Errorf("home_settlement_id = %v, want captor's home %s", homeSettlementID, f.captorHomeID)
	}
	if targetQ == nil || targetR == nil {
		t.Fatal("target_q/target_r not set — no march was dispatched")
	}

	// Drive the arrival exactly like the tick worker would.
	var evID int64
	var evPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT id, payload FROM scheduled_events WHERE world_id = $1 AND event_type = 'UnitArrival' ORDER BY id DESC LIMIT 1`,
		f.worldID,
	).Scan(&evID, &evPayload); err != nil {
		t.Fatalf("no UnitArrival scheduled: %v", err)
	}
	arrivalH := NewUnitArrivalHandler(pool, events.NewStore(pool), nil, sched, clk, economy.SitosConfig{})
	if err := arrivalH.Handle(ctx, events.ScheduledEvent{ID: evID, WorldID: f.worldID, Payload: evPayload}); err != nil {
		t.Fatalf("unit arrival handle: %v", err)
	}

	var finalStatus string
	var finalSettlement *uuid.UUID
	var finalOwner uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id, owner_id FROM units WHERE id = $1`, f.shipID,
	).Scan(&finalStatus, &finalSettlement, &finalOwner); err != nil {
		t.Fatalf("read final ship state: %v", err)
	}
	if finalStatus != "garrison" {
		t.Errorf("final status = %q, want garrison", finalStatus)
	}
	if finalSettlement == nil || *finalSettlement != f.captorHomeID {
		t.Errorf("final settlement = %v, want captor home %s", finalSettlement, f.captorHomeID)
	}
	if finalOwner != f.captor {
		t.Errorf("final owner = %s, want captor %s", finalOwner, f.captor)
	}
}

func TestNavalSeizureOutcome_IdempotentOnReplay(t *testing.T) {
	pool := testPool(t)
	f := newNavalCaptureFixture(t, pool)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)

	h := NewNavalSeizureOutcomeHandler(pool, sched, clk, nil)
	payload := NavalSeizureOutcomePayload{
		TransportID: uuid.New(), ShipUnitID: f.shipID, Outcome: "captured",
		CaptorID: f.captor, Q: 5, R: 5,
	}
	raw, _ := json.Marshal(payload)
	ev := events.ScheduledEvent{WorldID: f.worldID, DueTick: 100, Payload: raw}
	if err := h.Handle(ctx, ev); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if err := h.Handle(ctx, ev); err != nil {
		t.Fatalf("second handle (replay): %v", err)
	}

	var arrivalCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events WHERE world_id = $1 AND event_type = 'UnitArrival'`, f.worldID,
	).Scan(&arrivalCount); err != nil {
		t.Fatalf("count UnitArrival events: %v", err)
	}
	if arrivalCount != 1 {
		t.Errorf("UnitArrival events scheduled = %d, want exactly 1 (replay must not re-dispatch)", arrivalCount)
	}
}
