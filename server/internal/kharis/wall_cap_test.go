package kharis

import (
	"context"
	"sync/atomic"
	"testing"

	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// B1 "muren som befolkningstak" (Timothy 2026-10-21): the wall is the population
// ceiling. These tests run the real applyDecay SQL.

var wallTerrains = [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}

// feed gives a settlement a big food surplus and no hunger, at the given
// population and wall level, so only the ceiling decides what applyDecay does.
func feed(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, pop, wall int) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 100000, 5000, 1000000, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE
		 SET amount = EXCLUDED.amount, rate = EXCLUDED.rate, cap = EXCLUDED.cap, calc_tick = EXCLUDED.calc_tick`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET population = $2, wall_level = $3, food_unmet_amount = 0 WHERE id = $1`, sid, pop, wall); err != nil {
		t.Fatal(err)
	}
}

func decayOnce(t *testing.T, pool *pgxpool.Pool, h *TickHandler, world, sid uuid.UUID) int {
	t.Helper()
	h.applyDecay(context.Background(), world, atomic.AddInt64(&advanceOneDayEventID, 1))
	var pop int
	if err := pool.QueryRow(context.Background(), `SELECT population FROM settlements WHERE id = $1`, sid).Scan(&pop); err != nil {
		t.Fatal(err)
	}
	return pop
}

// (a) a city with a Palisade (cap 5 000) standing at 5 000 does not grow.
func TestMurtak_PalisadeCityAtCapDoesNotGrow(t *testing.T) {
	pool, world, sid := newGrowthFixture(t, wallTerrains, 100)
	feed(t, pool, sid, 5000, 1)
	if got := decayOnce(t, pool, newTestTickHandler(pool), world, sid); got != 5000 {
		t.Fatalf("Palisade city at its cap of %d went 5000 -> %d, want it to hold", economy.PopulationCap(1), got)
	}
}

// (b) the same city after Stone Wall (cap 10 000) grows again.
func TestMurtak_SameCityGrowsAfterStoneWall(t *testing.T) {
	pool, world, sid := newGrowthFixture(t, wallTerrains, 100)
	feed(t, pool, sid, 5000, 2)
	if got := decayOnce(t, pool, newTestTickHandler(pool), world, sid); got <= 5000 {
		t.Fatalf("Stone Wall city 5000 -> %d, want growth", got)
	}
}

// (d) a new 1 000-person city with no wall grows while its neighbour, standing
// at its own cap, holds still — in the same world, in the same tick.
func TestMurtak_NewCityGrowsWhileNeighbourAtCapHolds(t *testing.T) {
	pool, world, newSid := newGrowthFixture(t, wallTerrains, 100)
	ctx := context.Background()
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, newSid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var prov, oldSid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 40, 40, 'plains') RETURNING id`, world).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Old Town', 'achaean', $3, 'colony', false, 2000) RETURNING id`, world, prov, owner).Scan(&oldSid); err != nil {
		t.Fatal(err)
	}
	feed(t, pool, newSid, 1000, 0)
	feed(t, pool, oldSid, 2000, 0)
	h := newTestTickHandler(pool)
	h.applyDecay(ctx, world, atomic.AddInt64(&advanceOneDayEventID, 1))
	var newPop, oldPop int
	_ = pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, newSid).Scan(&newPop)
	_ = pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, oldSid).Scan(&oldPop)
	if newPop <= 1000 {
		t.Errorf("new city 1000 -> %d, want growth", newPop)
	}
	if oldPop != 2000 {
		t.Errorf("neighbour at its cap 2000 -> %d, want it to hold", oldPop)
	}
}

// (f) a city already over its cap loses nobody — it stands still.
func TestMurtak_CityOverCapLosesNoOne(t *testing.T) {
	pool, world, sid := newGrowthFixture(t, wallTerrains, 100)
	feed(t, pool, sid, 19000, 0) // far over the no-wall cap of 2 000
	h := newTestTickHandler(pool)
	for i := 0; i < 3; i++ {
		if got := decayOnce(t, pool, h, world, sid); got != 19000 {
			t.Fatalf("day %d: city over its cap went 19000 -> %d, want 19000", i+1, got)
		}
	}
}

// Growth never overshoots: one step below the cap lands exactly on it.
func TestMurtak_GrowthStopsExactlyAtCap(t *testing.T) {
	pool, world, sid := newGrowthFixture(t, wallTerrains, 100)
	feed(t, pool, sid, 1999, 0)
	h := newTestTickHandler(pool)
	if got := decayOnce(t, pool, h, world, sid); got != 2000 {
		t.Fatalf("1999 under cap 2000 -> %d, want exactly 2000", got)
	}
	if got := decayOnce(t, pool, h, world, sid); got != 2000 {
		t.Fatalf("at cap -> %d, want 2000", got)
	}
}
