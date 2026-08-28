package handlers

// Regression tests for the "riv procentallokeringen" slice
// (megaron_plan_riv_procentallokeringen.md): LaborAlloc is narrowed to a
// cult-devotion endpoint. Every non-cult key in the percent map must be
// ignored (no settlement_labor row written for it) and the response must say
// so explicitly — never stay silent about it (§2 of the plan).
//
// Real Postgres, gated by DATABASE_URL — reuses setupPlacementFixture.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestLaborAlloc_NonCultKeysIgnoredAndExplained is framgångskriterium 1: naming
// a production good writes nothing for it and the response explains why.
func TestLaborAlloc_NonCultKeysIgnoredAndExplained(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := seedTempleAndDevotion(t, f, 1, 0.15)

	code, resp := f.do(t, http.MethodPut, laborPath(f),
		map[string]any{"percent": map[string]float64{"grain": 60}})
	if code != http.StatusOK {
		t.Fatalf("LaborAlloc(grain=60) = %d: %v, want 200", code, resp)
	}

	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "cult") || !strings.Contains(msg, "grain") {
		t.Errorf("response message = %q, want it to name cult as the only lever and call out the ignored grain key", msg)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM settlement_labor WHERE settlement_id = $1 AND good_key = 'grain'`,
		f.settlementID,
	).Scan(&count); err != nil {
		t.Fatalf("count grain rows: %v", err)
	}
	if count != 0 {
		t.Errorf("settlement_labor has %d grain row(s) after PUT naming only grain — want 0 (dead write must not happen)", count)
	}
}

// TestLaborAlloc_CultOnlySetsDevotion is framgångskriterium 2: naming cult
// alone still sets devotion, unchanged from before the narrowing.
func TestLaborAlloc_CultOnlySetsDevotion(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := seedTempleAndDevotion(t, f, 3, 0.15)

	code, resp := f.do(t, http.MethodPut, laborPath(f),
		map[string]any{"percent": map[string]float64{"cult": 30}})
	if code != http.StatusOK {
		t.Fatalf("LaborAlloc(cult=30) = %d: %v, want 200", code, resp)
	}
	if cp, ok := resp["cult_percent"].(float64); !ok || !nearly(cp, 30) {
		t.Errorf("resp[cult_percent] = %v, want 30", resp["cult_percent"])
	}
	if w := readCultWeight(t, pool, f.settlementID); !nearly(w, 0.30) {
		t.Errorf("cult weight = %v, want 0.30", w)
	}
}

// TestLaborAlloc_NoNonCultRowSurvives is framgångskriterium 4: after an
// allocation, settlement_labor carries no row for any good except cult.
func TestLaborAlloc_NoNonCultRowSurvives(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := seedTempleAndDevotion(t, f, 2, 0.15)

	code, resp := f.do(t, http.MethodPut, laborPath(f),
		map[string]any{"percent": map[string]float64{"grain": 40, "timber": 20, "cult": 20}})
	if code != http.StatusOK {
		t.Fatalf("LaborAlloc = %d: %v, want 200", code, resp)
	}

	rows, err := pool.Query(context.Background(),
		`SELECT good_key FROM settlement_labor WHERE settlement_id = $1`, f.settlementID)
	if err != nil {
		t.Fatalf("query settlement_labor: %v", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		keys = append(keys, k)
	}
	if len(keys) != 1 || keys[0] != "cult" {
		t.Errorf("settlement_labor rows for settlement = %v, want exactly [cult]", keys)
	}
}
