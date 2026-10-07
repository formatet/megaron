package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/province"
)

func TestMapTradesFrozenPositionAndArrival(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	// Freeze a detour that deliberately differs from the world's present tiles.
	// Position reading must use saved data, even after the tiles are deleted.
	j := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0, R: 0}, {Q: 0, R: 1}, {Q: 1, R: 1}, {Q: 2, R: 0}}, StepCosts: []int64{750, 2500, 750}, Distance: 3, TravelTicks: 6}
	raw, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE transports SET journey=$1, departed_tick=10,due_tick=16 WHERE world_id=$2 AND owner_id=$3`, raw, f.worldID, f.playerA); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE worlds SET current_tick=12,last_tick_at=now() WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	m := findByGood(t, f.get(t, f.tokenA), "grain")
	if m["current_q"] != float64(1) || m["current_r"] != float64(1) || m["arrival_tick"] != float64(16) || m["travel_ticks"] != float64(6) {
		t.Fatalf("wrong frozen marker: %v", m)
	}
	// Corrupt new data fails closed; it must not become a NULL legacy journey.
	if _, err = f.pool.Exec(ctx, `UPDATE transports SET journey='{}'::jsonb WHERE world_id=$1 AND owner_id=$2`, f.worldID, f.playerA); err != nil {
		t.Fatal(err)
	}
	for _, m := range f.get(t, f.tokenA) {
		if m["mine"] == true {
			t.Fatalf("corrupt journey exposed via fallback: %v", m)
		}
	}
}
