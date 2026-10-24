package economy

import (
	"context"
	"testing"
)

// Storm stops the fishing (Timothy 2026-10-24): while a storm stands on a
// hex, that hex's fishing places give nothing; when it moves on, they fish
// again. The stored rate follows on each recompute.
func TestRecompute_StormOnTheHexStopsItsFishing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 200
	settlementID := recomputeWaterFixture(t, tick, 60, 0, 2)
	placeHexGubbe(t, pool, settlementID, 1, recomputeWaterFixtureOffsets[0], "fish")
	placeHexGubbe(t, pool, settlementID, 2, recomputeWaterFixtureOffsets[1], "fish")
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatal(err)
	}
	_, calm := readGood(t, settlementID, "fish")
	if calm <= 0 {
		t.Fatalf("calm fish rate %v, want > 0", calm)
	}
	var storm string
	if err := pool.QueryRow(ctx, `INSERT INTO sea_storms(world_id,heading,created_tick) SELECT world_id,0,$2 FROM settlements WHERE id=$1 RETURNING id`, settlementID, tick).Scan(&storm); err != nil {
		t.Fatal(err)
	}
	c := recomputeWaterFixtureOffsets[0]
	if _, err := pool.Exec(ctx, `INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES($1,$2,0,$3,$4)`, storm, tick, c.Q, c.R); err != nil {
		t.Fatal(err)
	}
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatal(err)
	}
	_, stormy := readGood(t, settlementID, "fish")
	if stormy <= 0 || stormy >= calm {
		t.Fatalf("storm over one of two fishing hexes: rate %v, want between 0 and calm %v", stormy, calm)
	}
	if got := StormNoFishingHexes(ctx, pool, settlementID); len(got) != 1 || got[0] != c {
		t.Fatalf("StormNoFishingHexes = %v, want [%v]", got, c)
	}
	// The storm moves on: the track for this tick no longer names the hex.
	if _, err := pool.Exec(ctx, `UPDATE sea_storm_track SET q=50,r=50 WHERE storm_id=$1`, storm); err != nil {
		t.Fatal(err)
	}
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatal(err)
	}
	if _, again := readGood(t, settlementID, "fish"); again != calm {
		t.Fatalf("after the storm: rate %v, want calm %v", again, calm)
	}
}
