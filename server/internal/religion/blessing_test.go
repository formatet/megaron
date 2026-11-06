package religion

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type blessFix struct {
	pool         *pgxpool.Pool
	world        uuid.UUID
	me, other    uuid.UUID
	cityA, cityB uuid.UUID
	marching     uuid.UUID // mine, supported by cityA, out on a march (settlement_id NULL)
	home         uuid.UUID // mine, supported by cityA, in the city
	elsewhere    uuid.UUID // mine, supported by cityB
	foreignInA   uuid.UUID // someone else's unit
	ship         uuid.UUID
	ship2        uuid.UUID
	foreignWorld uuid.UUID
}

func newBlessFix(t *testing.T) blessFix {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := blessFix{pool: pool}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES($1,'archived',100) RETURNING id`, "bless-"+uuid.NewString()).Scan(&f.world))
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, f.world) })
	for _, p := range []*uuid.UUID{&f.me, &f.other} {
		must(pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(p))
	}
	city := func(q int, dst *uuid.UUID) {
		var prov uuid.UUID
		must(pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,$2,0,'plains') RETURNING id`, f.world, q).Scan(&prov))
		must(pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type) VALUES($1,$2,'C','minoan',$3,'capital') RETURNING id`, f.world, prov, f.me).Scan(dst))
	}
	city(0, &f.cityA)
	city(1, &f.cityB)
	unitRow := func(owner uuid.UUID, typ, cat string, support *uuid.UUID, inCity *uuid.UUID, dst *uuid.UUID) {
		status := "marching"
		if inCity != nil {
			status = "garrison"
		}
		must(pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,settlement_id,support_settlement_id) VALUES($1,$2,$3,$4,10,0,$5,0,0,$6,$7) RETURNING id`,
			f.world, owner, typ, cat, status, inCity, support).Scan(dst))
	}
	unitRow(f.me, "infantry", "land", &f.cityA, nil, &f.marching)
	unitRow(f.me, "infantry", "land", &f.cityA, &f.cityA, &f.home)
	unitRow(f.me, "infantry", "land", &f.cityB, &f.cityB, &f.elsewhere)
	unitRow(f.other, "infantry", "land", &f.cityA, &f.cityA, &f.foreignInA)
	unitRow(f.me, "galley", "naval", &f.cityA, nil, &f.ship)
	unitRow(f.me, "galley", "naval", &f.cityA, nil, &f.ship2)
	return f
}

func (f blessFix) bless(t *testing.T, owner uuid.UUID, effect, scope string, unit, city *uuid.UUID, from, until int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,settlement_id,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		f.world, owner, effect, scope, unit, city, from, until).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f blessFix) blessed(t *testing.T, owner, unit uuid.UUID, effect string, tick int) bool {
	t.Helper()
	b, err := LoadBlessed(context.Background(), f.pool, owner, unit, effect, tick)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// One query answers all three scopes; the city scope reads the permanent
// support city, so a unit out on a march is still the city's.
func TestLoadBlessed_ThreeScopes(t *testing.T) {
	f := newBlessFix(t)
	e := EffectBlessedStrength
	f.bless(t, f.me, e, ScopeUnit, &f.home, nil, 100, 110)
	if !f.blessed(t, f.me, f.home, e, 105) || f.blessed(t, f.me, f.marching, e, 105) {
		t.Fatal("unit scope must cover exactly the named unit")
	}
	f.bless(t, f.me, e, ScopeCity, nil, &f.cityA, 100, 110)
	if !f.blessed(t, f.me, f.marching, e, 105) {
		t.Fatal("city scope must cover a unit of the city that is out on a march (support_settlement_id, not settlement_id)")
	}
	if f.blessed(t, f.me, f.elsewhere, e, 105) {
		t.Fatal("city scope leaked to another city's unit")
	}
	f.bless(t, f.me, e, ScopeRealm, nil, nil, 100, 110)
	if !f.blessed(t, f.me, f.elsewhere, e, 105) {
		t.Fatal("realm scope must cover every unit of the owner")
	}
	if f.blessed(t, f.me, f.home, EffectBlessedHull, 105) {
		t.Fatal("a strength blessing is not a hull blessing")
	}
}

func TestLoadBlessed_OnlyOwnUnitsAndOnlyWhileItLasts(t *testing.T) {
	f := newBlessFix(t)
	e := EffectBlessedStrength
	f.bless(t, f.me, e, ScopeRealm, nil, nil, 100, 110)
	f.bless(t, f.me, e, ScopeCity, nil, &f.cityA, 100, 110)
	// Someone else's unit supported by MY city, and my blessing asked for with the other's id.
	if f.blessed(t, f.me, f.foreignInA, e, 105) || f.blessed(t, f.other, f.foreignInA, e, 105) {
		t.Fatal("a blessing covered a foreign unit")
	}
	// Someone else's realm blessing does not cover my unit.
	g := newBlessFix(t)
	g.bless(t, g.other, e, ScopeRealm, nil, nil, 100, 110)
	if g.blessed(t, g.me, g.home, e, 105) {
		t.Fatal("a foreign owner's blessing covered my unit")
	}
	for tick, want := range map[int]bool{99: false, 100: true, 109: true, 110: false} {
		if got := f.blessed(t, f.me, f.home, e, tick); got != want {
			t.Fatalf("tick %d: blessed=%v want %v (from_tick<=tick<until_tick)", tick, got, want)
		}
	}
}

// A ward is spent once per (blessing, ship); a city blessing gives each ship its own.
func TestConsumeWard_OncePerShipPerBlessing(t *testing.T) {
	f := newBlessFix(t)
	ctx := context.Background()
	e := EffectBlessedHull
	f.bless(t, f.me, e, ScopeCity, nil, &f.cityA, 100, 112)
	spend := func(ship uuid.UUID) bool {
		ok, err := ConsumeWard(ctx, f.pool, f.me, ship, e, 105)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !spend(f.ship) {
		t.Fatal("first storm: the ward should be spent")
	}
	if spend(f.ship) {
		t.Fatal("the same blessing must not protect the same ship twice")
	}
	if !spend(f.ship2) {
		t.Fatal("city scope: the second ship has its own ward")
	}
	if spend(f.ship) || spend(f.ship2) {
		t.Fatal("both wards are spent")
	}
	if ok, _ := ConsumeWard(ctx, f.pool, f.me, f.ship, e, 112); ok {
		t.Fatal("a ward past until_tick was spent")
	}
}

func TestBlessingActive_SameTargetTwice(t *testing.T) {
	f := newBlessFix(t)
	ctx := context.Background()
	e := EffectBlessedStrength
	active := func(scope string, unit, city *uuid.UUID) bool {
		a, err := BlessingActive(ctx, f.pool, f.me, e, scope, unit, city, 105)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	if active(ScopeUnit, &f.home, nil) {
		t.Fatal("nothing blessed yet")
	}
	f.bless(t, f.me, e, ScopeUnit, &f.home, nil, 100, 110)
	if !active(ScopeUnit, &f.home, nil) || active(ScopeUnit, &f.marching, nil) {
		t.Fatal("a unit blessing conflicts with the same unit only")
	}
	f.bless(t, f.me, e, ScopeCity, nil, &f.cityA, 100, 110)
	if !active(ScopeCity, nil, &f.cityA) || active(ScopeCity, nil, &f.cityB) {
		t.Fatal("a city blessing conflicts with the same city only")
	}
	if !active(ScopeUnit, &f.marching, nil) {
		t.Fatal("a unit already covered by its city's blessing is already blessed")
	}
	if active(ScopeRealm, nil, nil) {
		t.Fatal("a realm blessing is not blocked by narrower ones")
	}
	f.bless(t, f.me, e, ScopeRealm, nil, nil, 100, 110)
	if !active(ScopeRealm, nil, nil) || !active(ScopeCity, nil, &f.cityB) {
		t.Fatal("a realm blessing covers everything")
	}
}
