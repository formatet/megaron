package transport

// R3/R5 (megaron_plan_sjohandel_kraver_skepp.md): a "ship_return" or
// "damaged_return" transport releases its bound ship on arrival, in the same
// tx as its own delivered flip.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func dispatchShipLeg(t *testing.T, pool *pgxpool.Pool, f fixture, kind string, shipID uuid.UUID, originID, destID uuid.UUID, originQ, originR, destQ, destR int, m Manifest) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	now := time.Now()
	id, err := Dispatch(ctx, tx, sched, DispatchParams{
		WorldID: f.worldID, OwnerID: f.owner, Kind: kind,
		OriginID: originID, DestID: destID, Category: "naval",
		OriginQ: originQ, OriginR: originR, DestQ: destQ, DestR: destR,
		DepartsAt: now, ArrivesAt: now.Add(time.Hour), DueTick: 1,
		Manifest: m, Interceptable: true, ShipUnitID: &shipID,
	})
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("dispatch %s: %v", kind, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

func TestArrival_ShipReturnReleasesShipAtHome(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.destID, "merchantman")
	if _, err := pool.Exec(context.Background(), `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("bind ship: %v", err)
	}

	// Ship sails from destID (where it currently sits, mid-round-trip) home to sourceID.
	dispatchShipLeg(t, pool, f, "ship_return", shipID, f.destID, f.sourceID, 3, 0, 0, 0, nil)
	fireArrival(t, pool, f.worldID, nil)

	status, settlementID := unitStatus(t, pool, shipID)
	if status != "garrison" {
		t.Errorf("status after ship_return arrival = %q, want garrison", status)
	}
	if settlementID == nil || *settlementID != f.sourceID {
		t.Errorf("settlement_id after ship_return arrival = %v, want %s (home)", settlementID, f.sourceID)
	}
}

func TestArrival_ShipReturnFallsBackToNearestOwnPortWhenHomeGone(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.destID, "galley")
	if _, err := pool.Exec(context.Background(), `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("bind ship: %v", err)
	}
	// Home (sourceID) is no longer active — conquered/collapsed.
	if _, err := pool.Exec(context.Background(), `UPDATE settlements SET state = 'collapsed' WHERE id = $1`, f.sourceID); err != nil {
		t.Fatalf("collapse home: %v", err)
	}

	dispatchShipLeg(t, pool, f, "ship_return", shipID, f.destID, f.sourceID, 3, 0, 0, 0, nil)
	fireArrival(t, pool, f.worldID, nil)

	status, settlementID := unitStatus(t, pool, shipID)
	if status != "garrison" {
		t.Errorf("status = %q, want garrison (fallback release)", status)
	}
	// The only other active own settlement is destID itself.
	if settlementID == nil || *settlementID != f.destID {
		t.Errorf("settlement_id = %v, want fallback port %s", settlementID, f.destID)
	}
}

func TestArrival_ShipReturnStrandsWhenNoOwnPortLeft(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.destID, "galley")
	if _, err := pool.Exec(context.Background(), `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("bind ship: %v", err)
	}
	// Both settlements gone — the owner has nowhere left at all.
	if _, err := pool.Exec(context.Background(), `UPDATE settlements SET state = 'collapsed' WHERE world_id = $1`, f.worldID); err != nil {
		t.Fatalf("collapse all: %v", err)
	}

	fb := &fakeBroadcaster{}
	dispatchShipLeg(t, pool, f, "ship_return", shipID, f.destID, f.sourceID, 3, 0, 0, 0, nil)
	fireArrival(t, pool, f.worldID, fb)

	status, settlementID := unitStatus(t, pool, shipID)
	if status != "positioned" {
		t.Errorf("status = %q, want positioned (stranded)", status)
	}
	if settlementID != nil {
		t.Errorf("settlement_id = %v, want nil (stranded on the map)", settlementID)
	}
	found := false
	for _, k := range fb.notified {
		if k == "ShipStranded" {
			found = true
		}
	}
	if !found {
		t.Errorf("notified = %v, want a ShipStranded notice", fb.notified)
	}
}

func TestArrival_DamagedReturnCreditsHalfCargoAndReleasesShip(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.destID, "merchantman")
	if _, err := pool.Exec(context.Background(), `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("bind ship: %v", err)
	}

	dispatchShipLeg(t, pool, f, "damaged_return", shipID, f.destID, f.sourceID, 3, 0, 0, 0, Manifest{"silver": 50})
	fireArrival(t, pool, f.worldID, nil)

	if got := goodAmount(t, pool, f.sourceID, "silver"); got != 50 {
		t.Errorf("credited silver = %v, want 50", got)
	}
	status, settlementID := unitStatus(t, pool, shipID)
	if status != "garrison" {
		t.Errorf("status after damaged_return = %q, want garrison", status)
	}
	if settlementID == nil || *settlementID != f.sourceID {
		t.Errorf("settlement_id after damaged_return = %v, want %s", settlementID, f.sourceID)
	}
}

// TestArrival_StandingOrderLegDoesNotReleaseShipWhileOrderLives is R4: a
// standing sea route's own legs must NOT release their ship on arrival while
// the order still owns it (standing_order_id set) — the ship stays freighting
// for the whole route's lifetime; only combat.StandingOrderTickHandler
// releases it, and only once the order goes inactive and idle.
func TestArrival_StandingOrderLegDoesNotReleaseShipWhileOrderLives(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.sourceID, "merchantman")
	if _, err := pool.Exec(context.Background(), `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("bind ship: %v", err)
	}
	orderID := uuid.New() // stand-in FK target isn't needed — standing_order_id is just a tag column here

	ctx := context.Background()
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	now := time.Now()
	// standing_orders FK requires a real row — insert a minimal one so the
	// column actually holds a live reference, matching production shape.
	if _, err := pool.Exec(ctx,
		`INSERT INTO standing_orders (id, world_id, owner_id, from_settlement_id, to_settlement_id, crewed_by_settlement_id, ship_unit_id)
		 VALUES ($1, $2, $3, $4, $5, $4, $6)`,
		orderID, f.worldID, f.owner, f.sourceID, f.destID, shipID,
	); err != nil {
		t.Fatalf("insert standing order: %v", err)
	}
	if _, err := Dispatch(ctx, tx, sched, DispatchParams{
		WorldID: f.worldID, OwnerID: f.owner, Kind: "standing_order_out",
		OriginID: f.sourceID, DestID: f.destID, Category: "naval",
		OriginQ: 0, OriginR: 0, DestQ: 3, DestR: 0,
		DepartsAt: now, ArrivesAt: now.Add(time.Hour), DueTick: 1,
		Manifest: Manifest{"grain": 10}, Interceptable: true,
		ShipUnitID: &shipID, StandingOrderID: &orderID,
	}); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("dispatch: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	fireArrival(t, pool, f.worldID, nil)

	status, _ := unitStatus(t, pool, shipID)
	if status != "freighting" {
		t.Errorf("ship status after standing_order_out arrival (order still exists) = %q, want still freighting", status)
	}
}

// TestArrival_OrphanedStandingOrderLegReleasesShip is R4's delete-while-in-
// flight case: once the order is gone (standing_order_id NULLed by the
// ON DELETE SET NULL cascade), the ship is released wherever this orphaned
// leg lands — there is no order left to do it later.
func TestArrival_OrphanedStandingOrderLegReleasesShip(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.sourceID, "merchantman")
	if _, err := pool.Exec(context.Background(), `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("bind ship: %v", err)
	}

	// No standing_order_id — an orphan, as if the order had been deleted
	// while this leg was already in flight.
	dispatchShipLeg(t, pool, f, "standing_order_return", shipID, f.destID, f.sourceID, 3, 0, 0, 0, nil)
	fireArrival(t, pool, f.worldID, nil)

	status, settlementID := unitStatus(t, pool, shipID)
	if status != "garrison" {
		t.Errorf("status after orphaned leg arrival = %q, want garrison", status)
	}
	if settlementID == nil || *settlementID != f.sourceID {
		t.Errorf("settlement_id after orphaned leg arrival = %v, want %s", settlementID, f.sourceID)
	}
}
