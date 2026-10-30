package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"formatet/megaron/server/internal/province"
)

// A shipyard needs a waterway to the Thalassa (Timothy 2026-10-30): a city on
// a closed lake is refused; the same lake with a river out to the sea is not.
func TestBuildShipyard_ClosedLakeRefused(t *testing.T) {
	ctx := context.Background()
	f := buildHexFixture(t)
	if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain = 'lake' WHERE world_id = $1 AND q = 0 AND r = 1`, f.worldID); err != nil {
		t.Fatalf("lake: %v", err)
	}
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "shipyard"})
	if code != http.StatusUnprocessableEntity || resp["error"] != province.ShipyardLakeRefusal {
		t.Fatalf("shipyard on a closed lake = %d %v, want 422 %q", code, resp, province.ShipyardLakeRefusal)
	}
	// A harbour is still allowed at the closed lake (fishing).
	code, resp = f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "harbour"})
	if msg, _ := resp["error"].(string); strings.Contains(msg, "waterway") || strings.Contains(msg, "inland") {
		t.Fatalf("harbour on a closed lake = %d %v, want the water gate passed", code, resp)
	}
}

func TestBuildShipyard_LakeWithRiverToSeaPasses(t *testing.T) {
	ctx := context.Background()
	f := buildHexFixture(t)
	if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain = 'lake' WHERE world_id = $1 AND q = 0 AND r = 1`, f.worldID); err != nil {
		t.Fatalf("lake: %v", err)
	}
	for _, tile := range []struct {
		q, r    int
		terrain string
	}{{0, 2, "river"}, {0, 3, "river_ford"}, {0, 4, "coastal_sea"}} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			f.worldID, tile.q, tile.r, tile.terrain); err != nil {
			t.Fatalf("seed (%d,%d): %v", tile.q, tile.r, err)
		}
	}
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "shipyard"})
	if msg, _ := resp["error"].(string); msg == province.ShipyardLakeRefusal {
		t.Fatalf("shipyard on a lake with a river to the sea = %d %v, want the waterway gate passed", code, resp)
	}
}
