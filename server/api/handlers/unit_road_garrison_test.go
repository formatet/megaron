package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// An army garrisoned in the ordering city hears the order at once (no Runner);
// standing at the "from" hex already, it is flagged there and the stone caravan
// is the only thing the road waits for.
func TestBuildRoad_GarrisonedArmyHearsAtOnce(t *testing.T) {
	a := newRoadAPI(t, 100)
	ctx := context.Background()
	if _, err := a.pool.Exec(ctx, `UPDATE units SET status = 'garrison', settlement_id = $2, q = NULL, r = NULL WHERE id = $1`, a.unit, a.city); err != nil {
		t.Fatal(err)
	}
	rec := a.do(t, http.MethodPost, a.buildURL(), a.token, map[string]int{"from_q": 0, "from_r": 0, "to_q": 1, "to_r": 0})
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "road_ordered") {
		t.Fatalf("garrisoned order = %d %s, want 202 road_ordered", rec.Code, rec.Body.String())
	}
	var n int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM messengers WHERE world_id = $1 AND kind = 'order'`, a.world).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a garrisoned army needs no Runner, found %d", n)
	}
	var army, stoneIn bool
	if err := a.pool.QueryRow(ctx, `SELECT army_arrived, stone_arrived FROM road_sites WHERE world_id = $1`, a.world).Scan(&army, &stoneIn); err != nil || !army || stoneIn {
		t.Fatalf("army_arrived=%v stone_arrived=%v (err %v), want true/false", army, stoneIn, err)
	}
}
