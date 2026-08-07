package combat

// Amphibious assault (opposed landing). A laden galley reaches the sea hex next
// to an enemy coastal settlement it does not own; its cargo storms the beach.
// This drives the real resolve() dispatcher through the intent=assault branch.
//
// KR3 cutover (megaron_plan_kr3_stridssystem.md §8): resolveAmphibiousAssault
// no longer resolves the fight itself — it only initiates a persistent
// battles row (cargo as attacker, garrison as defender) via
// initiateOrJoinBattle, same as resolveFieldCombat/resolveCombat. This test
// now asserts that initiation (a battle + both participants exist right after
// resolve(), cargo already landed/positioned, galley already emptied at sea),
// then drives BattleTickHandler to completion to reach the fight's outcome.
// It no longer asserts settlement capture/ownership/tin/is_capital/garrison
// eviction — resolveAmphibiousAssault does not perform those anymore (see
// battle.go's header comment: capture-on-win is a deliberate, still-open gap,
// same as the land-march siege cutover).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestAmphibiousAssault_InitiatesBattleAndResolvesToWipeout(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
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
	var garrisonID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', $3) RETURNING id`,
		worldID, defender, defSettlement,
	).Scan(&garrisonID); err != nil {
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
		    target_q, target_r, departs_at, arrives_at, march_intent, cargo_unit_id, capture_mode)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'marching', 1, 0,
		         2, 0, now(), now(), 'assault', $3, 'annex')
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

	// ── Phase 1: initiation only, right after resolve() — no dice rolled yet. ──

	// The cargo already disembarked onto the settlement's own hex (3,0), not
	// the sea landing hex — it is the cargo that fights, not the ship.
	var cargoStatus string
	var cargoSettlement *uuid.UUID
	var cargoQ, cargoR, cargoSize int
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id, q, r, size FROM units WHERE id = $1`, cargoID,
	).Scan(&cargoStatus, &cargoSettlement, &cargoQ, &cargoR, &cargoSize); err != nil {
		t.Fatalf("read cargo after assault initiation: %v", err)
	}
	if cargoStatus != "positioned" || cargoSettlement != nil || cargoQ != 3 || cargoR != 0 {
		t.Errorf("cargo status=%q settlement=%v pos=(%d,%d), want positioned/nil at (3,0) — holding the contested hex while KR3 resolves it",
			cargoStatus, cargoSettlement, cargoQ, cargoR)
	}
	if cargoSize != 1500 {
		t.Errorf("cargo size = %d, want 1500 (full — no dice rolled yet)", cargoSize)
	}

	// The galley is already empty and positioned at the landing sea hex,
	// regardless of how the fight on shore eventually goes.
	var galleyStatus string
	var galleyCargo *uuid.UUID
	var galleyQ, galleyR int
	if err := pool.QueryRow(ctx,
		`SELECT status, cargo_unit_id, q, r FROM units WHERE id = $1`, galleyID,
	).Scan(&galleyStatus, &galleyCargo, &galleyQ, &galleyR); err != nil {
		t.Fatalf("read galley after assault initiation: %v", err)
	}
	if galleyStatus != "positioned" || galleyCargo != nil || galleyQ != 2 || galleyR != 0 {
		t.Errorf("galley status=%q cargo=%v pos=(%d,%d), want positioned/empty at (2,0)",
			galleyStatus, galleyCargo, galleyQ, galleyR)
	}

	var battleID uuid.UUID
	var battleStatus string
	if err := pool.QueryRow(ctx,
		`SELECT id, status FROM battles WHERE world_id = $1 AND q = 3 AND r = 0`, worldID,
	).Scan(&battleID, &battleStatus); err != nil {
		t.Fatalf("read battle: %v (no battles row — initiateOrJoinBattle did not run)", err)
	}
	if battleStatus != "active" {
		t.Fatalf("battle status = %q, want active immediately after initiation", battleStatus)
	}
	var participantCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battle_participants WHERE battle_id = $1`, battleID,
	).Scan(&participantCount); err != nil {
		t.Fatalf("count battle participants: %v", err)
	}
	if participantCount != 2 {
		t.Errorf("battle_participants count = %d, want 2 (cargo attacker + garrison defender)", participantCount)
	}

	// ── Phase 2: drive the battle to its conclusion (§2's state machine). ──

	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil)
	runBattleToEnd(t, pool, battleH, worldID, battleID, 20)

	var garrisonStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM units WHERE id = $1`, garrisonID,
	).Scan(&garrisonStatus); err != nil {
		t.Fatalf("read garrison unit: %v", err)
	}
	if garrisonStatus != "disbanded" {
		t.Errorf("garrison unit status = %q, want disbanded (1500 cargo vs 100 garrison must annihilate it within 20 battle-ticks)", garrisonStatus)
	}

	var finalCargoSize int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, cargoID).Scan(&finalCargoSize); err != nil {
		t.Fatalf("read cargo size: %v", err)
	}
	// KR3 rolls discrete T12 dice (§4) rather than a deterministic %-loss
	// formula — an overwhelming 1500-vs-100 win can plausibly cost the winner
	// zero men, so this only asserts it never GAINS men and the battle
	// actually concluded (garrison wiped above).
	if finalCargoSize > 1500 || finalCargoSize <= 0 {
		t.Errorf("cargo size = %d, want in (0, 1500]", finalCargoSize)
	}

	// The settlement itself is untouched by this slice's cutover — no
	// capture, no ownership transfer (see battle.go's header comment).
	var stillOwner uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT owner_id FROM settlements WHERE id = $1`, defSettlement,
	).Scan(&stillOwner); err != nil {
		t.Fatalf("read settlement owner: %v", err)
	}
	if stillOwner != defender {
		t.Errorf("settlement owner = %s, want unchanged defender %s (capture-on-win is not implemented by this cutover)", stillOwner, defender)
	}
}
