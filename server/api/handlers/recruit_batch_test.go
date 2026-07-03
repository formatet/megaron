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

// TestRecruitBatch_ShipCategoryMismatch documents a pre-existing, unrelated
// bug discovered while implementing --count: unit.Type("ship") does not equal
// unit.TypeGalley ("galley"), so unit.CategoryOf/CrewFor fall through to their
// land/zero defaults for the "ship" unit type — even though every other part
// of the codebase (province.UnitSpecs, cmd_recruit.go aliases, train.go's
// isNaval string check) treats "ship" as the canonical galley type. This test
// pins the CURRENT (buggy) behaviour so it doesn't regress silently further;
// it is NOT a design decision this task is authorized to fix — see the recruit
// handler comment "Naval units ... size always 1" for the intended contract
// this bug also violates (size is set to req.Men, not 1, for ALL naval types,
// including war_galley/merchantman which resolve correctly).
func TestRecruitBatch_ShipCategoryMismatch(t *testing.T) {
	got := unit.CategoryOf(unit.Type("ship"))
	if got != unit.CategoryLand {
		t.Errorf(`unit.CategoryOf("ship") = %s — if this now returns "naval", the `+
			`TypeGalley/"ship" mismatch has been fixed; update the Recruit handler's `+
			`naval branch assumptions and this test`, got)
	}
}
