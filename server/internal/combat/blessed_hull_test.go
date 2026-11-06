package combat

// The Sea Daimons' "Stout Hulls" (bönesystemet v3 steg 2a): a blessed ship takes
// ONE storm unharmed, then the next in full. The ward is read before the gods'
// spare/wrath die and replaces it (no die is drawn). Only the owner's ships.

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

type countingGodDice struct{ floats int }

func (d *countingGodDice) Intn(n int) int   { return 0 }
func (d *countingGodDice) Float64() float64 { d.floats++; return .99 }

func stormBless(t *testing.T, worldID, owner uuid.UUID, scope string, unit *uuid.UUID, from, until int) {
	t.Helper()
	pool := testPool(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		worldID, owner, religion.EffectBlessedHull, scope, unit, from, until); err != nil {
		t.Fatal(err)
	}
}

func runThreeStorms(t *testing.T, worldID uuid.UUID, god *countingGodDice) {
	t.Helper()
	pool := testPool(t)
	// Lane groups 1,2 | 4,5 | 7 are three storms (see TestSeaStorm_OneBlowPerStormThenFounders).
	stormsOverHexes(t, pool, worldID, 1, 2, 4, 5, 7)
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(1)
	if god != nil {
		h.GodDice = god
	}
	for day := 0; day <= 9; day++ {
		runStormDay(t, h, worldID, day)
	}
}

func stormPayloads(t *testing.T, ship uuid.UUID) []SeaStormPayload {
	t.Helper()
	rows, err := testPool(t).Query(context.Background(), `SELECT payload FROM events WHERE stream_id=$1 AND event_type IN ('ShipStormDamaged','ShipFoundered','ShipStormSpared') ORDER BY id`, ship)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []SeaStormPayload
	for rows.Next() {
		var raw []byte
		var p SeaStormPayload
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func TestBlessedHull_TakesOneStormThenTheNextInFull(t *testing.T) {
	pool, world, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, world, owner, route)
	stormBless(t, world, owner, religion.ScopeUnit, &ship, 0, 20)
	runThreeStorms(t, world, nil)
	// Unwarded, three blows sink a full hull; with the ward she is left with 1.
	if status, hull := shipState(t, pool, ship); status == "disbanded" || hull != 1 {
		t.Fatalf("status %s hull %d, want afloat with 1 (one storm warded, two in full)", status, hull)
	}
	got := stormPayloads(t, ship)
	if len(got) != 3 || got[0].Divine != "warded" || got[0].Damage != 0 || got[0].Hull != 5 {
		t.Fatalf("first storm must be warded with no damage: %+v", got)
	}
	if got[1].Damage != StormDamage || got[2].Damage != StormDamage || got[1].Divine == "warded" || got[2].Divine == "warded" {
		t.Fatalf("the ward is spent after one storm: %+v", got)
	}
	var kind string
	if err := pool.QueryRow(context.Background(), `SELECT event_type FROM events WHERE stream_id=$1 AND payload->>'divine'='warded'`, ship).Scan(&kind); err != nil || kind != EventShipStormDamaged {
		t.Fatalf("warded notice kind %q %v, want ShipStormDamaged", kind, err)
	}
}

// A realm blessing gives each ship its own ward; a foreign owner's blessing gives none.
func TestBlessedHull_RealmWardsEachShipAndOnlyTheOwners(t *testing.T) {
	pool, world, owner, route := seaStormWorld(t)
	a, _ := mkStormFleet(t, pool, world, owner, route)
	b, _ := mkStormFleet(t, pool, world, owner, route)
	var foreign uuid.UUID
	if err := pool.QueryRow(context.Background(), `INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`, "f-"+uuid.NewString()).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	c, _ := mkStormFleet(t, pool, world, foreign, route)
	stormBless(t, world, owner, religion.ScopeRealm, nil, 0, 20)
	runThreeStorms(t, world, nil)
	for _, ship := range []uuid.UUID{a, b} {
		if _, hull := shipState(t, pool, ship); hull != 1 {
			t.Fatalf("ship %s hull %d, want 1: each ship has its own ward", ship, hull)
		}
	}
	if status, _ := shipState(t, pool, c); status != "disbanded" {
		t.Fatalf("a foreign owner's ship was warded by my realm blessing (status %s)", status)
	}
}

// "warded" draws no die: the gods' hand is not consulted for a storm the ward takes.
func TestBlessedHull_WardDrawsNoDie(t *testing.T) {
	pool, world, owner, route := seaStormWorld(t)
	ship, _ := mkStormFleet(t, pool, world, owner, route)
	if _, err := pool.Exec(context.Background(), `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,90,0,0)`, owner, world); err != nil {
		t.Fatal(err)
	}
	stormBless(t, world, owner, religion.ScopeUnit, &ship, 0, 20)
	stormsOverHexes(t, pool, world, 1, 2)
	god := &countingGodDice{}
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice, h.GodDice = newStormDice(1), god
	for day := 0; day <= 3; day++ {
		runStormDay(t, h, world, day)
	}
	if god.floats != 0 {
		t.Fatalf("the ward still drew the gods' die %d times", god.floats)
	}
	if _, hull := shipState(t, pool, ship); hull != 5 {
		t.Fatalf("hull %d, want 5", hull)
	}
}

// A storm met before the prayer was said is not covered; Favourable Passage's own
// protection spends no ward.
func TestBlessedHull_NotBackwardsAndNotWhenProtected(t *testing.T) {
	t.Run("storm before the prayer", func(t *testing.T) {
		pool, world, owner, route := seaStormWorld(t)
		ship, _ := mkStormFleet(t, pool, world, owner, route)
		stormBless(t, world, owner, religion.ScopeUnit, &ship, 5, 20)
		runThreeStorms(t, world, nil)
		got := stormPayloads(t, ship)
		if len(got) != 3 || got[0].Divine == "warded" || got[0].Damage != StormDamage || got[2].Divine != "warded" {
			t.Fatalf("the ward must go to the first storm met after the prayer (tick>=5), not the first one ever: %+v", got)
		}
	})
	t.Run("protected voyage keeps the ward", func(t *testing.T) {
		pool, world, owner, route := seaStormWorld(t)
		ship, _ := mkStormFleet(t, pool, world, owner, route)
		if _, err := pool.Exec(context.Background(), `UPDATE units SET stillness_depart_tick=0,stillness_arrive_tick=10,stillness_owner_id=$2 WHERE id=$1`, ship, owner); err != nil {
			t.Fatal(err)
		}
		stormBless(t, world, owner, religion.ScopeUnit, &ship, 0, 20)
		runThreeStorms(t, world, nil)
		if _, hull := shipState(t, pool, ship); hull != 5 {
			t.Fatalf("hull %d, want 5 (Stillness protected)", hull)
		}
		var used int
		_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM prayer_blessing_wards_used WHERE unit_id=$1`, ship).Scan(&used)
		if used != 0 {
			t.Fatalf("a protected voyage spent %d wards", used)
		}
	})
}
