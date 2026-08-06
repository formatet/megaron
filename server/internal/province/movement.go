package province

// TerrainMoveTicks returns ticks per hex for an army marching through the given terrain.
// Messengers travel at 0.5×, trade caravans at 1.5×.
func TerrainMoveTicks(terrain string) float64 {
	switch terrain {
	case "plains", "river_valley", "river_delta":
		return 0.75
	case "coastal_sea":
		return 0.4 // fast sailing near land
	case "river":
		return 0.5 // tunable: shallow and sheltered but narrow; between coastal_sea (0.4) and deep_sea (0.7)
	case "river_ford":
		// Steep on purpose (megaron_plan_flodbudget_och_vadstalle.md, Timothy
		// 2026-08-02: "stark rörelsekostnad") — bergsklass or above. A ship
		// pays this SAME rate too (moveHoursFor has no naval branch for it):
		// a ford is shallow and narrow water, a galley crawls through it just
		// as a land unit wades it, decision (a) in the plan's canon table.
		// Tunable, but never below hills/scrub (1.25) — a crossing point
		// must never be the CHEAP way through, only the only way through.
		return 2.5
	case "deep_sea":
		return 0.7 // slower open-sea sailing
	case "forest_olive_grove":
		return 1.5
	case "forest_cedar":
		// S2 (megaron_cederskogen_plan.md step 2): denser, road-less than the
		// olive grove — this is the forest where armies vanish (princip 8,
		// megaron_terrangrendering.md).
		return 2.0
	case "hills", "scrub_maquis":
		return 1.25
	case "mountain_limestone", "mountain_red", "semi_desert":
		return 2.0
	default:
		return 1.0
	}
}
