package combat

// Amphibious assault (opposed landing). A laden galley reaches the sea hex next
// to an enemy coastal settlement it does not own; its cargo storms the beach.
// On a win the cargo becomes the settlement's garrison, ownership flips, and the
// settlement's stores (its tin) are captured with it. This drives the real
// resolve() dispatcher through the intent=assault branch.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func TestAmphibiousAssault_CapturesCoastalSettlementAndTin(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	mkPlayer := func(tag string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	attacker := mkPlayer("raider")
	defender := mkPlayer("islander")

	// Map: attacker harbour (0,0), sea running east; the defender's island hex
	// (3,0) is coastal, its landing hex is the sea tile (2,0).
	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"},
		{1, 0, "coastal_sea"},
		{2, 0, "coastal_sea"},
		{3, 0, "hills"},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, $2, $3, $4, $5)`,
			worldID, tl.q, tl.r, tl.terrain, tl.terrain == "hills",
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	// Attacker capital (needed for the demographic pop-loss write).
	var attCapProv uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&attCapProv)
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Raider Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		worldID, attCapProv, attacker,
	); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}

	// Defender's coastal island settlement, holding tin.
	var defProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 3, 0, 'hills', true) RETURNING id`,
		worldID,
	).Scan(&defProv); err != nil {
		t.Fatalf("create defender province: %v", err)
	}
	var defSettlement uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Amarna', 'khemetiu', $3, 'capital', true, 'active', 9000) RETURNING id`,
		worldID, defProv, defender,
	).Scan(&defSettlement); err != nil {
		t.Fatalf("create defender settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'tin', 5000, 0, 100000, 0)`,
		defSettlement,
	); err != nil {
		t.Fatalf("seed defender tin: %v", err)
	}
	// Defender garrison: a modest force the raider's cargo should overwhelm.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', $3)`,
		worldID, defender, defSettlement,
	); err != nil {
		t.Fatalf("create defender garrison: %v", err)
	}

	// The raider's cargo: a strong spearman unit, embarked.
	var cargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status)
		 VALUES ($1, $2, 'spearman', 'land', 1500, 0, 'embarked') RETURNING id`,
		worldID, attacker,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create cargo unit: %v", err)
	}
	// The laden galley, arriving at the landing hex (2,0) with intent=assault.
	var galleyID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units
		   (world_id, owner_id, type, category, size, crew, status, q, r,
		    target_q, target_r, departs_at, arrives_at, march_intent, cargo_unit_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'marching', 1, 0,
		         2, 0, now(), now(), 'assault', $3)
		 RETURNING id`,
		worldID, attacker, cargoID,
	).Scan(&galleyID); err != nil {
		t.Fatalf("create galley: %v", err)
	}

	fb := &fakeBroadcaster{}
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        fb,
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.resolve(ctx, tx, galleyID, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("resolve assault: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Settlement now belongs to the raider.
	var newOwner uuid.UUID
	var controlType string
	if err := pool.QueryRow(ctx,
		`SELECT owner_id, control_type FROM settlements WHERE id = $1`, defSettlement,
	).Scan(&newOwner, &controlType); err != nil {
		t.Fatalf("read captured settlement: %v", err)
	}
	if newOwner != attacker {
		t.Errorf("settlement owner = %s, want attacker %s", newOwner, attacker)
	}
	if controlType != "occupied" {
		t.Errorf("control_type = %q, want \"occupied\"", controlType)
	}

	// Both sides must be notified the settlement changed hands — the dispossessed
	// owner especially (async play → offline when the raid lands). Regression guard
	// for the amphibious notification gap (was silent: only unit/combat-stream events).
	var captures int
	for _, k := range fb.notified {
		if k == "SettlementCaptured" {
			captures++
		}
	}
	if captures != 2 {
		t.Errorf("SettlementCaptured notifications = %d, want 2 (defender + attacker); got %v", captures, fb.notified)
	}

	// The tin came with it.
	var tin float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'tin'`, defSettlement,
	).Scan(&tin); err != nil {
		t.Fatalf("read captured tin: %v", err)
	}
	if tin != 5000 {
		t.Errorf("captured tin = %v, want 5000", tin)
	}

	// The cargo disembarked as the new garrison of the captured settlement.
	var cargoStatus string
	var cargoSettlement *uuid.UUID
	var cargoSize int
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id, size FROM units WHERE id = $1`, cargoID,
	).Scan(&cargoStatus, &cargoSettlement, &cargoSize); err != nil {
		t.Fatalf("read cargo after assault: %v", err)
	}
	if cargoStatus != "garrison" || cargoSettlement == nil || *cargoSettlement != defSettlement {
		t.Errorf("cargo status=%q settlement=%v, want garrison at %s", cargoStatus, cargoSettlement, defSettlement)
	}
	if cargoSize <= 0 {
		t.Errorf("cargo size after win = %d, want > 0", cargoSize)
	}

	// The captured metropolis is demoted to an ordinary colony — no Wanax may hold
	// two capitals (regression: is_capital stayed true under the conqueror).
	var stillCapital bool
	if err := pool.QueryRow(ctx,
		`SELECT is_capital FROM settlements WHERE id = $1`, defSettlement,
	).Scan(&stillCapital); err != nil {
		t.Fatalf("read is_capital after capture: %v", err)
	}
	if stillCapital {
		t.Errorf("captured settlement is_capital = true, want false (conqueror must not gain a second capital)")
	}

	// No ghost garrison: the defeated defender's surviving units are evicted, not
	// left as the conqueror's troops (regression for the ghost-garrison bug).
	var ghostGarrison int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM units
		 WHERE settlement_id = $1 AND status = 'garrison' AND owner_id = $2`,
		defSettlement, defender,
	).Scan(&ghostGarrison); err != nil {
		t.Fatalf("count ghost garrison: %v", err)
	}
	if ghostGarrison != 0 {
		t.Errorf("defender ghost garrison units = %d, want 0 (evicted on capture)", ghostGarrison)
	}

	// The galley is empty and positioned at the landing hex.
	var galleyStatus string
	var galleyCargo *uuid.UUID
	var galleyQ, galleyR int
	if err := pool.QueryRow(ctx,
		`SELECT status, cargo_unit_id, q, r FROM units WHERE id = $1`, galleyID,
	).Scan(&galleyStatus, &galleyCargo, &galleyQ, &galleyR); err != nil {
		t.Fatalf("read galley after assault: %v", err)
	}
	if galleyStatus != "positioned" || galleyCargo != nil || galleyQ != 2 || galleyR != 0 {
		t.Errorf("galley status=%q cargo=%v pos=(%d,%d), want positioned/empty at (2,0)",
			galleyStatus, galleyCargo, galleyQ, galleyR)
	}
}
