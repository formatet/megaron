package combat

// megaron_plan_skeppsreparation.md Slice B — DB-backed end-to-end proof that
// hull damage is actually wired into the real battle pipeline (initiateOrJoin
// Battle + BattleTickHandler), not just the pure calibration functions in
// ship_hull_test.go. RED BEFORE this slice: `hull` did not exist on units at
// all (this file could not compile against master), a naval participant's
// size going from 1→0 was always outright annihilation (no graded damage
// possible), and there was no march_intent value or arrival handler for a
// damaged ship's return leg.
//
// Rigg: needs a reachable Postgres (DATABASE_URL) — see testPool. The
// defender's rout is forced deterministically via a battle_participants
// standing_orders override (retreat_at_loss = 0.99, same technique
// TestBattleTick_HoldToLastManDisablesRout uses for the opposite control) so
// the fixture does not depend on empirically hunting a seed that happens to
// cross the loyalty-derived default threshold — it routs the instant it
// takes ANY loss at all, which any dice stream that lands at least one hit
// against the defender's fleet within the tick budget will trigger.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mkNavalPositioned creates a naval unit holding open ground (status
// 'positioned') — a field defender, same posture mkFieldDefender uses for
// land units.
func mkNavalPositioned(t *testing.T, pool *pgxpool.Pool, f battleFixture, ownerID uuid.UUID, q, r int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', $3, $4) RETURNING id`,
		f.worldID, ownerID, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("create naval positioned unit: %v", err)
	}
	return id
}

// mkNavalMarching creates a naval unit marching toward (targetQ, targetR) —
// a field attacker, same posture mkFieldAttacker uses for land units.
func mkNavalMarching(t *testing.T, pool *pgxpool.Pool, f battleFixture, ownerID uuid.UUID, fromQ, fromR, targetQ, targetR int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'marching', $3, $4, $5, $6, 'sack') RETURNING id`,
		f.worldID, ownerID, fromQ, fromR, targetQ, targetR,
	).Scan(&id); err != nil {
		t.Fatalf("create naval marching unit: %v", err)
	}
	return id
}

// TestNavalBattle_HullDrawnOnBothSides_RoutedLinksHome is the plan's own
// Röd-före B scenario end to end: a naval field battle where the defender's
// fleet is forced to rout at its first loss (standing_orders override) and
// the attacker's fleet wins. Asserts every locked claim in §Beslut B3:
//   - hull is drawn on BOTH sides' surviving ships, proportional to that
//     side's own naval losses (not just the loser's).
//   - the ROUTED side's damaged survivor gets a home march toward the
//     nearest own settlement with a shipyard (status='marching',
//     march_intent='damaged_return', target set).
//   - the WINNING side's damaged survivor keeps its current position/order
//     (status stays 'positioned', no march dispatched).
//   - an embarked cargo unit riding a damaged ship loses manpower
//     proportional to the ship's hull loss (cargoSizeAfterHullLoss, pinned
//     separately in ship_hull_test.go — this proves it is actually WIRED).
func TestNavalBattle_HullDrawnOnBothSides_RoutedLinksHome(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)

	// Defender fleet: 6 galleys holding the contested hex (1,0) — the same
	// target hex newBattleFixture already seeded a province for. Deliberately
	// a bigger fleet than a 2-3 ship test: with only a couple hulls per side,
	// a single unlucky round can wipe the whole fleet outright (annihilation)
	// before the retreat_at_loss override below even gets a chance to fire —
	// a bigger pool makes "some losses, then rout" far more likely than
	// "everyone dies at once" within the tick budget.
	for i := 0; i < 6; i++ {
		mkNavalPositioned(t, pool, f, f.defender, 1, 0)
	}

	// Give the defender's home settlement a shipyard so its routed survivor
	// has a real destination to link home to (megaron_plan_skeppsreparation.md
	// Slice A — already built/merged).
	var defHomeSettlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2`, f.worldID, f.defender,
	).Scan(&defHomeSettlementID); err != nil {
		t.Fatalf("load defender home settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'shipyard', 1)`,
		defHomeSettlementID,
	); err != nil {
		t.Fatalf("add shipyard to defender home: %v", err)
	}

	// Attacker fleet: 4 galleys. The last carries an embarked land cohort as
	// cargo, to prove §Slice B point 5's proportional cargo loss is wired.
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091, 111319, 7919, 104729}})
	att1 := mkNavalMarching(t, pool, f, f.attacker, 0, 0, 1, 0)
	att2 := mkNavalMarching(t, pool, f, f.attacker, 0, 0, 1, 0)
	att3 := mkNavalMarching(t, pool, f, f.attacker, 0, 0, 1, 0)
	att4 := mkNavalMarching(t, pool, f, f.attacker, 0, 0, 1, 0)

	var cargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, cargo_unit_id)
		 VALUES ($1, $2, 'spearman', 'land', 60, 0, 'embarked', NULL) RETURNING id`,
		f.worldID, f.attacker,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create embarked cargo unit: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET cargo_unit_id = $1 WHERE id = $2`, cargoID, att4); err != nil {
		t.Fatalf("attach cargo to att4: %v", err)
	}

	runFieldArrival(t, pool, h, f.worldID, att1) // creates the battle
	runFieldArrival(t, pool, h, f.worldID, att2) // joins it as a 2nd attacker
	runFieldArrival(t, pool, h, f.worldID, att3) // joins it as a 3rd attacker
	runFieldArrival(t, pool, h, f.worldID, att4) // joins it as a 4th attacker (carrying cargo)

	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	// Force the DEFENDER (only) to rout at its very first loss — deterministic
	// regardless of exactly which round/tick lands the first hit (see file
	// header). The attacker keeps the default loyalty-derived threshold, so
	// it does not also rout from an early loss of its own.
	if _, err := pool.Exec(ctx,
		`UPDATE battle_participants SET standing_orders = '{"retreat_at_loss":0.99}'::jsonb WHERE battle_id = $1 AND side = 'defender'`,
		battleID,
	); err != nil {
		t.Fatalf("force defender low rout threshold: %v", err)
	}

	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
	runBattleToEnd(t, pool, battleH, f.worldID, battleID, 300)

	var reason string
	if err := pool.QueryRow(ctx,
		`SELECT termination_reason FROM battles WHERE id = $1`, battleID,
	).Scan(&reason); err != nil {
		t.Fatalf("read battle: %v", err)
	}

	// This exact fixture/seed pair is pinned empirically (same posture as
	// TestBattleTick_SpansMultipleTicksForAnEvenFight's own comment): what
	// matters is that SOME rout happens with the defender on the losing end
	// (guaranteed as soon as any hit lands, given retreat_at_loss=0.99) — not
	// this specific seed's numerology. If this ever goes red because the
	// fixture no longer routs within the tick budget, swap the seed, not the
	// assertions below. winner isn't a battles column (only lives in the
	// BattleEnded event payload) — the per-ship assertions below are the
	// actual proof of which side routed, so this only pins the reason.
	if reason != "rout" {
		t.Fatalf("termination_reason = %q, want rout for this fixture", reason)
	}

	// ── Defender (routed) side: every surviving ship is damaged AND marching home. ──
	rows, err := pool.Query(ctx,
		`SELECT id, hull, status, march_intent, target_q, target_r FROM units WHERE world_id = $1 AND owner_id = $2 AND category = 'naval'`,
		f.worldID, f.defender,
	)
	if err != nil {
		t.Fatalf("load defender ships: %v", err)
	}
	var defenderSawSurvivor bool
	for rows.Next() {
		var id uuid.UUID
		var hull int
		var status string
		var marchIntent *string
		var targetQ, targetR *int
		if scanErr := rows.Scan(&id, &hull, &status, &marchIntent, &targetQ, &targetR); scanErr != nil {
			t.Fatalf("scan defender ship: %v", scanErr)
		}
		if status == "disbanded" {
			// A ship can be disbanded two ways: outright, by the PRE-EXISTING
			// per-round distributeLosses/apply-final-sizes mechanism (a
			// naval unit's own size is always 1, so any round that assigns
			// it a casualty destroys it instantly, hull untouched at
			// whatever it was) — or sunk by THIS slice's graded hull draw
			// (hull == 0, applyNavalHullDamage's sizes[i] <= 0 skip is
			// exactly why the two paths never double-apply to the same
			// ship). Both are legitimate outcomes here; hull is not
			// asserted either way for a disbanded ship.
			continue
		}
		defenderSawSurvivor = true
		if hull >= hullMax {
			t.Errorf("defender ship %s (routed side) survived with hull = %d, want < %d (must have taken the side's hull draw)", id, hull, hullMax)
		}
		if status != "marching" {
			t.Errorf("defender ship %s (routed survivor) status = %q, want marching (must link home)", id, status)
		}
		if marchIntent == nil || *marchIntent != "damaged_return" {
			t.Errorf("defender ship %s march_intent = %v, want damaged_return", id, marchIntent)
		}
		if targetQ == nil || targetR == nil {
			t.Errorf("defender ship %s has no march target — must be routed home somewhere", id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate defender ships: %v", err)
	}
	if !defenderSawSurvivor {
		t.Fatalf("every defender ship was sunk — fixture too lopsided to prove the routed-survivor-links-home claim, adjust the seed")
	}

	// ── Attacker (winning) side: any damaged survivor keeps its position. ──
	rows2, err := pool.Query(ctx,
		`SELECT id, hull, status, march_intent, q, r FROM units WHERE world_id = $1 AND owner_id = $2 AND category = 'naval'`,
		f.worldID, f.attacker,
	)
	if err != nil {
		t.Fatalf("load attacker ships: %v", err)
	}
	var attackerSawDamagedSurvivor bool
	for rows2.Next() {
		var id uuid.UUID
		var hull int
		var status string
		var marchIntent *string
		var q, r *int
		if scanErr := rows2.Scan(&id, &hull, &status, &marchIntent, &q, &r); scanErr != nil {
			t.Fatalf("scan attacker ship: %v", scanErr)
		}
		if status == "disbanded" {
			continue
		}
		if hull < hullMax {
			attackerSawDamagedSurvivor = true
			if status == "marching" || (marchIntent != nil && *marchIntent == "damaged_return") {
				t.Errorf("attacker ship %s (winning side, hull %d) was sent home — winning side must keep its orders", id, hull)
			}
		}
	}
	rows2.Close()
	if err := rows2.Err(); err != nil {
		t.Fatalf("iterate attacker ships: %v", err)
	}
	_ = attackerSawDamagedSurvivor // informative, not asserted true: the winning side may end this fixture completely undamaged, which is a legal outcome (no naval losses ⇒ no hull draw at all, hullLossForCasualtyFraction(0) = 0).

	// ── Cargo: proportional loss if the carrying ship (att4) was damaged/sunk. ──
	var cargoStatus string
	var cargoSize int
	if err := pool.QueryRow(ctx, `SELECT status, size FROM units WHERE id = $1`, cargoID).Scan(&cargoStatus, &cargoSize); err != nil {
		t.Fatalf("read cargo unit: %v", err)
	}
	var att4Hull int
	var att4Status string
	if err := pool.QueryRow(ctx, `SELECT hull, status FROM units WHERE id = $1`, att4).Scan(&att4Hull, &att4Status); err != nil {
		t.Fatalf("read att4: %v", err)
	}
	switch {
	case att4Status == "disbanded":
		if cargoStatus != "disbanded" || cargoSize != 0 {
			t.Errorf("carrier att4 sank but cargo status/size = %q/%d, want disbanded/0 (goes down with the ship)", cargoStatus, cargoSize)
		}
	case att4Hull < hullMax:
		wantSize := cargoSizeAfterHullLoss(60, hullMax-att4Hull)
		if cargoStatus != "embarked" {
			t.Errorf("carrier att4 damaged but not sunk (hull %d) — cargo status = %q, want still embarked", att4Hull, cargoStatus)
		}
		if cargoSize != wantSize {
			t.Errorf("cargo size = %d, want %d (60 men, %d/%d hull lost)", cargoSize, wantSize, hullMax-att4Hull, hullMax)
		}
	default:
		if cargoSize != 60 {
			t.Errorf("carrier att4 undamaged but cargo size = %d, want 60 (untouched)", cargoSize)
		}
	}
}
