package combat

// KR3 erövringens efterspel — RED BEFORE / GREEN AFTER (megaron_plan_erovring.md
// §Rött-före).
//
// RED BEFORE this slice: a besieging force that annihilated a settlement's
// garrison ended the battle with termination_reason="annihilation" and did
// NOTHING to the settlement — battle.go's own header comment ("neither entry
// point performs settlement CAPTURE on an attacker win anymore ... a
// besieging or landing force can now only annihilate/rout a garrison; taking
// the city itself is unbuilt"). occupied_since_tick/occupant_id/
// recolonizable_after_tick did not exist (migration 117 introduces them —
// tests below could not even compile against the pre-slice schema).
//
// Reuses newSiegeFixture/mkSiegeGarrison/mkSiegeAttacker/runFieldArrival/
// loadBattleID/runBattleToEnd from battle_wall_test.go/battle_test.go (same
// package, same test DB conventions — DATABASE_URL against poleia_test).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// TestSiegeBattle_AttackerWinsOccupiesCity is S1's core proof: an attacker
// win against a settlement's garrison must NOT annex directly. It flips
// termination_reason to "attacker_reached_city", moves the settlement to
// state='occupied' under the winning army, and — the PO1 invariant — leaves
// owner_id UNTOUCHED (occupy is not annex).
func TestSiegeBattle_AttackerWinsOccupiesCity(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)
	garrisonID := mkSiegeGarrison(t, pool, f, 100)
	attackerUnitID := mkSiegeAttacker(t, pool, f, 3000, false)

	h := newArrivalHandler(pool, economy.NewWallDice())
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	fb := &fakeBroadcaster{}
	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, fb)
	runBattleToEnd(t, pool, battleH, f.worldID, battleID, 30)

	var garrisonStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, garrisonID).Scan(&garrisonStatus); err != nil {
		t.Fatalf("read garrison: %v", err)
	}
	if garrisonStatus != "disbanded" {
		t.Fatalf("garrison status = %q, want disbanded (fixture must fully wipe the garrison for this test to be meaningful)", garrisonStatus)
	}

	var reason string
	if err := pool.QueryRow(ctx, `SELECT termination_reason FROM battles WHERE id = $1`, battleID).Scan(&reason); err != nil {
		t.Fatalf("read battle: %v", err)
	}
	if reason != "attacker_reached_city" {
		t.Errorf("termination_reason = %q, want %q", reason, "attacker_reached_city")
	}

	var state string
	var ownerID, occupantID uuid.UUID
	var sinceTick *int
	if err := pool.QueryRow(ctx,
		`SELECT state, owner_id, occupant_id, occupied_since_tick FROM settlements WHERE id = $1`, f.defSettlement,
	).Scan(&state, &ownerID, &occupantID, &sinceTick); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if state != "occupied" {
		t.Errorf("settlement state = %q, want \"occupied\"", state)
	}
	if ownerID != f.defender {
		t.Errorf("settlement owner_id = %s, want UNCHANGED %s — occupy is not annex (PO1)", ownerID, f.defender)
	}
	if occupantID != f.attacker {
		t.Errorf("settlement occupant_id = %s, want %s", occupantID, f.attacker)
	}
	if sinceTick == nil {
		t.Errorf("occupied_since_tick is nil, want set")
	}

	var attStatus string
	var attSettlement uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT status, settlement_id FROM units WHERE id = $1`, attackerUnitID).Scan(&attStatus, &attSettlement); err != nil {
		t.Fatalf("read attacker unit: %v", err)
	}
	if attStatus != "garrison" || attSettlement != f.defSettlement {
		t.Errorf("winning attacker unit status=%q settlement_id=%s, want garrison at %s", attStatus, attSettlement, f.defSettlement)
	}

	found := false
	for _, k := range fb.notified {
		if k == "CityOccupied" {
			found = true
		}
	}
	if !found {
		t.Errorf("NotifyPlayer kinds = %v, want a \"CityOccupied\" notification", fb.notified)
	}
}

// TestBattle_RelievedOccupationResetsCounter is S2's "en attack nollar
// räknaren": the occupant's garrison successfully DEFENDS the already-
// occupied city against a new attacker — same occupant, but
// occupied_since_tick restarts and annex_ready_notified clears.
func TestBattle_RelievedOccupationResetsCounter(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)

	// Seed: the city is already occupied by f.attacker; owner stays f.defender.
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET state = 'occupied', occupant_id = $2, occupied_since_tick = 0 WHERE id = $1`,
		f.defSettlement, f.attacker,
	); err != nil {
		t.Fatalf("seed occupied settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 2000, 0, 'garrison', $3)`,
		f.worldID, f.attacker, f.defSettlement,
	); err != nil {
		t.Fatalf("seed occupying garrison: %v", err)
	}

	// A third party's relief force, too small to break the occupant's garrison.
	var reliefOwner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"relief-"+uuid.New().String(), "relief-"+uuid.New().String()+"@test.invalid",
	).Scan(&reliefOwner); err != nil {
		t.Fatalf("create relief player: %v", err)
	}
	var reliefUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 50, 0, 'marching', 0, 0, 1, 0, 'sack') RETURNING id`,
		f.worldID, reliefOwner,
	).Scan(&reliefUnitID); err != nil {
		t.Fatalf("create relief unit: %v", err)
	}

	h := newArrivalHandler(pool, economy.NewWallDice())
	runFieldArrival(t, pool, h, f.worldID, reliefUnitID)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, &fakeBroadcaster{})
	runBattleToEnd(t, pool, battleH, f.worldID, battleID, 30)

	var reason string
	if err := pool.QueryRow(ctx, `SELECT termination_reason FROM battles WHERE id = $1`, battleID).Scan(&reason); err != nil {
		t.Fatalf("read battle: %v", err)
	}
	if reason != "annihilation" {
		t.Fatalf("termination_reason = %q, want \"annihilation\" (fixture must fully wipe the small relief force for this test to be meaningful)", reason)
	}

	var state string
	var ownerID, occupantID uuid.UUID
	var sinceTick *int
	var annexNotified bool
	if err := pool.QueryRow(ctx,
		`SELECT state, owner_id, occupant_id, occupied_since_tick, annex_ready_notified FROM settlements WHERE id = $1`,
		f.defSettlement,
	).Scan(&state, &ownerID, &occupantID, &sinceTick, &annexNotified); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if state != "occupied" {
		t.Errorf("settlement state = %q, want still \"occupied\" (the occupant defended successfully)", state)
	}
	if occupantID != f.attacker {
		t.Errorf("occupant_id = %s, want UNCHANGED %s", occupantID, f.attacker)
	}
	if ownerID != f.defender {
		t.Errorf("owner_id = %s, want UNCHANGED %s", ownerID, f.defender)
	}
	if sinceTick == nil || *sinceTick <= 0 {
		t.Errorf("occupied_since_tick = %v, want RESET to a tick > 0 (the relief attack), not the seeded 0", sinceTick)
	}
	if annexNotified {
		t.Errorf("annex_ready_notified = true, want reset to false by the counter reset")
	}
}

// TestOccupationCheck_MaturesToAnnexReady is S2's maturity half: once the
// occupation has stood occupationTicksToAnnex ticks unchallenged,
// ScheduledOccupationCheck notifies the occupant and sets
// annex_ready_notified — exactly once (idempotent replay does not re-notify).
func TestOccupationCheck_MaturesToAnnexReady(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)

	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET state = 'occupied', occupant_id = $2, occupied_since_tick = 0 WHERE id = $1`,
		f.defSettlement, f.attacker,
	); err != nil {
		t.Fatalf("seed occupied settlement: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`, f.worldID, occupationTicksToAnnex); err != nil {
		t.Fatalf("advance world tick: %v", err)
	}

	fb := &fakeBroadcaster{}
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	h := NewOccupationCheckHandler(pool, sched, fb)

	raw, err := json.Marshal(OccupationCheckPayload{SettlementID: f.defSettlement})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: occupationTicksToAnnex, Payload: raw}); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var notified bool
	if err := pool.QueryRow(ctx, `SELECT annex_ready_notified FROM settlements WHERE id = $1`, f.defSettlement).Scan(&notified); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if !notified {
		t.Errorf("annex_ready_notified = false, want true after %d unchallenged ticks", occupationTicksToAnnex)
	}
	found := 0
	for _, k := range fb.notified {
		if k == "CityAnnexReady" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("CityAnnexReady notifications = %d, want exactly 1", found)
	}

	// Idempotent replay: firing the same check again must not re-notify.
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: occupationTicksToAnnex, Payload: raw}); err != nil {
		t.Fatalf("handle (replay): %v", err)
	}
	found = 0
	for _, k := range fb.notified {
		if k == "CityAnnexReady" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("CityAnnexReady notifications after replay = %d, want still exactly 1 (idempotent)", found)
	}
}

// TestExecuteOccupyAction_AnnexBeforeMaturityRejected proves the annex gate:
// even with a valid occupant, annex must fail before the counter matures.
func TestExecuteOccupyAction_AnnexBeforeMaturityRejected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET state = 'occupied', occupant_id = $2, occupied_since_tick = 0 WHERE id = $1`,
		f.defSettlement, f.attacker,
	); err != nil {
		t.Fatalf("seed occupied settlement: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)

	_, err := ExecuteOccupyAction(ctx, pool, sched, store, clk, &fakeBroadcaster{}, OccupyActionOrder{
		WorldID: f.worldID, PlayerID: f.attacker, SettlementID: f.defSettlement, Action: "annex",
	})
	if err == nil {
		t.Fatal("annex before maturity: want an error, got nil")
	}
}

// TestExecuteOccupyAction_AnnexAfterMaturity proves the annex outcome once
// the gate is satisfied: ownership transfers, occupied bookkeeping clears.
func TestExecuteOccupyAction_AnnexAfterMaturity(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET state = 'occupied', occupant_id = $2, occupied_since_tick = 0 WHERE id = $1`,
		f.defSettlement, f.attacker,
	); err != nil {
		t.Fatalf("seed occupied settlement: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`, f.worldID, occupationTicksToAnnex); err != nil {
		t.Fatalf("advance world tick: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	fb := &fakeBroadcaster{}

	res, err := ExecuteOccupyAction(ctx, pool, sched, store, clk, fb, OccupyActionOrder{
		WorldID: f.worldID, PlayerID: f.attacker, SettlementID: f.defSettlement, Action: "annex",
	})
	if err != nil {
		t.Fatalf("annex after maturity: %v", err)
	}
	if res.Action != "annex" {
		t.Errorf("result.Action = %q, want \"annex\"", res.Action)
	}

	var ownerID uuid.UUID
	var state string
	var occupantID *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id, state, occupant_id FROM settlements WHERE id = $1`, f.defSettlement).
		Scan(&ownerID, &state, &occupantID); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if ownerID != f.attacker {
		t.Errorf("owner_id = %s, want %s (annex transfers ownership)", ownerID, f.attacker)
	}
	if state != "active" {
		t.Errorf("state = %q, want \"active\"", state)
	}
	if occupantID != nil {
		t.Errorf("occupant_id = %v, want cleared", occupantID)
	}
}

// TestExecuteOccupyAction_Sack is S4: pop -⅓, top production building -1
// level, loot dispatched as a caravan, city STAYS with the defender (no
// ownership change).
func TestExecuteOccupyAction_Sack(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)

	const startPop = 900
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET state = 'occupied', occupant_id = $2, occupied_since_tick = 0, population = $3 WHERE id = $1`,
		f.defSettlement, f.attacker, startPop,
	); err != nil {
		t.Fatalf("seed occupied settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 1000, 0, 1000000, 0)`,
		f.defSettlement,
	); err != nil {
		t.Fatalf("seed settlement_goods: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'mine', 2)`,
		f.defSettlement,
	); err != nil {
		t.Fatalf("seed building: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	fb := &fakeBroadcaster{}

	res, err := ExecuteOccupyAction(ctx, pool, sched, store, clk, fb, OccupyActionOrder{
		WorldID: f.worldID, PlayerID: f.attacker, SettlementID: f.defSettlement, Action: "sack",
	})
	if err != nil {
		t.Fatalf("sack: %v", err)
	}
	if res.Action != "sack" {
		t.Errorf("result.Action = %q, want \"sack\"", res.Action)
	}

	var ownerID uuid.UUID
	var state string
	var occupantID *uuid.UUID
	var population int
	if err := pool.QueryRow(ctx,
		`SELECT owner_id, state, occupant_id, population FROM settlements WHERE id = $1`, f.defSettlement,
	).Scan(&ownerID, &state, &occupantID, &population); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if ownerID != f.defender {
		t.Errorf("owner_id = %s, want UNCHANGED %s — sack is not annex (S4)", ownerID, f.defender)
	}
	if state != "active" {
		t.Errorf("state = %q, want \"active\" (city stands, weakened)", state)
	}
	if occupantID != nil {
		t.Errorf("occupant_id = %v, want cleared", occupantID)
	}
	wantPop := startPop - startPop/3
	if population > wantPop+1 || population < wantPop-1 {
		t.Errorf("population = %d, want ~%d (start %d minus ⅓)", population, wantPop, startPop)
	}

	var buildingLevel int
	if err := pool.QueryRow(ctx, `SELECT level FROM buildings WHERE settlement_id = $1 AND building_type = 'mine'`, f.defSettlement).
		Scan(&buildingLevel); err != nil {
		t.Fatalf("read building: %v", err)
	}
	if buildingLevel != 1 {
		t.Errorf("mine level = %d, want 1 (decremented from 2)", buildingLevel)
	}

	var caravanCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transports WHERE origin_id = $1 AND kind = 'plunder'`, f.defSettlement,
	).Scan(&caravanCount); err != nil {
		t.Fatalf("read transports: %v", err)
	}
	if caravanCount != 1 {
		t.Errorf("plunder transports from settlement = %d, want 1", caravanCount)
	}

	var silverLeft float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`, f.defSettlement,
	).Scan(&silverLeft); err != nil {
		t.Fatalf("read remaining silver: %v", err)
	}
	if silverLeft != 500 {
		t.Errorf("silver remaining = %v, want 500 (1000 minus 50%% loot)", silverLeft)
	}
}

// TestExecuteOccupyAction_Burn is S5: sack's loot step plus razing —
// ownerless, blocked from recolonization until recolonizable_after_tick.
func TestExecuteOccupyAction_Burn(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)

	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET state = 'occupied', occupant_id = $2, occupied_since_tick = 0 WHERE id = $1`,
		f.defSettlement, f.attacker,
	); err != nil {
		t.Fatalf("seed occupied settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 1000, 0, 1000000, 0)`,
		f.defSettlement,
	); err != nil {
		t.Fatalf("seed settlement_goods: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	fb := &fakeBroadcaster{}

	res, err := ExecuteOccupyAction(ctx, pool, sched, store, clk, fb, OccupyActionOrder{
		WorldID: f.worldID, PlayerID: f.attacker, SettlementID: f.defSettlement, Action: "burn",
	})
	if err != nil {
		t.Fatalf("burn: %v", err)
	}
	if res.Action != "burn" {
		t.Errorf("result.Action = %q, want \"burn\"", res.Action)
	}

	var ownerID *uuid.UUID
	var state string
	var recolAfter *int
	if err := pool.QueryRow(ctx,
		`SELECT owner_id, state, recolonizable_after_tick FROM settlements WHERE id = $1`, f.defSettlement,
	).Scan(&ownerID, &state, &recolAfter); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if ownerID != nil {
		t.Errorf("owner_id = %v, want NULL — a burned city is ownerless", ownerID)
	}
	if state != "razed" {
		t.Errorf("state = %q, want \"razed\"", state)
	}
	if recolAfter == nil {
		t.Fatal("recolonizable_after_tick is nil, want set")
	}

	// RED BEFORE / GREEN AFTER for the karens itself (S5's other half): the
	// pre-flight colonize-existing-settlement check (march_start.go) must
	// block before the karens and allow after — replacing what used to be a
	// PERMANENT block for every razed row.
	var blockedBefore int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3
		   AND NOT (s.state = 'razed' AND s.recolonizable_after_tick IS NOT NULL
		            AND s.recolonizable_after_tick <= current_world_tick())`,
		f.worldID, 1, 0,
	).Scan(&blockedBefore); err != nil {
		t.Fatalf("check karens gate (before): %v", err)
	}
	if blockedBefore == 0 {
		t.Errorf("colonize existing-settlement count (before karens) = 0, want > 0 (still blocked)")
	}

	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`, f.worldID, *recolAfter); err != nil {
		t.Fatalf("advance world past karens: %v", err)
	}
	var blockedAfter int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3
		   AND NOT (s.state = 'razed' AND s.recolonizable_after_tick IS NOT NULL
		            AND s.recolonizable_after_tick <= current_world_tick())`,
		f.worldID, 1, 0,
	).Scan(&blockedAfter); err != nil {
		t.Fatalf("check karens gate (after): %v", err)
	}
	if blockedAfter != 0 {
		t.Errorf("colonize existing-settlement count (after karens) = %d, want 0 (recolonization now allowed)", blockedAfter)
	}
}
