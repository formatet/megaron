package transport

// R1/R2 (megaron_plan_sjohandel_kraver_skepp.md): finding, binding and
// releasing the real ship a naval transport/route requires. DB-gated on
// DATABASE_URL, same fixture family as transport_test.go.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mkShip inserts a garrison ship of the given type at settlementID and
// returns its id.
func mkShip(t *testing.T, pool *pgxpool.Pool, f fixture, settlementID uuid.UUID, shipType string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, $3, 'naval', 1, 10, 'garrison', $4) RETURNING id`,
		f.worldID, f.owner, shipType, settlementID,
	).Scan(&id); err != nil {
		t.Fatalf("create ship %s: %v", shipType, err)
	}
	return id
}

func unitStatus(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) (status string, settlementID *uuid.UUID) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT status, settlement_id FROM units WHERE id = $1`, id,
	).Scan(&status, &settlementID); err != nil {
		t.Fatalf("read unit status: %v", err)
	}
	return status, settlementID
}

func TestFindFreeShip_PrefersMerchantmanOverGalley(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	galleyID := mkShip(t, pool, f, f.sourceID, "galley")
	merchantID := mkShip(t, pool, f, f.sourceID, "merchantman")

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	got, found, err := FindFreeShip(ctx, tx, f.worldID, f.owner, f.sourceID)
	if err != nil {
		t.Fatalf("find free ship: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got.ID != merchantID {
		t.Errorf("picked %s (galley=%s, merchantman=%s), want merchantman", got.ID, galleyID, merchantID)
	}
	if got.Capacity != ShipCapacityMerchantman {
		t.Errorf("capacity = %v, want %v", got.Capacity, ShipCapacityMerchantman)
	}
}

func TestFindFreeShip_NeverSelectsWarGalley(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	mkShip(t, pool, f, f.sourceID, "war_galley")

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	_, found, err := FindFreeShip(ctx, tx, f.worldID, f.owner, f.sourceID)
	if err != nil {
		t.Fatalf("find free ship: %v", err)
	}
	if found {
		t.Error("found a war_galley as a carrier — R1 forbids this, war_galley may never carry cargo")
	}
}

func TestBindReleaseShip_RoundTrip(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.sourceID, "merchantman")

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	ship, found, err := FindFreeShip(ctx, tx, f.worldID, f.owner, f.sourceID)
	if err != nil || !found {
		t.Fatalf("find free ship: found=%v err=%v", found, err)
	}
	if err := BindShip(ctx, tx, ship.ID); err != nil {
		t.Fatalf("bind ship: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	status, settlementID := unitStatus(t, pool, shipID)
	if status != "freighting" {
		t.Errorf("status after bind = %q, want freighting", status)
	}
	if settlementID == nil || *settlementID != f.sourceID {
		t.Errorf("settlement_id after bind = %v, want unchanged (%s)", settlementID, f.sourceID)
	}

	// Release at the destination — a bound ship may come home to a different
	// port than it left from (R3/R4 fallback to the nearest own port).
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin release: %v", err)
	}
	if err := ReleaseShip(ctx, tx2, shipID, f.destID); err != nil {
		t.Fatalf("release ship: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit release: %v", err)
	}

	status, settlementID = unitStatus(t, pool, shipID)
	if status != "garrison" {
		t.Errorf("status after release = %q, want garrison", status)
	}
	if settlementID == nil || *settlementID != f.destID {
		t.Errorf("settlement_id after release = %v, want %s", settlementID, f.destID)
	}
}

func TestBindShip_RefusesAnAlreadyBoundShip(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.sourceID, "merchantman")
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("pre-bind: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := BindShip(ctx, tx, shipID); err == nil {
		t.Error("BindShip on an already-freighting ship: want error, got nil")
	}
}

// TestFindFreeShip_DoubleBindingImpossible is R2's invariant under REAL
// concurrency, not just sequential re-checking: with exactly one free ship in
// port, a transaction that has already locked it (FOR UPDATE, uncommitted)
// makes a second, concurrent transaction's search see zero candidates —
// SKIP LOCKED means the second caller never blocks waiting for the first, it
// is simply told "none free" and must fall back to land or reject.
func TestFindFreeShip_DoubleBindingImpossible(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	shipID := mkShip(t, pool, f, f.sourceID, "merchantman")
	ctx := context.Background()

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	defer tx1.Rollback(ctx)
	first, found, err := FindFreeShip(ctx, tx1, f.worldID, f.owner, f.sourceID)
	if err != nil || !found || first.ID != shipID {
		t.Fatalf("tx1 find free ship: found=%v id=%v err=%v", found, first.ID, err)
	}
	if err := BindShip(ctx, tx1, first.ID); err != nil {
		t.Fatalf("tx1 bind: %v", err)
	}
	// tx1 deliberately left uncommitted — it still holds the row lock.

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer tx2.Rollback(ctx)
	_, found2, err := FindFreeShip(ctx, tx2, f.worldID, f.owner, f.sourceID)
	if err != nil {
		t.Fatalf("tx2 find free ship: %v", err)
	}
	if found2 {
		t.Error("tx2 found a free ship while tx1 held it uncommitted — double-binding is possible")
	}
}
