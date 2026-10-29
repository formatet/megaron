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

func TestMarketBoardReadOnlyWithinReachOfLevel(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	_, city := f.marketCity(t, 8, 1)
	if err := economy.WriteMarketBoard(ctx, f.pool, f.world, city, []string{"tin"}, []string{"wine"}, 0); err != nil {
		t.Fatal(err)
	}
	// Level 1 reaches 2 hexes: a march ending at q=5 (distance 3) reads nothing.
	f.mover(t, "infantry", f.owner, reachRoute(0, 5, 0, 5))
	f.scan(t, 5)
	if _, ok := f.reading(t, f.owner, city); ok {
		t.Fatal("board read from 3 hexes at market level 1")
	}
	// One step further, distance 2: read.
	f.mover(t, "infantry", f.owner, reachRoute(6, 12, 0, 6))
	f.scan(t, 12)
	buys, ok := f.reading(t, f.owner, city)
	if !ok || len(buys) != 1 || buys[0] != "tin" {
		t.Fatalf("board within reach not read: ok=%v buys=%v", ok, buys)
	}
	var heard int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM player_heard_tiles WHERE world_id=$1 AND player_id=$2 AND q=8 AND r=0 AND source='board'`, f.world, f.owner).Scan(&heard); err != nil {
		t.Fatal(err)
	}
	if heard != 1 {
		t.Fatal("reading from afar did not make the city's hex known by word")
	}
}

func TestMarketBoardReachGrowsWithLevel(t *testing.T) {
	f := reachWorld(t)
	_, city := f.marketCity(t, 8, 2)
	// Level 2 reaches 4 hexes: q=4 reads.
	f.mover(t, "infantry", f.owner, reachRoute(0, 4, 0, 4))
	f.scan(t, 4)
	if _, ok := f.reading(t, f.owner, city); !ok {
		t.Fatal("market level 2 did not reach 4 hexes")
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
	f.mover(t, "infantry", f.owner, reachRoute(0, 6, 0, 6))
	f.scan(t, 6)
	if err := economy.WriteMarketBoard(ctx, f.pool, f.world, city, []string{"copper"}, nil, 7); err != nil {
		t.Fatal(err)
	}
	f.scan(t, 8) // rerun: no new entry, the reading stays as it was read
	buys, ok := f.reading(t, f.owner, city)
	if !ok || len(buys) != 1 || buys[0] != "tin" {
		t.Fatalf("reading followed the board after it was read: %v", buys)
	}
}
