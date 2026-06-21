package province

// TerrainMoveHours returns hours per hex for an army marching through the given terrain.
// Messengers travel at 0.5×, trade caravans at 1.5×.
func TerrainMoveHours(terrain string) float64 {
	switch terrain {
	case "plains", "river_valley", "river_delta":
		return 0.75
	case "coastal_sea":
		return 0.4 // fast sailing near land
	case "deep_sea":
		return 0.7 // slower open-sea sailing
	case "forest_olive_grove":
		return 1.5
	case "hills", "scrub_maquis":
		return 1.25
	case "mountain_limestone", "mountain_red", "semi_desert":
		return 2.0
	default:
		return 1.0
	}
}
