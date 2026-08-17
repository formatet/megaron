package handlers

// Regression test for KH1 (megaron_todo.md §Krig, decision A locked 2026-08-07):
// the server must PRESERVE a settlement's cult devotion when the client's
// labor re-allocation does not name cult. LaborAlloc wipes every settlement_labor
// row (DELETE) and then restores cult; before the fix the restore pinned cult
// back to the bare 0.15 floor, so a level-3 temple at 0.45 devotion silently
// dropped to 0.15 whenever a re-allocation named only its producing jobs (as a
// keryx/agent client does). Cult is the one settlement_labor row still read live
// (kharis/tick.go), so this is a real devotion loss, not a dead write.
//
// Real Postgres, gated by DATABASE_URL — reuses setupPlacementFixture (catchment
// tiles make grain producible so the re-allocation is accepted).

import (
	"context"
	"math"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// nearly compares weights read back from the real (float32) settlement_labor.weight
// column, tolerating the float32→float64 widening error.
func nearly(a, b float64) bool { return math.Abs(a-b) < 1e-4 }

// laborPath is the PUT target the placement fixture now registers.
func laborPath(f *placementFixture) string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/labor"
}

// seedTempleAndDevotion gives the fixture settlement a level-L temple and sets
// its cult weight, returning a fresh pool handle for read-back.
func seedTempleAndDevotion(t *testing.T, f *placementFixture, level int, cult float64) *pgxpool.Pool {
	t.Helper()
	pool := p10TestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'temple', $2)`,
		f.settlementID, level,
	); err != nil {
		t.Fatalf("seed temple: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_labor (settlement_id, good_key, weight) VALUES ($1, 'cult', $2)
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET weight = $2`,
		f.settlementID, cult,
	); err != nil {
		t.Fatalf("seed cult devotion: %v", err)
	}
	return pool
}

func readCultWeight(t *testing.T, pool *pgxpool.Pool, settlementID interface{ String() string }) float64 {
	t.Helper()
	var w float64
	if err := pool.QueryRow(context.Background(),
		`SELECT weight FROM settlement_labor WHERE settlement_id = $1 AND good_key = 'cult'`,
		settlementID.String(),
	).Scan(&w); err != nil {
		t.Fatalf("read cult weight: %v", err)
	}
	return w
}

// TestLaborAlloc_PreservesCultWhenNotNamed is the core KH1 case: a level-3 temple
// at 0.45 devotion, re-allocating labour naming ONLY grain, must keep 0.45 — not
// collapse to the 0.15 floor.
func TestLaborAlloc_PreservesCultWhenNotNamed(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := seedTempleAndDevotion(t, f, 3, 0.45)

	code, resp := f.do(t, http.MethodPut, laborPath(f),
		map[string]any{"percent": map[string]float64{"grain": 50}})
	if code != http.StatusOK {
		t.Fatalf("LaborAlloc(grain=50, no cult) = %d: %v, want 200", code, resp)
	}

	if w := readCultWeight(t, pool, f.settlementID); !nearly(w, 0.45) {
		t.Errorf("cult weight after re-alloc omitting cult = %v, want preserved 0.45 (KH1); "+
			"0.15 means it collapsed to the floor", w)
	}
}

// TestLaborAlloc_CultBelowFloorStillFloored guards the sentinel path: if the
// settlement somehow carries a below-floor cult weight (or none above floor),
// omitting cult holds the 0.15 floor as before — the fix only preserves values
// worth preserving, it does not let devotion sink below the holy floor.
func TestLaborAlloc_CultBelowFloorStillFloored(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := seedTempleAndDevotion(t, f, 3, 0.05) // below the 0.15 floor

	code, resp := f.do(t, http.MethodPut, laborPath(f),
		map[string]any{"percent": map[string]float64{"grain": 50}})
	if code != http.StatusOK {
		t.Fatalf("LaborAlloc = %d: %v, want 200", code, resp)
	}

	if w := readCultWeight(t, pool, f.settlementID); !nearly(w, 0.15) {
		t.Errorf("cult weight = %v, want floor 0.15 (below-floor devotion is not preserved)", w)
	}
}
