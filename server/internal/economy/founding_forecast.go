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
