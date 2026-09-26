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
