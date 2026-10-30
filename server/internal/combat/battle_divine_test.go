package combat

// R2 battle, mild (Timothy 2026-10-30, megaron_religion_talforslag_20261030
// §R2): Favorable men fight 5 % stronger, Wrathful 5 % weaker. The mood is
// fixed per unit when it enters the battle and named in the outcome.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// fracDice: every Intn misses (never a 12), Float64 returns f and is counted.
type fracDice struct {
	f      float64
	floats int
}

func (d *fracDice) Intn(n int) int   { return 0 }
func (d *fracDice) Float64() float64 { d.floats++; return d.f }

func TestRollSide_GodsHandScalesTheDice(t *testing.T) {
	for _, c := range []struct {
		name   string
		size   int
		divine string
		f      float64
		dice   int
		floats int
	}{
		{"no hand: 60 dice, no roll drawn", 20, "", .0, 60, 0},
		{"favor: 60 → 63", 20, "favor", .99, 63, 1},
		{"wrath: 60 → 57", 20, "wrath", .99, 57, 1},
		{"favor on 9 dice, lucky fraction: 10", 3, "favor", .44, 10, 1},
		{"favor on 9 dice, unlucky fraction: 9", 3, "favor", .46, 9, 1},
	} {
		p := []battleParticipant{{utype: "elite_infantry", divine: c.divine}}
		d := &fracDice{f: c.f}
		_, dice, _ := rollSide(p, []int{c.size}, []int{0}, 1, d)
		if dice != c.dice || d.floats != c.floats {
			t.Errorf("%s: dice=%d floats=%d, want %d/%d", c.name, dice, d.floats, c.dice, c.floats)
		}
	}
}

func TestBattle_GodsHandFixedAtJoinAndNamedInOutcome(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)
	setKharis := func(player uuid.UUID, k float64) {
		if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,0,0)`,
			player, f.worldID, k); err != nil {
			t.Fatal(err)
		}
	}
	setKharis(f.attacker, 60)  // Favorable boundary
	setKharis(f.defender, 4.9) // Wrathful

	mkFieldDefender(t, pool, f, 10)
	attackerUnit := mkFieldAttacker(t, pool, f, 200)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{111, 222}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnit)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	divineOf := func(owner uuid.UUID) string {
		var d *string
		if err := pool.QueryRow(ctx, `SELECT divine FROM battle_participants WHERE battle_id=$1 AND owner_id=$2`, battleID, owner).Scan(&d); err != nil {
			t.Fatal(err)
		}
		if d == nil {
			return ""
		}
		return *d
	}
	if got := divineOf(f.attacker); got != "favor" {
		t.Fatalf("attacker divine = %q, want favor", got)
	}
	if got := divineOf(f.defender); got != "wrath" {
		t.Fatalf("defender divine = %q, want wrath", got)
	}

	// The mood is fixed at join: the attacker falling out of favour mid-battle
	// does not change the hand their men carry.
	if _, err := pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=30 WHERE player_id=$1 AND world_id=$2`, f.attacker, f.worldID); err != nil {
		t.Fatal(err)
	}
	fb := &fakeBroadcaster{}
	runBattleToEnd(t, pool, NewBattleTickHandler(pool, h.eventStore, h.scheduler, fb, h.clk), f.worldID, battleID, 50)
	if got := divineOf(f.attacker); got != "favor" {
		t.Fatalf("attacker divine after kharis change = %q, want favor", got)
	}
	// …and their dice still carry it: 200 spearmen → 180 active → 90 dice → ×1.05.
	var first BattleSideRoundResult
	if err := json.Unmarshal(loadBattleRounds(t, pool, battleID)[0].attacker, &first); err != nil {
		t.Fatal(err)
	}
	if first.DiceRolled < 94 || first.DiceRolled > 95 {
		t.Fatalf("attacker first-round dice = %d, want 94–95 (favor frozen at join)", first.DiceRolled)
	}

	want := map[string]string{"BattleWon": "favor", "BattleLost": "wrath"}
	for i, kind := range fb.notified {
		w, ok := want[kind]
		if !ok {
			continue
		}
		if got, _ := fb.payloads[i].(map[string]any)["divine"].(string); got != w {
			t.Errorf("%s divine = %q, want %q", kind, got, w)
		}
		delete(want, kind)
	}
	if len(want) != 0 {
		t.Fatalf("missing battle notices: %v (got %v)", want, fb.notified)
	}
}

func TestBattle_NoRealmRecordCarriesNoHand(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)
	mkFieldDefender(t, pool, f, 10)
	attackerUnit := mkFieldAttacker(t, pool, f, 200)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{111, 222}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnit)
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM battle_participants WHERE battle_id=$1 AND divine IS NOT NULL`,
		loadBattleID(t, pool, f.worldID, 1, 0)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d participants carry a hand without a realm record", n)
	}
}
