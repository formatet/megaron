package economy

// Fertility (bönesystemet v3 steg 5a): Potnia's blessing makes a covered city's people
// and herd grow half as fast again while it lasts. The blessing is read from
// prayer_blessings by economy's own SQL (economy may not import religion), so the
// effect name is pinned here against the string the prayer is stored under.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFertileBirths_RoundsDownAndNeverFewer(t *testing.T) {
	for born, want := range map[int]int{0: 0, 1: 1, 2: 3, 3: 4, 4: 6, 5: 7, 40: 60} {
		if got := FertileBirths(born); got != want {
			t.Errorf("FertileBirths(%d)=%d want %d", born, got, want)
		}
	}
}

func layFertility(t *testing.T, pool *pgxpool.Pool, f foodFixture, owner uuid.UUID, scope string, city *uuid.UUID, from, until int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,settlement_id,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		f.worldID, owner, FertilityEffect, scope, city, from, until); err != nil {
		t.Fatal(err)
	}
}

func ownerOf(t *testing.T, pool *pgxpool.Pool, city uuid.UUID) uuid.UUID {
	t.Helper()
	var o uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT owner_id FROM settlements WHERE id=$1`, city).Scan(&o); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestFertility_HerdCalvesHalfAgainMoreWhileItLasts(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "fert-herd", 100)
	seedFoodStock(t, pool, f, 100, 0, 40)
	layFertility(t, pool, f, ownerOf(t, pool, f.settlementID), "city", &f.settlementID, 4000, 4020)
	herdBirthsTick(t, pool, f, 284001, 4005)
	// four calves would be born; the blessing makes it six. Only the four bought ones cost grain.
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 46 {
		t.Fatalf("herd=%g want 46", got)
	}
	if got := foodStockOf(t, pool, f.settlementID, GoodGrain); got < 95.4 || got > 95.6 {
		t.Fatalf("grain=%g want 95.5 (four calves fed + the meal)", got)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM events WHERE stream_id=$1 AND event_type='HerdBirths' AND (payload->>'born')::int=6 AND (payload->>'fed')::int=4`, f.settlementID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("HerdBirths(born 6, fed 4) events=%d err=%v", n, err)
	}
}

func TestFertility_HerdUnchangedOutsideTheWindowOrForAnotherCityOrAnotherWanax(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "fert-herd-out", 100)
	seedFoodStock(t, pool, f, 100, 0, 40)
	owner := ownerOf(t, pool, f.settlementID)
	var otherProv, other uuid.UUID
	if err := pool.QueryRow(context.Background(), `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,5,5,'plains') RETURNING id`, f.worldID).Scan(&otherProv); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES($1,$2,'Neighbour','achaean',$3,'colony','active',100) RETURNING id`, f.worldID, otherProv, owner).Scan(&other); err != nil {
		t.Fatal(err)
	}
	var stranger uuid.UUID
	if err := pool.QueryRow(context.Background(), `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "fert-stranger-"+uuid.NewString()).Scan(&stranger); err != nil {
		t.Fatal(err)
	}
	layFertility(t, pool, f, owner, "realm", nil, 3990, 4005)    // ended on this very tick: until_tick > tick is false
	layFertility(t, pool, f, owner, "city", &other, 4000, 4020)  // another city
	layFertility(t, pool, f, stranger, "realm", nil, 4000, 4020) // another Wanax's whole realm
	layFertility(t, pool, f, owner, "realm", nil, 4006, 4020)    // not yet begun
	herdBirthsTick(t, pool, f, 284002, 4005)
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 44 {
		t.Fatalf("herd=%g want the plain 44", got)
	}
}

func TestFertility_RealmCoversEveryCityOfTheOwner(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "fert-realm", 100)
	seedFoodStock(t, pool, f, 100, 0, 40)
	layFertility(t, pool, f, ownerOf(t, pool, f.settlementID), "realm", nil, 4000, 4020)
	herdBirthsTick(t, pool, f, 284003, 4005)
	if got := foodStockOf(t, pool, f.settlementID, GoodLivestock); got != 46 {
		t.Fatalf("herd=%g want 46", got)
	}
}

// The status surface reads the same projection as the tick (next_citizen_growth_per_tick).
func TestFertility_StatusProjectionShowsTheBlessedGrowth(t *testing.T) {
	pool := testPool(t)
	f := newFoodFixture(t, pool, "fert-status", 2000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlements SET wall_level=3 WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',1000,50,1000000,$2)`, f.settlementID, f.tick); err != nil {
		t.Fatal(err)
	}
	plain, err := LoadNextCitizen(ctx, pool, f.settlementID)
	if err != nil {
		t.Fatal(err)
	}
	if plain.GrowthPerTick != 10 {
		t.Fatalf("plain growth %d want 10", plain.GrowthPerTick)
	}
	layFertility(t, pool, f, ownerOf(t, pool, f.settlementID), "city", &f.settlementID, 4000, 4020)
	blessed, err := LoadNextCitizen(ctx, pool, f.settlementID)
	if err != nil {
		t.Fatal(err)
	}
	if blessed.GrowthPerTick != 15 {
		t.Fatalf("blessed growth %d want 15 (x1.5)", blessed.GrowthPerTick)
	}
}
