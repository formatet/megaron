package combat

// A man is counted exactly once — either in settlements.population or in a
// living unit's size/crew (megaron_plan_disband_returnerar_folket.md §2's
// invariant, made true in the disband direction by 6e65b0a on 2026-09-01).
//
// RED BEFORE this slice: BattleTickHandler charged every combat loss a SECOND
// time against the owner's CAPITAL — `UPDATE settlements SET population =
// GREATEST(50, population - popLost) WHERE owner_id = $1 AND is_capital =
// true` (battle.go:657-664). Those men had already left population at
// recruitment (C2, 52fa1c5, 2026-06-15), so the deduction killed an equal
// number of unrelated civilians, in the capital, no matter which settlement
// raised the unit or where the battle was fought. It was Variant B's
// (c618974, 2026-06-05) leftover — introduced when the army did NOT cost
// population, never revisited when C2 made the draft physical. Same root as
// the disband bug, opposite sign.
//
// The player-facing loss report is unaffected: the stridsrapport's pop_lost
// is size_before - size_after (battle.go's battleReportUnit,
// battle_notify_test.go:167) — men lost from the UNIT, which is what actually
// happened. Civilian deaths from war have their own, deliberate home: the
// sack (occupation.go's sackPopLossFraction), which is untouched.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestBattleTick_CombatLossesDoNotDeductSettlementPopulation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)

	// Evenly matched, same fixture/seed shape as
	// TestBattleTick_MultiTickBattleTerminates — a fight that actually bleeds
	// both sides, so the assertion below is about a real loss and not about a
	// battle where nobody died.
	defenderUnitID := mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)

	popOf := func(owner uuid.UUID) int {
		t.Helper()
		var pop int
		if err := pool.QueryRow(ctx,
			`SELECT population FROM settlements
			  WHERE owner_id = $1 AND world_id = $2 AND is_capital = true`,
			owner, f.worldID,
		).Scan(&pop); err != nil {
			t.Fatalf("read capital population for %s: %v", owner, err)
		}
		return pop
	}
	sizeOf := func(unitID uuid.UUID) int {
		t.Helper()
		var size int
		if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, unitID).Scan(&size); err != nil {
			t.Fatalf("read unit size for %s: %v", unitID, err)
		}
		return size
	}

	attPopBefore, defPopBefore := popOf(f.attacker), popOf(f.defender)
	attSizeBefore, defSizeBefore := sizeOf(attackerUnitID), sizeOf(defenderUnitID)

	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
	runBattleToEnd(t, pool, battleH, f.worldID, battleID, 200)

	// Guard against a vacuous pass: if nobody bled, "population unchanged"
	// proves nothing. Both sides must have lost men for the real assertion to
	// mean anything.
	attLost := attSizeBefore - sizeOf(attackerUnitID)
	defLost := defSizeBefore - sizeOf(defenderUnitID)
	if attLost <= 0 || defLost <= 0 {
		t.Fatalf("fixture did not produce losses on both sides (attacker lost %d, defender lost %d) — "+
			"the population assertion below would be vacuous; adjust sizes/seed", attLost, defLost)
	}

	if got := popOf(f.attacker); got != attPopBefore {
		t.Errorf("attacker capital population = %d, want %d (unchanged) — the %d men lost in battle "+
			"left population at recruitment (C2); deducting them again kills unrelated civilians",
			got, attPopBefore, attLost)
	}
	if got := popOf(f.defender); got != defPopBefore {
		t.Errorf("defender capital population = %d, want %d (unchanged) — the %d men lost in battle "+
			"left population at recruitment (C2); deducting them again kills unrelated civilians",
			got, defPopBefore, defLost)
	}
}
