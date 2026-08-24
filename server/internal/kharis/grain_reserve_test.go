package kharis

import "testing"

// spearmanGrainLevy is what a 100-man spearman cohort costs in grain
// (internal/province/training.go's recruit catalogue). It is repeated here as a
// LITERAL on purpose: this test's claim is about the player-visible gate ("can
// a Wanax raise a cohort from this city?"), so it must not be written in terms
// of growthGrainReserve — a test that measures a constant against itself passes
// no matter what the constant is, and this one silently did until 2026-08-24.
const spearmanGrainLevy = 300.0

// TestApplyDecay_GrowthNeverEatsTheReserve is the behavioural half of
// growthGrainReserve. The measurable claim is not "growth is slower" — it is
// that a self-sufficient city's STORED grain stops living in [0, 300) and
// settles at or above one cohort's levy, so a Wanax can actually raise troops.
//
// Before the reserve, the stored amount was a remainder below grainPerCitizen
// in 2 511 of 2 511 samples across three cities and 141 ticks (world e7923ca8,
// rapport_lagersvangningen_20260824.md). This test runs the same shape locally:
// a minimal but self-sufficient catchment, neglected, over many days.
func TestApplyDecay_GrowthNeverEatsTheReserve(t *testing.T) {
	terrains := [6]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, worldID, settlementID := newGrowthFixture(t, terrains, 1500)
	h := newTestTickHandler(pool)

	startPop, startGrain := snapshot(t, pool, settlementID)
	t.Logf("day 0: pop=%d grain=%.2f", startPop, startGrain)

	const days = 40
	// The first day may still open below the reserve if the fixture starts
	// there; from the day the stock first reaches the reserve it must never
	// drop back under it, because growth is the only thing that spends it here.
	reached := false
	minAfterReached := 1e9
	prevPop := startPop
	for day := 1; day <= days; day++ {
		advanceOneDay(t, h, pool, worldID)
		pop, grain := snapshot(t, pool, settlementID)
		t.Logf("day %d: pop=%d grain=%.2f", day, pop, grain)

		if pop < prevPop {
			t.Fatalf("day %d: population shrank %d -> %d — the reserve must never push a city onto "+
				"the starvation branch; it only sets growth's draw to zero", day, prevPop, pop)
		}
		prevPop = pop

		if grain >= spearmanGrainLevy {
			reached = true
		}
		if reached {
			if grain < minAfterReached {
				minAfterReached = grain
			}
			if grain < spearmanGrainLevy {
				t.Fatalf("day %d: stored grain fell to %.2f, below one cohort's levy (%.0f) — growth spent "+
					"the seed corn, and a Wanax cannot raise troops from this city",
					day, grain, spearmanGrainLevy)
			}
		}
	}

	if !reached {
		t.Fatalf("stored grain never reached one cohort's levy (%.0f) over %d days — this fixture is "+
			"supposed to be self-sufficient, so either the fixture changed or growth is still draining it",
			spearmanGrainLevy, days)
	}
	t.Logf("minimum grain after first reaching the reserve: %.2f", minAfterReached)

	if growthGrainReserve < spearmanGrainLevy {
		t.Errorf("growthGrainReserve (%.0f) is below one cohort's levy (%.0f) — the reserve exists so a "+
			"city can always raise the men it can feed; below the levy it does not deliver that",
			growthGrainReserve, spearmanGrainLevy)
	}

	if prevPop <= startPop {
		t.Errorf("expected net growth over %d days for a grain-positive city — the reserve must slow "+
			"growth near the line, not stop it; pop stayed at %d", days, prevPop)
	}
}
