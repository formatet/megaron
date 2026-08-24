package handlers

// Proof test for megaron_plan_tysta_forluster.md §Hål 3: Recruit already
// computed totalMen (the actual head-count drafted from population — the
// insufficient-population check at province.go ~line 1898 uses it) and
// popAfter (the RETURNING value from the population-deduction UPDATE, ~line
// 2032), but never put either in the response. A successful recruit showed a
// new unit and nothing else — the population cost was invisible until the
// Wanax happened to diff two `keryx status` calls. Reuses
// setupRecruitShipFixture from recruit_ship_test.go (same package).

import (
	"context"
	"net/http"
	"testing"
)

func TestRecruit_ResponseReportsPopulationDrawn(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()

	recruitPath := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"

	// Land: a spearman cohort always drafts a full 100-man cohort
	// (kohort-rekrytering) regardless of the men sent. Fixture seeds
	// population=5000.
	rec, resp := f.post(t, recruitPath, map[string]any{"unit_type": "spearman", "men": 30})
	if rec.Code != http.StatusCreated {
		t.Fatalf("Recruit(spearman) = %d %q, want 201", rec.Code, rec.Body.String())
	}
	popDrawn, _ := resp["pop_drawn"].(float64)
	popBefore, _ := resp["population_before"].(float64)
	popAfter, _ := resp["population_after"].(float64)
	if popDrawn != 100 {
		t.Errorf("land pop_drawn = %v, want 100 (full cohort, men=30 ignored)", resp["pop_drawn"])
	}
	if popBefore != 5000 {
		t.Errorf("land population_before = %v, want 5000 (fixture seed)", resp["population_before"])
	}
	if popAfter != 4900 {
		t.Errorf("land population_after = %v, want 4900", resp["population_after"])
	}
	if popBefore-popAfter != popDrawn {
		t.Errorf("population_before(%v) - population_after(%v) = %v, want pop_drawn(%v)",
			popBefore, popAfter, popBefore-popAfter, popDrawn)
	}

	var dbPop int
	if err := f.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, f.settlementID).Scan(&dbPop); err != nil {
		t.Fatalf("load settlement population: %v", err)
	}
	if float64(dbPop) != popAfter {
		t.Errorf("DB population = %d, want response population_after = %v — client must see the same truth as the server", dbPop, popAfter)
	}

	// Naval: a galley draws its fixed crew (20), on top of the land draft above.
	rec2, resp2 := f.post(t, recruitPath, map[string]any{"unit_type": "galley"})
	if rec2.Code != http.StatusCreated {
		t.Fatalf("Recruit(galley) = %d %q, want 201", rec2.Code, rec2.Body.String())
	}
	popDrawn2, _ := resp2["pop_drawn"].(float64)
	popBefore2, _ := resp2["population_before"].(float64)
	popAfter2, _ := resp2["population_after"].(float64)
	if popDrawn2 != 20 {
		t.Errorf("naval pop_drawn = %v, want 20 (galley crew)", resp2["pop_drawn"])
	}
	if popBefore2 != 4900 {
		t.Errorf("naval population_before = %v, want 4900 (continues from the land draft above)", resp2["population_before"])
	}
	if popAfter2 != 4880 {
		t.Errorf("naval population_after = %v, want 4880", resp2["population_after"])
	}
}
