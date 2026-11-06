package combat

// Velchanos, "The Young God's Arm" (bönesystemet v3 steg 2a): a blessed LAND unit
// rolls religion.BlessedStrengthFactor more dice, attacking or defending, read
// afresh every battle tick. Only the owner's own units can be covered.

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

// strengthLiteral is Velchanos' x1.25 written out: the test must not follow the constant it checks.
const strengthLiteral = 1.25

type strengthRun struct {
	att, def BattleSideRoundResult
}

// blessedBattle runs one siege tick (world tick 10) after prep(...) laid whatever blessings it wants.
func blessedBattle(t *testing.T, prep func(f siegeFixture, attackerCity uuid.UUID, attacker, garrison uuid.UUID)) strengthRun {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)
	garrison := mkSiegeGarrison(t, pool, f, 3000)
	attacker := mkSiegeAttacker(t, pool, f, 3000, false)
	var attCity uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id=$1 AND name='Attacker Home'`, f.worldID).Scan(&attCity); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=10 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	// The marching attacker is the attacker city's, NOT standing in it (settlement_id is NULL on a march).
	if _, err := pool.Exec(ctx, `UPDATE units SET support_settlement_id=$2 WHERE id=$1`, attacker, attCity); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET support_settlement_id=$2 WHERE id=$1`, garrison, f.defSettlement); err != nil {
		t.Fatal(err)
	}
	if prep != nil {
		prep(f, attCity, attacker, garrison)
	}
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{55, 2029, 7717, 40961}})
	runFieldArrival(t, pool, h, f.worldID, attacker)
	battle := loadBattleID(t, pool, f.worldID, 1, 0)
	bh := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
	runOneBattleTick(t, pool, bh, f.worldID, battle, 1)
	return strengthRun{loadRoundSideResult(t, pool, battle, 1, 1, "attacker"), loadRoundSideResult(t, pool, battle, 1, 1, "defender")}
}

func bless(t *testing.T, f siegeFixture, owner uuid.UUID, scope string, unit, city *uuid.UUID, from, until int) {
	t.Helper()
	pool := testPool(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,settlement_id,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		f.worldID, owner, religion.EffectBlessedStrength, scope, unit, city, from, until); err != nil {
		t.Fatal(err)
	}
}

func TestBlessedStrength_ScopesOwnerAndExpiry(t *testing.T) {
	base := blessedBattle(t, nil)
	boosted := int(math.Round(float64(base.att.DiceRolled) * strengthLiteral))
	boostedDef := int(math.Round(float64(base.def.DiceRolled) * strengthLiteral))
	if boosted <= base.att.DiceRolled {
		t.Fatalf("fixture too small to show a boost: %d", base.att.DiceRolled)
	}
	cases := []struct {
		name         string
		prep         func(f siegeFixture, attCity uuid.UUID, att, gar uuid.UUID)
		wantAtt, def int
	}{
		{"unit scope on the attacker", func(f siegeFixture, _ uuid.UUID, att, _ uuid.UUID) {
			bless(t, f, f.attacker, religion.ScopeUnit, &att, nil, 0, 16)
		}, boosted, base.def.DiceRolled},
		{"city scope covers a unit out on a march (support_settlement_id)", func(f siegeFixture, attCity uuid.UUID, _, _ uuid.UUID) {
			bless(t, f, f.attacker, religion.ScopeCity, nil, &attCity, 0, 16)
		}, boosted, base.def.DiceRolled},
		{"realm scope", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			bless(t, f, f.attacker, religion.ScopeRealm, nil, nil, 0, 16)
		}, boosted, base.def.DiceRolled},
		{"defending garrison, realm scope: attack and defence alike", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			bless(t, f, f.defender, religion.ScopeRealm, nil, nil, 0, 16)
		}, base.att.DiceRolled, boostedDef},
		{"another city's blessing covers nobody here", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			other := f.defSettlement
			bless(t, f, f.attacker, religion.ScopeCity, nil, &other, 0, 16)
		}, base.att.DiceRolled, base.def.DiceRolled},
		{"a foreign owner's blessing on my unit does nothing", func(f siegeFixture, _ uuid.UUID, att, _ uuid.UUID) {
			bless(t, f, f.defender, religion.ScopeUnit, &att, nil, 0, 16)
			bless(t, f, f.defender, religion.ScopeRealm, nil, nil, 0, 16) // covers the defender's own, not the attacker's
		}, base.att.DiceRolled, boostedDef},
		{"expired (until_tick = now)", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			bless(t, f, f.attacker, religion.ScopeRealm, nil, nil, 0, 10)
		}, base.att.DiceRolled, base.def.DiceRolled},
		{"not yet cast (from_tick in the future)", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			bless(t, f, f.attacker, religion.ScopeRealm, nil, nil, 11, 20)
		}, base.att.DiceRolled, base.def.DiceRolled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := blessedBattle(t, c.prep)
			if got.att.DiceRolled != c.wantAtt || got.def.DiceRolled != c.def {
				t.Fatalf("attacker dice %d want %d · defender dice %d want %d", got.att.DiceRolled, c.wantAtt, got.def.DiceRolled, c.def)
			}
		})
	}
}

// A prayer said in the middle of a battle works from the next battle tick, not before.
func TestBlessedStrength_MidBattleFromTheNextTick(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	run := func(blessBetween bool) (first, second BattleSideRoundResult) {
		f := newSiegeFixture(t, pool, 0)
		mkSiegeGarrison(t, pool, f, 3000)
		attacker := mkSiegeAttacker(t, pool, f, 3000, false)
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=10 WHERE id=$1`, f.worldID); err != nil {
			t.Fatal(err)
		}
		h := newArrivalHandler(pool, &sequenceDice{ints: []int{55, 2029, 7717, 40961}})
		runFieldArrival(t, pool, h, f.worldID, attacker)
		battle := loadBattleID(t, pool, f.worldID, 1, 0)
		bh := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
		runOneBattleTick(t, pool, bh, f.worldID, battle, 1)
		if blessBetween {
			bless(t, f, f.attacker, religion.ScopeRealm, nil, nil, 10, 16)
		}
		runOneBattleTick(t, pool, bh, f.worldID, battle, 2)
		return loadRoundSideResult(t, pool, battle, 1, 1, "attacker"), loadRoundSideResult(t, pool, battle, 2, 1, "attacker")
	}
	plain1, plain2 := run(false)
	b1, b2 := run(true)
	if b1.DiceRolled != plain1.DiceRolled {
		t.Fatalf("the prayer reached back into the tick before it was said: %d vs %d", b1.DiceRolled, plain1.DiceRolled)
	}
	if want := int(math.Round(float64(plain2.DiceRolled) * strengthLiteral)); b2.DiceRolled != want {
		t.Fatalf("next tick not strengthened: %d want %d (unblessed %d)", b2.DiceRolled, want, plain2.DiceRolled)
	}
}

// Ships are not troops: a strength blessing leaves naval dice alone.
func TestBlessedStrength_NotForShips(t *testing.T) {
	p := []battleParticipant{{utype: "galley", hull: hullMax, blessedStrength: true}}
	q := []battleParticipant{{utype: "galley", hull: hullMax}}
	_, blessed, _ := rollSide(p, []int{20}, []int{0}, 1, &fracDice{})
	_, plain, _ := rollSide(q, []int{20}, []int{0}, 1, &fracDice{})
	if blessed != plain {
		t.Fatalf("naval dice changed by a land blessing: %d vs %d", blessed, plain)
	}
}
