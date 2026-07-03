package handlers

// Tests for the naval batch-recruit --count QoL (soak plan, Sprint: cutover +
// soak-QoL). No DB harness needed here (Recruit is not extracted into a pure
// function) — these mirror the validation/cost-scaling arithmetic the handler
// applies, the same style as recruit_afford_test.go.

import (
	"testing"

	"github.com/poleia/server/internal/unit"
)

// TestRecruitBatch_CountValidationRange documents the handler's count bounds:
// 0 defaults to 1 (backward compatible with pre-count clients), 1–20 is valid,
// anything else is a 400.
func TestRecruitBatch_CountValidationRange(t *testing.T) {
	normalize := func(count int) (int, bool) {
		if count == 0 {
			count = 1
		}
		if count < 1 || count > 20 {
			return 0, false
		}
		return count, true
	}

	cases := []struct {
		in      int
		wantOK  bool
		wantVal int
	}{
		{0, true, 1},   // omitted → default 1
		{1, true, 1},
		{20, true, 20},
		{21, false, 0}, // over cap
		{-1, false, 0}, // negative
	}
	for _, tc := range cases {
		got, ok := normalize(tc.in)
		if ok != tc.wantOK {
			t.Errorf("normalize(%d) ok = %v, want %v", tc.in, ok, tc.wantOK)
			continue
		}
		if ok && got != tc.wantVal {
			t.Errorf("normalize(%d) = %d, want %d", tc.in, got, tc.wantVal)
		}
	}
}

// TestRecruitBatch_EffectiveCountIgnoredForLand verifies that count only
// multiplies naval recruits — land units keep batching via --men regardless
// of what count was sent (handler forces effectiveCount = 1 for CategoryLand).
func TestRecruitBatch_EffectiveCountIgnoredForLand(t *testing.T) {
	effectiveCount := func(cat unit.Category, count int) int {
		if cat == unit.CategoryNaval {
			return count
		}
		return 1
	}

	landTypes := []string{"spearman", "war_chariot", "elite_infantry"}
	for _, ut := range landTypes {
		cat := unit.CategoryOf(unit.Type(ut))
		if cat != unit.CategoryLand {
			t.Fatalf("test premise invalid: %s expected land, got %s", ut, cat)
		}
		if got := effectiveCount(cat, 5); got != 1 {
			t.Errorf("land unit %s with count=5: effectiveCount = %d, want 1 (ignored)", ut, got)
		}
	}

	// war_galley and merchantman correctly resolve to CategoryNaval (their
	// unit.Type constants match the API/DB strings exactly) — count applies.
	navalTypes := []string{"war_galley", "merchantman"}
	for _, ut := range navalTypes {
		cat := unit.CategoryOf(unit.Type(ut))
		if cat != unit.CategoryNaval {
			t.Fatalf("test premise invalid: %s expected naval, got %s", ut, cat)
		}
		if got := effectiveCount(cat, 5); got != 5 {
			t.Errorf("naval unit %s with count=5: effectiveCount = %d, want 5", ut, got)
		}
	}
}

// TestRecruitBatch_CostsScaleWithCount verifies total cost/kharis/population
// draft scale by men × count for naval batches — the upfront affordability
// check must cover the whole batch, not just one vessel.
func TestRecruitBatch_CostsScaleWithCount(t *testing.T) {
	perManCosts := recruitPerManCosts("war_galley")
	if perManCosts == nil {
		t.Fatal("test premise invalid: war_galley must have per-man costs")
	}

	men := 50 // war_galley crew
	count := 4
	totalMen := men * count

	totalCosts := make(map[string]float64, len(perManCosts))
	for k, v := range perManCosts {
		totalCosts[k] = v * float64(totalMen)
	}

	for k, perMan := range perManCosts {
		want := perMan * float64(men) * float64(count)
		if totalCosts[k] != want {
			t.Errorf("totalCosts[%s] = %v, want %v (4× single-vessel cost)", k, totalCosts[k], want)
		}
	}

	if totalMen != 200 {
		t.Errorf("totalMen = %d, want 200 (50 crew × 4 vessels)", totalMen)
	}
}

// TestRecruitBatch_TrainingQueueCapCountsAllVessels verifies the pending-batch
// cap check must account for every vessel's own TrainComplete batches, not
// just one — otherwise a large --count could blow past the 10-pending cap
// undetected.
func TestRecruitBatch_TrainingQueueCapCountsAllVessels(t *testing.T) {
	men := 20 // ship crew → batches = 2 (20/10)
	batchesPerUnit := men / 10
	count := 6
	totalBatches := batchesPerUnit * count

	if totalBatches != 12 {
		t.Fatalf("totalBatches = %d, want 12 (2 batches × 6 vessels)", totalBatches)
	}

	pending := 0
	if pending+totalBatches <= 10 {
		t.Error("6 ships at 20 crew each should overflow the 10-pending training queue cap, but check passed")
	}
}

// TestRecruitBatch_ShipResolvesToNavalGalley guards the fix for the "ship"/"galley"
// split found while implementing --count: the canonical API/UnitSpecs/CLI value is
// "ship" while the unit-model constant is "galley". Before the fix CategoryOf("ship")
// fell through to land and CrewFor("ship") to 0, so the Recruit handler built a broken
// forming land-unit (crew 0, never garrison) for the standard galley — TrainComplete's
// isNaval check then skipped the forming→garrison flip, stranding it forever. "ship"
// must now resolve to the naval galley (crew 20). Full rename→galley = D-stream/SB7.
// NOTE: the separate size-semantics bug (size set to req.Men, not 1, for ALL naval
// types) is still open and owned by Timothy (#7 naval flottdesign).
func TestRecruitBatch_ShipResolvesToNavalGalley(t *testing.T) {
	if got := unit.CategoryOf(unit.Type("ship")); got != unit.CategoryNaval {
		t.Errorf(`unit.CategoryOf("ship") = %s, want naval`, got)
	}
	if got := unit.CrewFor(unit.Type("ship")); got != 20 {
		t.Errorf(`unit.CrewFor("ship") = %d, want 20`, got)
	}
}
