package economy

import "testing"

// TestFoundingGrainNetPerTick_Regression reproduces the colonize-preview
// sign-flip bug: a building-free base that is negative once labor-scaled
// down (raw base − consumption < 0) but strongly positive once the real
// founding labor formula (base/REF_LABOR × 0.85×pop) is applied — the
// formula the actual founding uses via RecomputeProduction. Before the fix,
// ColonizePreview reported the raw unscaled netto (negative); the real
// settlement ends up net-positive.
func TestFoundingGrainNetPerTick_Regression(t *testing.T) {
	// buildingFreeBase alone (2.4) is far below what a 4000-pop metropolis eats
	// per tick, so an unscaled "base - consumption" netto goes negative. But the
	// metropolis gets a starter farm (withFarmBase=6.0), and once labor-scaled
	// by 0.85×4000/REF_LABOR the real production dwarfs consumption.
	_, netPerTick := FoundingGrainNetPerTick(2.4, 6.0, 4000, true)
	if netPerTick <= 0 {
		t.Fatalf("expected positive labor-scaled net grain rate, got %v", netPerTick)
	}
}

// TestFoundingGrainNetPerTick_MirrorsRecomputeProduction asserts the
// production term uses exactly the same formula as
// RecomputeProduction: (base/REF_LABOR) * (weight * pop).
func TestFoundingGrainNetPerTick_MirrorsRecomputeProduction(t *testing.T) {
	withFarmBase := 6.0
	pop := 4000
	prodPerTick, _ := FoundingGrainNetPerTick(2.4, withFarmBase, pop, true)
	want := (withFarmBase / REF_LABOR) * (FoundingGrainLaborWeight * float64(pop))
	if prodPerTick != want {
		t.Fatalf("prodPerTick = %v, want %v (RecomputeProduction formula)", prodPerTick, want)
	}
}

// TestFoundingGrainNetPerTick_ColonyNoFarm asserts starterFarm=false uses
// the building-free base, not the with-farm base — a colony builds its own
// farm later and gets no starter farm.
func TestFoundingGrainNetPerTick_ColonyNoFarm(t *testing.T) {
	buildingFreeBase := 2.4
	withFarmBase := 6.0
	pop := 1500
	prodPerTick, _ := FoundingGrainNetPerTick(buildingFreeBase, withFarmBase, pop, false)
	want := (buildingFreeBase / REF_LABOR) * (FoundingGrainLaborWeight * float64(pop))
	if prodPerTick != want {
		t.Fatalf("prodPerTick = %v, want %v (should use buildingFreeBase, not withFarmBase)", prodPerTick, want)
	}
}

// TestFoundingGrainNetPerTick_Consumption asserts net = production − consumption,
// using the shared GrainConsumptionPerTick helper.
func TestFoundingGrainNetPerTick_Consumption(t *testing.T) {
	pop := 4000
	prodPerTick, netPerTick := FoundingGrainNetPerTick(2.4, 6.0, pop, true)
	want := prodPerTick - GrainConsumptionPerTick(pop)
	if netPerTick != want {
		t.Fatalf("netPerTick = %v, want %v (production - consumption)", netPerTick, want)
	}
}
