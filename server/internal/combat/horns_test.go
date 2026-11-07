package combat

// Asterion, "The Bull's Horns" (bönesystemet v3 steg 5b): a blessed LAND unit rolls
// religion.HornsDiceFactor more dice in the FIRST round of a battle only (round 1 of its first
// battle tick), attacking or defending, and the factor multiplies with Velchanos' strength.

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

// hornsLiteral is the x1.5 written out: the test must not follow the constant it checks.
const hornsLiteral = 1.5

// spearDice is what N spearmen roll (half a die each, rounded up), unblessed.
func spearDice(active int) int { return (active + 1) / 2 }

func blessEffect(t *testing.T, f siegeFixture, effect string, owner uuid.UUID, scope string, unit, city *uuid.UUID, from, until int) {
	t.Helper()
	if _, err := testPool(t).Exec(context.Background(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,settlement_id,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		f.worldID, owner, effect, scope, unit, city, from, until); err != nil {
		t.Fatal(err)
	}
}

type hornsRun struct {
	t1r1, t1r2, t2r1 [2]BattleSideRoundResult // [attacker, defender]
}

// hornsBattle runs two battle ticks (world tick 10) after prep laid whatever blessings it wants.
func hornsBattle(t *testing.T, prep func(f siegeFixture, attCity uuid.UUID, attacker, garrison uuid.UUID)) hornsRun {
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
	for _, q := range []string{`UPDATE worlds SET current_tick=10 WHERE id=$1`} {
		if _, err := pool.Exec(ctx, q, f.worldID); err != nil {
			t.Fatal(err)
		}
	}
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
	runOneBattleTick(t, pool, bh, f.worldID, battle, 2)
	get := func(tick, round int) [2]BattleSideRoundResult {
		return [2]BattleSideRoundResult{loadRoundSideResult(t, pool, battle, tick, round, "attacker"), loadRoundSideResult(t, pool, battle, tick, round, "defender")}
	}
	return hornsRun{get(1, 1), get(1, 2), get(2, 1)}
}

func hornsBoosted(active int) int { return int(math.Round(float64(spearDice(active)) * hornsLiteral)) }

func TestHorns_FirstRoundOnly(t *testing.T) {
	r := hornsBattle(t, func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
		blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeRealm, nil, nil, 0, 30)
	})
	if got, want := r.t1r1[0].DiceRolled, hornsBoosted(r.t1r1[0].ActiveCombatants); got != want || want <= spearDice(r.t1r1[0].ActiveCombatants) {
		t.Fatalf("first round: attacker dice %d want %d (plain %d)", got, want, spearDice(r.t1r1[0].ActiveCombatants))
	}
	if got, want := r.t1r1[1].DiceRolled, spearDice(r.t1r1[1].ActiveCombatants); got != want {
		t.Fatalf("first round: the unblessed defender rolls %d want %d", got, want)
	}
	for name, res := range map[string]BattleSideRoundResult{"second round": r.t1r2[0], "next tick, first round": r.t2r1[0]} {
		if got, want := res.DiceRolled, spearDice(res.ActiveCombatants); got != want {
			t.Fatalf("%s: horns must be spent, dice %d want %d", name, got, want)
		}
	}
}

func TestHorns_ScopesOwnerAndExpiry(t *testing.T) {
	cases := []struct {
		name               string
		prep               func(f siegeFixture, attCity uuid.UUID, att, gar uuid.UUID)
		attBoost, defBoost bool
	}{
		{"unit scope on the attacker", func(f siegeFixture, _ uuid.UUID, att, _ uuid.UUID) {
			blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeUnit, &att, nil, 0, 30)
		}, true, false},
		{"city scope covers a unit out on a march", func(f siegeFixture, attCity uuid.UUID, _, _ uuid.UUID) {
			blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeCity, nil, &attCity, 0, 30)
		}, true, false},
		{"the defending garrison, realm scope: horns defend too", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			blessEffect(t, f, religion.EffectBlessedHorns, f.defender, religion.ScopeRealm, nil, nil, 0, 30)
		}, false, true},
		{"another city's blessing covers nobody here", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			other := f.defSettlement
			blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeCity, nil, &other, 0, 30)
		}, false, false},
		{"a foreign owner's blessing on my unit does nothing", func(f siegeFixture, _ uuid.UUID, att, _ uuid.UUID) {
			blessEffect(t, f, religion.EffectBlessedHorns, f.defender, religion.ScopeUnit, &att, nil, 0, 30)
		}, false, false},
		{"expired", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeRealm, nil, nil, 0, 10)
		}, false, false},
		{"not yet cast", func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
			blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeRealm, nil, nil, 11, 30)
		}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := hornsBattle(t, c.prep)
			for i, boost := range []bool{c.attBoost, c.defBoost} {
				a := r.t1r1[i].ActiveCombatants
				want := spearDice(a)
				if boost {
					want = hornsBoosted(a)
				}
				if r.t1r1[i].DiceRolled != want {
					t.Fatalf("side %d first round dice %d want %d", i, r.t1r1[i].DiceRolled, want)
				}
			}
		})
	}
}

// The two blessings stack: both factors multiply in the first round (1.25 x 1.5).
func TestHorns_StackWithVelchanosStrength(t *testing.T) {
	r := hornsBattle(t, func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
		blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeRealm, nil, nil, 0, 30)
		blessEffect(t, f, religion.EffectBlessedStrength, f.attacker, religion.ScopeRealm, nil, nil, 0, 30)
	})
	a := r.t1r1[0].ActiveCombatants
	want := int(math.Round(math.Round(float64(spearDice(a))*1.25) * hornsLiteral))
	if r.t1r1[0].DiceRolled != want {
		t.Fatalf("first round with both: %d want %d", r.t1r1[0].DiceRolled, want)
	}
	if got, want := r.t1r2[0].DiceRolled, int(math.Round(float64(spearDice(r.t1r2[0].ActiveCombatants))*1.25)); got != want {
		t.Fatalf("second round keeps only the strength: %d want %d", got, want)
	}
}

// "First round" is read from the stored rounds, so a replay of the same tick is still first and
// every tick after it is not: whoever joins a battle later never gets the horns.
func TestHorns_FirstBattleTickIsReadFromTheStoredRounds(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)
	var battle uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO battles (world_id,q,r,started_tick,current_tick,status,seed,wall_level,storm) VALUES ($1,1,0,5,5,'active',1,0,false) RETURNING id`, f.worldID).Scan(&battle); err != nil {
		t.Fatal(err)
	}
	first := func(tick int) bool {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		ok, err := isFirstBattleTick(ctx, tx, battle, tick)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !first(6) {
		t.Fatal("a battle with no rounds yet: its tick is the first")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO battle_rounds(battle_id,tick_index,round_index,attacker,defender) VALUES ($1,6,1,'{}','{}'),($1,6,2,'{}','{}')`, battle); err != nil {
		t.Fatal(err)
	}
	if !first(6) {
		t.Fatal("a replay of the first tick is still the first tick")
	}
	if first(7) {
		t.Fatal("the tick after the first is not the first")
	}
}

// Ships are not troops.
func TestHorns_NotForShips(t *testing.T) {
	p := []battleParticipant{{utype: "galley", hull: hullMax, blessedHorns: true, hornsNow: true}}
	q := []battleParticipant{{utype: "galley", hull: hullMax}}
	_, blessed, _ := rollSide(p, []int{20}, []int{0}, 1, &fracDice{})
	_, plain, _ := rollSide(q, []int{20}, []int{0}, 1, &fracDice{})
	if blessed != plain {
		t.Fatalf("naval dice changed by the horns: %d vs %d", blessed, plain)
	}
}

// The outcome is recorded in the round result so a replay can see why the dice were more.
func TestHorns_TheRoundResultSaysSo(t *testing.T) {
	r := hornsBattle(t, func(f siegeFixture, _ uuid.UUID, _, _ uuid.UUID) {
		blessEffect(t, f, religion.EffectBlessedHorns, f.attacker, religion.ScopeRealm, nil, nil, 0, 30)
	})
	if !r.t1r1[0].BlessedHorns || r.t1r1[1].BlessedHorns || r.t1r2[0].BlessedHorns {
		t.Fatalf("blessed_horns flag: att r1 %v def r1 %v att r2 %v", r.t1r1[0].BlessedHorns, r.t1r1[1].BlessedHorns, r.t1r2[0].BlessedHorns)
	}
}
