package combat

import (
	"context"
	"math"
	"testing"
)

func TestAlphaWallsRealDefenderStrengthAndOwnerExpiry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	run := func(flag string) (att, def BattleSideRoundResult) {
		t.Helper()
		f := newSiegeFixture(t, pool, 0)
		garrison := mkSiegeGarrison(t, pool, f, 3000)
		attacker := mkSiegeAttacker(t, pool, f, 3000, false)
		if flag != "none" {
			owner := f.defender
			until := 6
			if flag == "wrong owner" {
				owner = f.attacker
			}
			if flag == "expired" {
				until = 0
			}
			if _, err := pool.Exec(ctx, `UPDATE settlements SET garrison_defense_owner_id=$2,garrison_defense_until_tick=$3 WHERE id=$1`, f.defSettlement, owner, until); err != nil {
				t.Fatal(err)
			}
		}
		h := newArrivalHandler(pool, &sequenceDice{ints: []int{55, 2029, 7717, 40961}})
		runFieldArrival(t, pool, h, f.worldID, attacker)
		battle := loadBattleID(t, pool, f.worldID, 1, 0)
		if flag == "away from city" {
			_, err := pool.Exec(ctx, `UPDATE units SET settlement_id=NULL WHERE id=$1`, garrison)
			if err != nil {
				t.Fatal(err)
			}
		}
		bh := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
		runOneBattleTick(t, pool, bh, f.worldID, battle, 1)
		return loadRoundSideResult(t, pool, battle, 1, 1, "attacker"), loadRoundSideResult(t, pool, battle, 1, 1, "defender")
	}
	baseAtt, base := run("none")
	boostAtt, boost := run("active")
	if boost.DiceRolled != int(math.Round(float64(base.DiceRolled)*1.25)) {
		t.Fatalf("Walls did not strengthen real city defenders: base %d boosted %d", base.DiceRolled, boost.DiceRolled)
	}
	if boostAtt.DiceRolled != baseAtt.DiceRolled {
		t.Fatal("Walls strengthened the attacker")
	}
	for _, flag := range []string{"wrong owner", "expired", "away from city"} {
		_, got := run(flag)
		if got.DiceRolled != base.DiceRolled {
			t.Fatalf("Walls escaped owner/expiry/garrison scope (%s): got%d want%d", flag, got.DiceRolled, base.DiceRolled)
		}
	}
}
