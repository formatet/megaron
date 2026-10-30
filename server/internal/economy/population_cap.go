package economy

// PopulationCap is the ONE owner of how many people a city holds: the wall is
// the ceiling (Timothy 2026-10-21, megaron_plan_budvagar_och_murtak §B0). A city
// at its cap does not shrink — it only stops growing. Walls are bound to the
// Granary (province.SiloLevelForWall), so a bigger ceiling needs bigger stores.
//
// Surfaces show only the cap ("Walls hold 10 000"), never a "room for N more".
var populationCaps = [4]int{2000, 5000, 10000, 20000}

// PopulationCap returns the ceiling for a wall level (0 = no wall; clamped to 0..3).
func PopulationCap(wallLevel int) int {
	if wallLevel < 0 {
		wallLevel = 0
	}
	if wallLevel >= len(populationCaps) {
		wallLevel = len(populationCaps) - 1
	}
	return populationCaps[wallLevel]
}
