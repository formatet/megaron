package combat

// Prayers of permanence (bönesystemet v3 steg 4). A blessing on one NAMED unit lasts for ever:
// strength rolls x1.25 long after an ordinary prayer ended, a ship sails every voyage at 3/4
// of the time, and Stout Hulls gives the ship +1 max hull (its own ceiling, per ship).

import (
	"context"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func blessForEver(t *testing.T, worldID, owner uuid.UUID, effect string, unit uuid.UUID) {
	t.Helper()
	if _, err := testPool(t).Exec(context.Background(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,from_tick,until_tick,permanent) VALUES($1,$2,$3,'unit',$4,0,$5,true)`,
		worldID, owner, effect, unit, religion.PermanentUntil); err != nil {
		t.Fatal(err)
	}
}

// Strength: the permanent unit rolls x1.25 a thousand game days after the prayer; the other side gets nothing.
func TestPermanentStrength_StillBoostedAThousandDaysLater(t *testing.T) {
	base := blessedBattle(t, nil)
	want := int(math.Round(float64(base.att.DiceRolled) * strengthLiteral))
	if want <= base.att.DiceRolled {
		t.Fatalf("fixture too small to show a boost: %d", base.att.DiceRolled)
	}
	got := blessedBattle(t, func(f siegeFixture, _ uuid.UUID, att, _ uuid.UUID) {
		if _, err := testPool(t).Exec(context.Background(), `UPDATE worlds SET current_tick=1010 WHERE id=$1`, f.worldID); err != nil {
			t.Fatal(err)
		}
		blessForEver(t, f.worldID, f.attacker, religion.EffectBlessedStrength, att)
		// An ordinary prayer cast at tick 0 on the defender would have ended 1000 days ago.
		bless(t, f, f.defender, religion.ScopeRealm, nil, nil, 0, 6)
	})
	if got.att.DiceRolled != want {
		t.Fatalf("permanent unit rolled %d dice, want %d (unblessed %d)", got.att.DiceRolled, want, base.att.DiceRolled)
	}
	if got.def.DiceRolled != base.def.DiceRolled {
		t.Fatalf("an ordinary prayer from tick 0 still boosted the defender at tick 1010: %d vs %d", got.def.DiceRolled, base.def.DiceRolled)
	}
}

// Speed: every voyage the named ship begins sails at 3/4 of the time; once, not twice per voyage.
func TestPermanentSpeed_EveryVoyageOfTheNamedShipSailsAtThreeQuarters(t *testing.T) {
	pool, world, owner, city := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1`, world); err != nil {
		t.Fatal(err)
	}
	for q := 0; q <= 12; q++ {
		terrain := "coastal_sea"
		if q == 0 {
			terrain = "plains"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, world, q, terrain); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	mkShip := func() uuid.UUID {
		var ship uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,settlement_id,support_settlement_id) VALUES($1,$2,'galley','naval',1,20,'garrison',0,0,$3,$3) RETURNING id`, world, owner, city).Scan(&ship); err != nil {
			t.Fatal(err)
		}
		return ship
	}
	named, plain := mkShip(), mkShip()
	blessForEver(t, world, owner, religion.EffectBlessedSpeedSea, named)
	seedDepartureFood(t, pool, city)
	sail := func(ship uuid.UUID) int {
		t.Helper()
		r, err := StartMarch(ctx, pool, sched, store, clk, MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 12, Intent: "patrol"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r.DurationTicks
	}
	if got := sail(plain); got != 4 { // 11 coastal hexes x 1/3 = 3.67 -> 4
		t.Fatalf("an unblessed ship sails in %d, want 4", got)
	}
	for voyage := 1; voyage <= 2; voyage++ {
		if got := sail(named); got != 3 { // x0.75 once: 3, not 2
			t.Fatalf("voyage %d of the blessed ship takes %d game days, want 3", voyage, got)
		}
		// Home again, a long time later.
		if _, err := pool.Exec(ctx, `UPDATE units SET status='garrison',settlement_id=$2,q=0,r=0,depart_tick=NULL,arrive_tick=NULL,march_route=NULL,target_q=NULL,target_r=NULL WHERE id=$1`, named, city); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=current_tick+500 WHERE id=$1`, world); err != nil {
			t.Fatal(err)
		}
		seedDepartureFood(t, pool, city)
	}
}

// Hull: a ship with hull_bonus 1 stands at 6 of 6; one storm takes 2 -> 4, and the notice says 4 of 6.
func TestPermanentHull_AStormTakesTwoOfSix(t *testing.T) {
	pool, world, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, world, owner, route)
	if _, err := pool.Exec(context.Background(), `UPDATE units SET hull_bonus=1, hull=6 WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	stormsOverHexes(t, pool, world, 1, 2)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(1)
	for day := 0; day <= 6; day++ {
		runStormDay(t, h, world, day)
	}
	if status, hull := shipState(t, pool, ship); status == "disbanded" || hull != 4 {
		t.Fatalf("status %s hull %d, want afloat at 4 (6 - one storm of %d)", status, hull, StormDamage)
	}
	got := stormPayloads(t, ship)
	if len(got) != 1 || got[0].HullBefore != 6 || got[0].Hull != 4 || got[0].HullMax != 6 {
		t.Fatalf("storm notice should read 6 -> 4 of 6: %+v", got)
	}
}

// The sinking rule is as before: three storms take 6 of 6, a ship at 0 founders.
func TestPermanentHull_StillSinksAtZero(t *testing.T) {
	pool, world, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, world, owner, route)
	if _, err := pool.Exec(context.Background(), `UPDATE units SET hull_bonus=1, hull=6 WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	runThreeStorms(t, world, nil)
	if status, _ := shipState(t, pool, ship); status != "disbanded" {
		t.Fatalf("a 6-hull ship met three storms (6 hull) and is %s, want sunk", status)
	}
}

// Battle dice scale with the ship's OWN ceiling: a full blessed ship rolls what a full ordinary ship rolls.
func TestPermanentHull_BattleDiceScaleWithTheShipsOwnCeiling(t *testing.T) {
	dice := func(hull, bonus int) int {
		p := []battleParticipant{{utype: "galley", hull: hull, hullBonus: bonus}}
		_, total, _ := rollSide(p, []int{20}, []int{0}, 1, &fracDice{})
		return total
	}
	if dice(6, 1) != dice(5, 0) {
		t.Fatalf("a full 6/6 ship rolls %d dice, a full 5/5 ship %d: it must be the same", dice(6, 1), dice(5, 0))
	}
	if dice(3, 1) >= dice(5, 0) {
		t.Fatalf("a 3/6 ship should roll fewer dice than a full one: %d vs %d", dice(3, 1), dice(5, 0))
	}
	// 3/6 is half a hull, 3/5 is three fifths: the denominators differ per ship.
	if dice(3, 1) >= dice(3, 0) {
		t.Fatalf("3/6 (%d dice) must roll fewer than 3/5 (%d dice)", dice(3, 1), dice(3, 0))
	}
}

// A battle draws absolute hull points; the cargo's loss is the share of THE SHIP'S hull it lost.
func TestPermanentHull_CargoLossIsTheShareOfTheShipsOwnHull(t *testing.T) {
	if got := cargoSizeAfterHullLossOf(60, 5, 6); got != 10 { // lost 5 of 6: floor(60*5/6)=50 gone
		t.Fatalf("lost 5 of 6 hull, cargo left %d, want 10", got)
	}
	if got := cargoSizeAfterHullLossOf(60, 6, 6); got != 0 {
		t.Fatalf("sunk ship keeps cargo %d", got)
	}
	if got := cargoSizeAfterHullLoss(60, 3); got != cargoSizeAfterHullLossOf(60, 3, hullMax) {
		t.Fatalf("the plain ship's mapping changed")
	}
}

func TestHullCap(t *testing.T) {
	if HullCap(0) != 5 || HullCap(1) != 6 {
		t.Fatalf("HullCap: %d %d", HullCap(0), HullCap(1))
	}
}
