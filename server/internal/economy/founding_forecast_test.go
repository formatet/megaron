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
	// buildingFreeBase alone (57.6) is far below what a 4000-pop metropolis eats
	// per tick, so an unscaled "base - consumption" netto goes negative. But the
	// metropolis gets a starter farm (withFarmBase=144.0), and once labor-scaled
	// by 0.85×4000/REF_LABOR the real production dwarfs consumption.
	// ⭐ CANON 2026-08-06: a tick is the day now, so
	// GrainConsumptionPerTick(pop) went from pop*0.5/24 to
	// pop*0.5 — a 24× jump in consumption. production_rules base potentials
	// (mig 109) scaled ×24 too, so these literals are the pre-canon 2.4/6.0 ×24
	// — matching real base_potential magnitudes read by world.go, not an
	// arbitrary rescale to make the assertion pass.
	_, netPerTick := FoundingGrainNetPerTick(57.6, 144.0, 0, 4000, true)
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
	prodPerTick, _ := FoundingGrainNetPerTick(2.4, withFarmBase, 0, pop, true)
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
	prodPerTick, _ := FoundingGrainNetPerTick(buildingFreeBase, withFarmBase, 0, pop, false)
	want := (buildingFreeBase / REF_LABOR) * (FoundingGrainLaborWeight * float64(pop))
	if prodPerTick != want {
		t.Fatalf("prodPerTick = %v, want %v (should use buildingFreeBase, not withFarmBase)", prodPerTick, want)
	}
}

// TestFoundingGrainNetPerTick_Consumption asserts net = production − consumption,
// using the shared GrainConsumptionPerTick helper.
func TestFoundingGrainNetPerTick_Consumption(t *testing.T) {
	pop := 4000
	prodPerTick, netPerTick := FoundingGrainNetPerTick(2.4, 6.0, 0, pop, true)
	want := prodPerTick - GrainConsumptionPerTick(pop)
	if netPerTick != want {
		t.Fatalf("netPerTick = %v, want %v (production - consumption)", netPerTick, want)
	}
}

// TestFoundingGrainNetPerTick_FishRaisesNet is AK5's founding-forecast half: a
// hex with water in the catchment (fishBase > 0) must forecast a HIGHER net
// than the identical hex without water, because fish covers whatever grain
// does not reach. Mirrors the AK1 shape (grain alone insufficient) at
// founding-preview scale.
func TestFoundingGrainNetPerTick_FishRaisesNet(t *testing.T) {
	buildingFreeBase := 1.0 // deliberately low: grain alone will not cover demand
	withFarmBase := 1.0
	pop := 1500

	_, netNoWater := FoundingGrainNetPerTick(buildingFreeBase, withFarmBase, 0, pop, false)
	_, netWithWater := FoundingGrainNetPerTick(buildingFreeBase, withFarmBase, 20.0, pop, false)

	if netWithWater <= netNoWater {
		t.Fatalf("water in the catchment must forecast a higher net: no-water=%v, with-water=%v",
			netNoWater, netWithWater)
	}
}

// TestFoundingGrainNetPerTick_FishNeverAffectsGrainProduction pins that
// prodPerTick (grain's OWN production) stays on the "production" footing
// api/handlers/world.go's with_farm_per_tick contract depends on — fish must
// never leak into the grain production figure, only into the net.
func TestFoundingGrainNetPerTick_FishNeverAffectsGrainProduction(t *testing.T) {
	pop := 1500
	prodNoFish, _ := FoundingGrainNetPerTick(2.4, 6.0, 0, pop, true)
	prodWithFish, _ := FoundingGrainNetPerTick(2.4, 6.0, 20.0, pop, true)
	if prodNoFish != prodWithFish {
		t.Fatalf("fishBase must not affect prodPerTick: no-fish=%v, with-fish=%v", prodNoFish, prodWithFish)
	}
}
