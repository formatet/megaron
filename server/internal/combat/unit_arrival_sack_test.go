package combat

// Del 2b: sack/plunder vs annex. Conquest is now a CHOICE carried on the marching
// unit (capture_mode). These tests drive applyAttackerWins directly (same package)
// with a hand-built CombatResult so the outcome is deterministic — resolveCombat's
// fortune roll is random and irrelevant to what's under test here.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func TestApplyAttackerWins_SackLootsRazesAndDisbandsGarrison(t *testing.T) {
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
	defender := mkPlayer("victim")

	// Map: attacker capital at (0,0), defender's settlement at (1,0) — adjacent
	// plains so FindPath can route the plunder caravan home.
	for _, tl := range []struct {
		q, r    int
		terrain string
	}{{0, 0, "plains"}, {1, 0, "plains"}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var attCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&attCapProv); err != nil {
		t.Fatalf("create attacker capital province: %v", err)
	}
	var attCapitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Raider Home', 'achaean', $3, 'capital', true, 'active', 8000) RETURNING id`,
		worldID, attCapProv, attacker,
	).Scan(&attCapitalID); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}

	var defProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&defProv); err != nil {
		t.Fatalf("create defender province: %v", err)
	}
	var defSettlement uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, sitos_fund_silver)
		 VALUES ($1, $2, 'Doomed City', 'khemetiu', $3, 'capital', true, 'active', 9000, 100) RETURNING id`,
		worldID, defProv, defender,
	).Scan(&defSettlement); err != nil {
		t.Fatalf("create defender settlement: %v", err)
	}
	// Silver 1000 (flat 50% share) + tin 800 (weight 2 → 0.5/2 = 25% share).
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES
		   ($1, 'silver', 1000, 0, 100000, 0),
		   ($1, 'tin', 800, 0, 100000, 0)`,
		defSettlement,
	); err != nil {
		t.Fatalf("seed defender goods: %v", err)
	}
	// Defender garrison — must be disbanded by the sack regardless of loss rate.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 60, 0, 'garrison', $3)`,
		worldID, defender, defSettlement,
	); err != nil {
		t.Fatalf("create defender garrison: %v", err)
	}

	// The attacking unit — capture_mode defaults to 'sack' (mig 082).
	var attackerUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 1000, 0, 'marching', 0, 0, 'sack') RETURNING id`,
		worldID, attacker,
	).Scan(&attackerUnitID); err != nil {
		t.Fatalf("create attacker unit: %v", err)
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	u := unitRow{
		id: attackerUnitID, ownerID: attacker, utype: "spearman", category: "land",
		size: 1000, status: "marching", q: 0, r: 0, captureMode: "sack",
	}
	destOwnerID := defender
	dest := destSettlement{
		provinceID: defProv, settlementID: &defSettlement, ownerID: &destOwnerID,
		wallLevel: 0, terrain: "plains",
	}
	result := CombatResult{Outcome: OutcomeAttackerWins, DefenderLosses: 1.0}
	const attSizeAfter, attPopLost = 500, 100

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.applyAttackerWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, 1, 0, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("applyAttackerWins (sack): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// The settlement is razed, not occupied.
	var state string
	var ownerID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT state, owner_id FROM settlements WHERE id = $1`, defSettlement,
	).Scan(&state, &ownerID); err != nil {
		t.Fatalf("read sacked settlement: %v", err)
	}
	if state != "razed" {
		t.Errorf("state = %q, want \"razed\"", state)
	}
	if ownerID != nil {
		t.Errorf("owner_id = %v, want nil (razed = ownerless ruin)", ownerID)
	}

	// Province freed.
	var territoryState string
	var controllerID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT territory_state, controller_id FROM provinces WHERE id = $1`, defProv,
	).Scan(&territoryState, &controllerID); err != nil {
		t.Fatalf("read province: %v", err)
	}
	if territoryState != "free" || controllerID != nil {
		t.Errorf("province territory_state=%q controller=%v, want free/nil", territoryState, controllerID)
	}

	// Loot: silver 1000*0.5=500 (settlement_goods) + sitos fund 100*0.5=50 → 550;
	// tin 800*0.5/2=200.
	var silverLeft, tinLeft float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`, defSettlement,
	).Scan(&silverLeft); err != nil {
		t.Fatalf("read remaining silver: %v", err)
	}
	if silverLeft != 500 {
		t.Errorf("remaining silver = %v, want 500 (1000 - 50%% looted)", silverLeft)
	}
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'tin'`, defSettlement,
	).Scan(&tinLeft); err != nil {
		t.Fatalf("read remaining tin: %v", err)
	}
	if tinLeft != 600 {
		t.Errorf("remaining tin = %v, want 600 (800 - 25%% looted)", tinLeft)
	}
	var fundLeft float64
	if err := pool.QueryRow(ctx,
		`SELECT sitos_fund_silver FROM settlements WHERE id = $1`, defSettlement,
	).Scan(&fundLeft); err != nil {
		t.Fatalf("read sitos fund: %v", err)
	}
	if fundLeft != 50 {
		t.Errorf("remaining sitos fund = %v, want 50 (100 - 50%% looted)", fundLeft)
	}

	// A physical, interceptable plunder caravan was dispatched toward the attacker's capital.
	var transportID uuid.UUID
	var kind, category, tStatus string
	var tDestID *uuid.UUID
	var interceptable bool
	if err := pool.QueryRow(ctx,
		`SELECT id, kind, category, status, dest_id, interceptable FROM transports WHERE world_id = $1 AND owner_id = $2`,
		worldID, attacker,
	).Scan(&transportID, &kind, &category, &tStatus, &tDestID, &interceptable); err != nil {
		t.Fatalf("read plunder transport: %v", err)
	}
	if kind != "plunder" {
		t.Errorf("transport kind = %q, want \"plunder\"", kind)
	}
	if category != "land" {
		t.Errorf("transport category = %q, want \"land\"", category)
	}
	if tStatus != "in_transit" {
		t.Errorf("transport status = %q, want \"in_transit\"", tStatus)
	}
	if tDestID == nil || *tDestID != attCapitalID {
		t.Errorf("transport dest_id = %v, want attacker capital %s", tDestID, attCapitalID)
	}
	if !interceptable {
		t.Errorf("transport interceptable = false, want true")
	}
	manifest := map[string]float64{}
	rows, err := pool.Query(ctx, `SELECT good_key, quantity FROM transport_goods WHERE transport_id = $1`, transportID)
	if err != nil {
		t.Fatalf("read transport manifest: %v", err)
	}
	for rows.Next() {
		var good string
		var qty float64
		if scanErr := rows.Scan(&good, &qty); scanErr != nil {
			rows.Close()
			t.Fatalf("scan manifest row: %v", scanErr)
		}
		manifest[good] = qty
	}
	rows.Close()
	if manifest["silver"] != 550 {
		t.Errorf("manifest silver = %v, want 550 (500 settlement + 50 sitos fund)", manifest["silver"])
	}
	if manifest["tin"] != 200 {
		t.Errorf("manifest tin = %v, want 200", manifest["tin"])
	}

	// The defender's garrison dies with the city.
	var survivingGarrison int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM units WHERE settlement_id = $1 AND status = 'garrison'`, defSettlement,
	).Scan(&survivingGarrison); err != nil {
		t.Fatalf("count surviving garrison: %v", err)
	}
	if survivingGarrison != 0 {
		t.Errorf("surviving garrison units = %d, want 0 (disbanded with the razed city)", survivingGarrison)
	}

	// The attacker's own unit stands positioned on the battlefield hex, not
	// garrisoned in a city that no longer exists.
	var attStatus string
	var attSettlement *uuid.UUID
	var attSize int
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id, size FROM units WHERE id = $1`, attackerUnitID,
	).Scan(&attStatus, &attSettlement, &attSize); err != nil {
		t.Fatalf("read attacker unit: %v", err)
	}
	if attStatus != "positioned" || attSettlement != nil {
		t.Errorf("attacker status=%q settlement=%v, want positioned/nil", attStatus, attSettlement)
	}
	if attSize != attSizeAfter {
		t.Errorf("attacker size = %d, want %d (its own combat losses only, sack applies no further reduction)", attSize, attSizeAfter)
	}
}

// TestApplyAttackerWins_AnnexDoesNotDoublePunishAttackerGarrison is the regression
// guard for the latent bug found during the Del 2b build (todo ⚰️ finding #6): the
// annex branch used to place the attacker as garrison BEFORE applying defender
// losses, so applyDefenderUnitLosses (which filters only by settlement_id, not
// owner) also struck the attacker's own newly-placed row on top of the
// AttackerLosses it already took in resolveCombat. Fixed by moving
// applyDefenderUnitLosses before the attacker is placed as garrison.
func TestApplyAttackerWins_AnnexDoesNotDoublePunishAttackerGarrison(t *testing.T) {
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
	attacker := mkPlayer("annexer")
	defender := mkPlayer("annexed")

	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains'), ($1, 1, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("insert map tiles: %v", err)
	}

	var attCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&attCapProv); err != nil {
		t.Fatalf("create attacker capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Annexer Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		worldID, attCapProv, attacker,
	); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}

	var defProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&defProv); err != nil {
		t.Fatalf("create defender province: %v", err)
	}
	var defSettlement uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Annexed City', 'khemetiu', $3, 'capital', true, 'active', 9000) RETURNING id`,
		worldID, defProv, defender,
	).Scan(&defSettlement); err != nil {
		t.Fatalf("create defender settlement: %v", err)
	}
	// Defender garrison survives the fight at 50% loss — enough left that the
	// pre-fix bug (attacker's own garrison row also hit by this rate) is observable.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 60, 0, 'garrison', $3)`,
		worldID, defender, defSettlement,
	); err != nil {
		t.Fatalf("create defender garrison: %v", err)
	}

	var attackerUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', 1000, 0, 'marching', 0, 0, 'annex') RETURNING id`,
		worldID, attacker,
	).Scan(&attackerUnitID); err != nil {
		t.Fatalf("create attacker unit: %v", err)
	}

	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	u := unitRow{
		id: attackerUnitID, ownerID: attacker, utype: "spearman", category: "land",
		size: 1000, status: "marching", q: 0, r: 0, captureMode: "annex",
	}
	destOwnerID := defender
	dest := destSettlement{
		provinceID: defProv, settlementID: &defSettlement, ownerID: &destOwnerID,
		wallLevel: 0, terrain: "plains",
	}
	// DefenderLosses = 0.5 is the value under test: pre-fix, the attacker's own
	// garrison row (already reduced to attSizeAfter by resolveCombat) would ALSO be
	// cut by this 50% when applyDefenderUnitLosses ran after garrisoning.
	result := CombatResult{Outcome: OutcomeAttackerWins, DefenderLosses: 0.5}
	const attSizeAfter, attPopLost = 700, 300

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.applyAttackerWins(ctx, tx, u, dest, attSizeAfter, attPopLost, result, 1, 0, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("applyAttackerWins (annex): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Annex is unchanged: settlement taken, attacker becomes garrison.
	var newOwner uuid.UUID
	var isCapital bool
	if err := pool.QueryRow(ctx,
		`SELECT owner_id, is_capital FROM settlements WHERE id = $1`, defSettlement,
	).Scan(&newOwner, &isCapital); err != nil {
		t.Fatalf("read annexed settlement: %v", err)
	}
	if newOwner != attacker {
		t.Errorf("owner_id = %s, want attacker %s", newOwner, attacker)
	}
	if isCapital {
		t.Errorf("is_capital = true, want false (no Wanax may hold two capitals)")
	}

	// The regression assertion: the winner's garrison size equals attSizeAfter
	// exactly — NOT attSizeAfter further reduced by DefenderLosses.
	var attackerGarrisonSize int
	var attackerStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status, size FROM units WHERE id = $1`, attackerUnitID,
	).Scan(&attackerStatus, &attackerGarrisonSize); err != nil {
		t.Fatalf("read attacker unit after annex: %v", err)
	}
	if attackerStatus != "garrison" {
		t.Errorf("attacker status = %q, want \"garrison\"", attackerStatus)
	}
	if attackerGarrisonSize != attSizeAfter {
		t.Errorf("attacker garrison size = %d, want %d (double-punish bug: applyDefenderUnitLosses must not also reduce the winner)",
			attackerGarrisonSize, attSizeAfter)
	}

	// The actual defender garrison DID take its 50% loss (30 of 60 men) before
	// being evicted (evictDefeatedDefenders disbands ANY surviving non-conqueror
	// garrison after ownership transfer — a separate, correct mechanism, not the
	// bug under test here) — visible via the demographic cost applyDefenderUnitLosses
	// applies to the settlement's population (9000 - 30 lost = 8970).
	var defenderPop int
	if err := pool.QueryRow(ctx,
		`SELECT population FROM settlements WHERE id = $1`, defSettlement,
	).Scan(&defenderPop); err != nil {
		t.Fatalf("read defender settlement population: %v", err)
	}
	if defenderPop != 8970 {
		t.Errorf("defender population = %d, want 8970 (9000 - 30 men lost at 50%% of the 60-man garrison)", defenderPop)
	}
}
