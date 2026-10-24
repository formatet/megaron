package economy

import (
	"context"
	"encoding/json"
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

	placements := func() string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT gubbe_ordinal,target_kind,hex_q,hex_r,good_key FROM settlement_placement WHERE settlement_id=$1 ORDER BY gubbe_ordinal`, settlementID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var result [][]any
		for rows.Next() {
			v, err := rows.Values()
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, v)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	before := placements()
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
	if got := placements(); got != before {
		t.Fatalf("storm removed or changed fisher placements: %s != %s", got, before)
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
	if got := placements(); got != before {
		t.Fatalf("storm departure changed fisher placements: %s != %s", got, before)
	}
	if got := StormNoFishingHexes(ctx, pool, settlementID); len(got) != 0 {
		t.Fatalf("storm left but fishing grounds still covered: %v", got)
	}
	if _, again := readGood(t, settlementID, "fish"); again != calm {
		t.Fatalf("after the storm: rate %v, want calm %v", again, calm)
	}
}
