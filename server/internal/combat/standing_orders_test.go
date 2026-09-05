package combat

// Proof for megaron_plan_staende_leverans.md's two named traps, plus the happy
// path and the return leg. Fixture: newSupportFixture (upkeep_support_settlement_test.go)
// gives a capital (q=0) and a town (q=4), both population 1000 — hex distance 4,
// travelMins = 30+4*2 = 38, travelTicks = round(38/60) = 1. Every test below
// reuses those numbers rather than re-deriving them.
//
// Reserve math used throughout (dispatchOutboundIfNeeded): a settlement of
// population 1000 eats 1000*GrainConsumptionPerCitizenPerTick = 5 grain/tick;
// the round-trip reserve is 5*(2*travelTicks) = 10. When the source also
// crews the route, VoyageProvisions(standingOrderRation(), 1, 0) = 1.0*2 = 2
// is folded in on top, so the full reserve at the source is 12.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newStandingOrder(t *testing.T, pool *pgxpool.Pool, worldID, owner, from, to, crew uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO standing_orders (world_id, owner_id, from_settlement_id, to_settlement_id, crewed_by_settlement_id)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		worldID, owner, from, to, crew,
	).Scan(&id); err != nil {
		t.Fatalf("create standing order: %v", err)
	}
	return id
}

func addOutbound(t *testing.T, pool *pgxpool.Pool, orderID uuid.UUID, good string, threshold float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO standing_order_outbound_goods (standing_order_id, good_key, threshold) VALUES ($1,$2,$3)`,
		orderID, good, threshold,
	); err != nil {
		t.Fatalf("add outbound good: %v", err)
	}
}

func addReturnGood(t *testing.T, pool *pgxpool.Pool, orderID uuid.UUID, good string, floor float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO standing_order_return_goods (standing_order_id, good_key, floor) VALUES ($1,$2,$3)`,
		orderID, good, floor,
	); err != nil {
		t.Fatalf("add return good: %v", err)
	}
}

// eventID hands out distinct scheduled-event ids so each call simulates a
// DIFFERENT tick's firing — reusing one id would trip the per-(event,order)
// idempotency claim and every call after the first would silently no-op,
// which would make every test below pass for the wrong reason.
var standingOrderTestEventSeq int64 = 900000

func nextEventID() int64 {
	standingOrderTestEventSeq++
	return standingOrderTestEventSeq
}

func runStandingOrderTick(t *testing.T, pool *pgxpool.Pool, f supportFixture) {
	t.Helper()
	h := NewStandingOrderTickHandler(pool,
		events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clock.NewTestClock(time.Now()),
		&fakeBroadcaster{})
	if err := h.Handle(context.Background(),
		events.ScheduledEvent{ID: nextEventID(), WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("standing order Handle: %v", err)
	}
}

func orderStatus(t *testing.T, pool *pgxpool.Pool, orderID uuid.UUID) (status string, reason *string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT status, pause_reason FROM standing_orders WHERE id = $1`, orderID,
	).Scan(&status, &reason); err != nil {
		t.Fatalf("read order status: %v", err)
	}
	return status, reason
}

func transportCountForOrder(t *testing.T, pool *pgxpool.Pool, orderID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM transports WHERE standing_order_id = $1`, orderID,
	).Scan(&n); err != nil {
		t.Fatalf("count transports: %v", err)
	}
	return n
}

func latestTransportForOrder(t *testing.T, pool *pgxpool.Pool, orderID uuid.UUID) (id uuid.UUID, kind, status string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT id, kind, status FROM transports WHERE standing_order_id = $1 ORDER BY created_at DESC LIMIT 1`,
		orderID,
	).Scan(&id, &kind, &status); err != nil {
		t.Fatalf("read latest transport: %v", err)
	}
	return id, kind, status
}

func transportGoodsCount(t *testing.T, pool *pgxpool.Pool, transportID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM transport_goods WHERE transport_id = $1`, transportID,
	).Scan(&n); err != nil {
		t.Fatalf("count transport_goods: %v", err)
	}
	return n
}

// placeGubbar fills n settlement_placement rows for a settlement — used to
// simulate "every gubbe is already at work" for the crew-availability test.
func placeGubbar(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
			 VALUES ($1, $2, 'hex', $3, 0, 'grain')`,
			settlementID, i, i,
		); err != nil {
			t.Fatalf("place gubbe %d: %v", i, err)
		}
	}
}

// 1. Happy path: destination below threshold, source has ample surplus, crew
// (the source itself) has a gubbe to spare — the route dispatches.
func TestStandingOrder_DispatchesWhenDestinationBelowThreshold(t *testing.T) {
	pool := testPool(t)
	f := newSupportFixture(t, pool, "so-happy")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	// townID starts with no grain row at all — settledStock reads that as 0.

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)

	runStandingOrderTick(t, pool, f)

	status, reason := orderStatus(t, pool, orderID)
	if status != "active" {
		t.Fatalf("order status = %q (reason=%v), want active — dispatch should have succeeded", status, reason)
	}
	if n := transportCountForOrder(t, pool, orderID); n != 1 {
		t.Fatalf("transports for order = %d, want 1", n)
	}
	_, kind, tstatus := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_out" || tstatus != "in_transit" {
		t.Errorf("latest transport = (%s, %s), want (standing_order_out, in_transit)", kind, tstatus)
	}

	// 1000 seeded − 200 shipped − 2 provisions (crew == source) = 798.
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != 798 {
		t.Errorf("capital grain after dispatch = %v, want 798 (1000 − 200 shipped − 2 provisions)", got)
	}
}

// 2. TRAP 1: a caravan already in flight must block a fresh dispatch, or a
// full shipment goes out every tick until the first one lands.
func TestStandingOrder_DoesNotRedispatchWhileOutboundInFlight(t *testing.T) {
	pool := testPool(t)
	f := newSupportFixture(t, pool, "so-trap1")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)

	runStandingOrderTick(t, pool, f) // dispatches leg 1
	if n := transportCountForOrder(t, pool, orderID); n != 1 {
		t.Fatalf("after first tick, transports = %d, want 1", n)
	}
	grainAfterFirst := settledAmount(t, pool, f.capitalID, "grain")

	// Simulate several more ticks firing while the caravan is still en route —
	// the destination is STILL below threshold (nothing has arrived yet), so a
	// non-mutexed sweep would dispatch again every time.
	runStandingOrderTick(t, pool, f)
	runStandingOrderTick(t, pool, f)
	runStandingOrderTick(t, pool, f)

	if n := transportCountForOrder(t, pool, orderID); n != 1 {
		t.Fatalf("transports for order after 3 more ticks = %d, want still 1 — "+
			"the sweep dispatched again while a caravan was already in flight (trap 1)", n)
	}
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != grainAfterFirst {
		t.Errorf("capital grain drifted from %v to %v across no-op ticks — goods were deducted again", grainAfterFirst, got)
	}
}

// 3. TRAP 2: the source's own need must be read before anything ships. A
// source sitting exactly at its own round-trip reserve must be left alone —
// the order pauses instead of draining it.
func TestStandingOrder_PausesInsteadOfStarvingSource(t *testing.T) {
	pool := testPool(t)
	f := newSupportFixture(t, pool, "so-trap2")
	// Exactly the reserve (10 own consumption + 2 provisions, crew == source):
	// zero is spendable.
	seedGoods(t, pool, f.capitalID, f.tick, 12, 0)

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)

	runStandingOrderTick(t, pool, f)

	status, reason := orderStatus(t, pool, orderID)
	if status != "paused" {
		t.Fatalf("order status = %q, want paused — a source at its own reserve must not ship", status)
	}
	if reason == nil || *reason == "" {
		t.Errorf("pause_reason is empty — a paused order must tell the Wanax why (never silent)")
	}
	if n := transportCountForOrder(t, pool, orderID); n != 0 {
		t.Errorf("transports for order = %d, want 0 — nothing should have shipped", n)
	}
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != 12 {
		t.Errorf("capital grain after pause = %v, want unchanged 12 — the worst bug named in the plan is an "+
			"automation that quietly starves the capital", got)
	}
}

// 4. Crew availability: a crewing settlement with no idle gubbe must pause
// rather than dispatch — a route cannot conjure a worker that doesn't exist.
func TestStandingOrder_PausesWhenCrewHasNoIdleGubbe(t *testing.T) {
	pool := testPool(t)
	f := newSupportFixture(t, pool, "so-crew")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	// Population 1000 = 10 gubbar; place all 10 so none is idle.
	placeGubbar(t, pool, f.capitalID, 10)

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)

	runStandingOrderTick(t, pool, f)

	status, reason := orderStatus(t, pool, orderID)
	if status != "paused" {
		t.Fatalf("order status = %q, want paused — every gubbe is already placed", status)
	}
	if reason == nil || *reason == "" {
		t.Errorf("pause_reason is empty")
	}
	if n := transportCountForOrder(t, pool, orderID); n != 0 {
		t.Errorf("transports for order = %d, want 0", n)
	}
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != 1000 {
		t.Errorf("capital grain after pause = %v, want unchanged 1000 — no goods should move if no gubbe is free", got)
	}
}

// 5. Return leg: once the outbound leg has landed (simulated here — the
// generic transport.ArrivalHandler is what flips it in production, unchanged
// by this slice), the sweep loads home the destination's surplus above the
// return floor and sends the SAME gubbe back. Also proves trap 1 holds for
// the return leg too: a second sweep tick while the return caravan is en
// route must not dispatch a second one.
func TestStandingOrder_ReturnLegAfterOutboundArrives(t *testing.T) {
	pool := testPool(t)
	f := newSupportFixture(t, pool, "so-return")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)
	addReturnGood(t, pool, orderID, "stone", 20)

	runStandingOrderTick(t, pool, f) // dispatch outbound leg

	outboundID, kind, status := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_out" || status != "in_transit" {
		t.Fatalf("latest transport = (%s, %s), want (standing_order_out, in_transit)", kind, status)
	}

	// Simulate the generic ArrivalHandler having credited the manifest and
	// marked the leg delivered (that handler is untouched by this slice — this
	// line stands in for it so the test doesn't need a live tick worker).
	if _, err := pool.Exec(context.Background(),
		`UPDATE transports SET status = 'delivered' WHERE id = $1`, outboundID,
	); err != nil {
		t.Fatalf("mark outbound delivered: %v", err)
	}
	// Destination now has stone above the return floor.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'stone', 50, 0, 1000000, $2)`,
		f.townID, f.tick,
	); err != nil {
		t.Fatalf("seed destination stone: %v", err)
	}

	runStandingOrderTick(t, pool, f) // should dispatch the return leg

	if n := transportCountForOrder(t, pool, orderID); n != 2 {
		t.Fatalf("transports for order = %d, want 2 (outbound + return)", n)
	}
	returnID, kind, status := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_return" || status != "in_transit" {
		t.Fatalf("latest transport = (%s, %s), want (standing_order_return, in_transit)", kind, status)
	}
	if got := settledAmount(t, pool, f.townID, "stone"); got != 20 {
		t.Errorf("destination stone after return dispatch = %v, want 20 (50 − 30 sent home, floor respected)", got)
	}

	// Trap 1 must hold for the return leg exactly as it does for the outbound
	// one: while it's in flight, further ticks must not dispatch a second one.
	runStandingOrderTick(t, pool, f)
	runStandingOrderTick(t, pool, f)
	if n := transportCountForOrder(t, pool, orderID); n != 2 {
		t.Errorf("transports for order after 2 more ticks = %d, want still 2 — "+
			"the return leg was re-dispatched while one was already in flight", n)
	}
	if _, _, s := latestTransportForOrder(t, pool, orderID); s != "in_transit" {
		t.Fatalf("return leg status = %q, want unchanged in_transit", s)
	}
	_ = returnID
}

// 6. An empty return leg (destination has nothing above the floor) must still
// send the gubbe home — quietly, per the plan — not skip dispatch entirely,
// or the mutex would never clear and the route would wedge forever.
func TestStandingOrder_EmptyReturnLegStillDispatches(t *testing.T) {
	pool := testPool(t)
	f := newSupportFixture(t, pool, "so-return-empty")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)
	addReturnGood(t, pool, orderID, "stone", 20)
	// Destination never seeds any stone — stock (0) sits below the floor (20).

	runStandingOrderTick(t, pool, f)
	outboundID, _, _ := latestTransportForOrder(t, pool, orderID)
	if _, err := pool.Exec(context.Background(),
		`UPDATE transports SET status = 'delivered' WHERE id = $1`, outboundID,
	); err != nil {
		t.Fatalf("mark outbound delivered: %v", err)
	}

	runStandingOrderTick(t, pool, f)

	returnID, kind, status := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_return" || status != "in_transit" {
		t.Fatalf("latest transport = (%s, %s), want (standing_order_return, in_transit) — "+
			"an empty return leg must still be sent, or the route wedges busy forever", kind, status)
	}
	if n := transportGoodsCount(t, pool, returnID); n != 0 {
		t.Errorf("return leg manifest rows = %d, want 0 (nothing to bring home)", n)
	}
}
