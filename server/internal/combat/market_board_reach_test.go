package combat

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
)

// marketCity puts a city owned by a new ruler at (q,0) with a Market of level.
func (f reachFixture) marketCity(t *testing.T, q, level int) (owner, city uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "board-"+uuid.New().String()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var prov uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,$2,0,'plains') RETURNING id`, f.world, q).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population)
		VALUES($1,$2,'Ugarit','achaean',$3,'capital',true,'active',1000) RETURNING id`, f.world, prov, owner).Scan(&city); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'market',$2)`, city, level); err != nil {
		t.Fatal(err)
	}
	return owner, city
}

func (f reachFixture) reading(t *testing.T, reader, city uuid.UUID) (buys []string, ok bool) {
	t.Helper()
	readings, err := economy.LoadBoardReadings(context.Background(), f.pool, f.world, reader)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := readings[city]
	return r.Buys, ok
}

// Only who touches the city reads its board (Timothy 2026-11-04): no reach,
// whatever the market's level.
func TestMarketBoardReadOnlyOnTheCityHex(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	_, city := f.marketCity(t, 8, 3) // an old level-3 market reaches no further
	if err := economy.WriteMarketBoard(ctx, f.pool, f.world, city, []string{"tin"}, []string{"wine"}, 0); err != nil {
		t.Fatal(err)
	}
	// A march ending next to the city (q=7, distance 1) reads nothing.
	f.mover(t, "infantry", f.owner, reachRoute(0, 7, 0, 7))
	f.scan(t, 7)
	if _, ok := f.reading(t, f.owner, city); ok {
		t.Fatal("board read from the neighbouring hex")
	}
	// A march whose road runs through the city hex on to q=10 reads it.
	f.mover(t, "infantry", f.owner, reachRoute(8, 18, 0, 10))
	f.scan(t, 18)
	buys, ok := f.reading(t, f.owner, city)
	if !ok || len(buys) != 1 || buys[0] != "tin" {
		t.Fatalf("board not read by a march through the city: ok=%v buys=%v", ok, buys)
	}
}

func TestMarketBoardOwnJourneyDoesNotRead(t *testing.T) {
	f := reachWorld(t)
	owner, city := f.marketCity(t, 8, 3)
	f.mover(t, "infantry", owner, reachRoute(0, 8, 0, 8))
	f.scan(t, 8)
	if _, ok := f.reading(t, owner, city); ok {
		t.Fatal("a ruler read their own board")
	}
}

func TestMarketBoardReadingIsASnapshot(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	_, city := f.marketCity(t, 8, 1)
	if err := economy.WriteMarketBoard(ctx, f.pool, f.world, city, []string{"tin"}, nil, 0); err != nil {
		t.Fatal(err)
	}
	f.mover(t, "infantry", f.owner, reachRoute(0, 8, 0, 8))
	f.scan(t, 8)
	if err := economy.WriteMarketBoard(ctx, f.pool, f.world, city, []string{"copper"}, nil, 9); err != nil {
		t.Fatal(err)
	}
	f.scan(t, 10) // rerun: no new entry, the reading stays as it was read
	buys, ok := f.reading(t, f.owner, city)
	if !ok || len(buys) != 1 || buys[0] != "tin" {
		t.Fatalf("reading followed the board after it was read: %v", buys)
	}
}
