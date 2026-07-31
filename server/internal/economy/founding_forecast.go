package economy

// FoundingGrainLaborWeight is the grain labor weight the colonize/settle forecast
// assumes. foundColony seeds 'grain' at exactly this (unit_arrival.go). A
// metropolis now seeds grain at 1.0 (create_metropolis.go — the old cult floor
// folded into grain once starter buildings were removed), so the forecast is a
// deliberately conservative ~15% under-estimate of a metropolis's opening grain
// rate: the founding labor seed is transient (the Wanax reallocates immediately),
// so the prognosis does not chase it — and under-predicting grain errs safe. The
// A13 forecast fix (building-free vs with-farm scenario) is orthogonal to this
// weight. KEEP IN SYNC with the 0.85 literal in unit_arrival.go.
const FoundingGrainLaborWeight = 0.85

// FoundingGrainNetPerTick mirrors RecomputeProduction's food-invariant math
// (grain first, fish for the remainder — FoodConsumptionSplit) for a hex that
// has NOT been settled yet, so the colonize/settle preview can forecast the
// real net grain rate a founding would get. buildingFreeBase and withFarmBase
// are the catchment grain base_potential without/with a farm (from
// CatchmentBasePotentialAt). fishBase is the catchment's fish base_potential
// (same source, fish is never building-gated for this purpose — every fish
// production_rule since mig 101 matches with no building assumption needed).
// starterFarm=true for a metropolis founding (gets a starter farm), false for
// a colony (builds its own farm later).
//
// fishBase is labor-scaled with the SAME conservative FoundingGrainLaborWeight
// as grain, not a second invented constant: neither the colony seed (0.85
// grain / 0.15 cult, unit_arrival.go) nor the metropolis seed (1.0 grain,
// create_metropolis.go) allocates any opening weight to fish — a Wanax must
// reallocate manually after founding, exactly the "reallocates immediately"
// case FoundingGrainLaborWeight already exists to under-estimate for grain
// (see its own doc comment). This is why AK5's colonize-preview and keryx
// numbers should be read as the catchment's food POTENTIAL once labor is
// sensibly allocated — same spirit as "prognos + spelarval, INTE garanti"
// (CLAUDE.md) — not the literal opening-tick production, which is 0 for fish
// either way until the Wanax allocates it.
//
// Returns prodPerTick = grain's OWN production (unaffected by fish, so the
// existing with_farm_per_tick "production footing" contract in
// api/handlers/world.go/ColonizePreview is untouched) and netPerTick = grain's
// net rate after the food invariant (production − consumption, with fish
// covering whatever grain does not reach).
func FoundingGrainNetPerTick(buildingFreeBase, withFarmBase, fishBase float64, pop int, starterFarm bool) (prodPerTick, netPerTick float64) {
	prodBase := buildingFreeBase
	if starterFarm {
		prodBase = withFarmBase
	}
	effectiveWorkers := FoundingGrainLaborWeight * float64(pop)
	grainProd := (prodBase / REF_LABOR) * effectiveWorkers
	fishProd := (fishBase / REF_LABOR) * effectiveWorkers
	grainNet, _ := FoodConsumptionSplit(GrainConsumptionPerTick(pop), grainProd, fishProd)
	return grainProd, grainNet
}
