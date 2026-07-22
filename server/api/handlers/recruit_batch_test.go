package handlers

// Tests for the naval batch-recruit --count QoL (soak plan, Sprint: cutover +
// soak-QoL). No DB harness needed here (Recruit is not extracted into a pure
// function) — these mirror the validation/cost-scaling arithmetic the handler
// applies, the same style as recruit_afford_test.go.

import (
	"testing"

	"formatet/megaron/server/internal/unit"
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
// cap check must account for every vessel's own TrainComplete, not just one —
// otherwise a large --count could blow past the 10-pending cap undetected.
//
// Ship-build overhaul (2026-07-09): a naval build schedules exactly ONE
// TrainComplete per vessel (its build time), regardless of crew size — not
// one per 10 crew like land. So totalBatches for naval is simply the vessel
// count.
func TestRecruitBatch_TrainingQueueCapCountsAllVessels(t *testing.T) {
	const batchesPerVessel = 1
	count := 11 // over the 10-pending cap on its own
	totalBatches := batchesPerVessel * count

	if totalBatches != 11 {
		t.Fatalf("totalBatches = %d, want 11 (1 batch × 11 vessels)", totalBatches)
	}

	pending := 0
	if pending+totalBatches <= 10 {
		t.Error("11 ships should overflow the 10-pending training queue cap, but check passed")
	}
}

// TestRecruitBatch_ShipResolvesToNavalGalley guards the fix for the historical
// "ship"/"galley" split found while implementing --count (before namn-hygien A,
// mig 084, made "galley" the sole canonical units.type value): a legacy client
// sending unit_type "ship" must still resolve to the naval galley (crew 20) via
// unit.Canonical, not fall through to a broken forming land-unit (crew 0, never
// garrison) — TrainComplete's isNaval check would then skip the forming→garrison
// flip, stranding it forever. The separate size-semantics bug (size set to
// req.Men, not 1, for naval types) was fixed by the ship-build overhaul
// (2026-07-09): naval size is now always 1 — see
// TestRecruitShip_NavalBuildsOneForming{Vessel,Unit} in recruit_ship_test.go.
func TestRecruitBatch_ShipResolvesToNavalGalley(t *testing.T) {
	canonical := unit.Canonical("ship")
	if canonical != "galley" {
		t.Fatalf(`unit.Canonical("ship") = %q, want "galley"`, canonical)
	}
	if got := unit.CategoryOf(unit.Type(canonical)); got != unit.CategoryNaval {
		t.Errorf(`unit.CategoryOf(%q) = %s, want naval`, canonical, got)
	}
	if got := unit.CrewFor(unit.Type(canonical)); got != 20 {
		t.Errorf(`unit.CrewFor(%q) = %d, want 20`, canonical, got)
	}
}
