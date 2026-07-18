package economy

// FoundingGrainLaborWeight is the grain labor weight every founding seeds
// (createMetropolis + foundColony both seed 'grain' at 0.85). The
// colonize/settle forecast reuses it so the prognosis tracks the real
// founding rate. KEEP IN SYNC with the 0.85 literal in create_metropolis.go
// and unit_arrival.go.
const FoundingGrainLaborWeight = 0.85

// FoundingGrainNetPerTick mirrors RecomputeProduction's grain math for a hex
// that has NOT been settled yet, so the colonize/settle preview can forecast
// the real net grain rate a founding would get. buildingFreeBase and
// withFarmBase are the catchment grain base_potential without/with a farm
// (from CatchmentBasePotentialAt). starterFarm=true for a metropolis founding
// (gets a starter farm), false for a colony (builds its own farm later).
// Returns production and net (production − consumption) per tick.
func FoundingGrainNetPerTick(buildingFreeBase, withFarmBase float64, pop int, starterFarm bool) (prodPerTick, netPerTick float64) {
	prodBase := buildingFreeBase
	if starterFarm {
		prodBase = withFarmBase
	}
	effectiveWorkers := FoundingGrainLaborWeight * float64(pop)
	prodPerTick = (prodBase / REF_LABOR) * effectiveWorkers
	netPerTick = prodPerTick - GrainConsumptionPerTick(pop)
	return
}
