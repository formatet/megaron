package province

// TerrainMoveHours returns hours per hex for an army marching through the given terrain.
// Messengers travel at 0.5×, trade caravans at 1.5×.
func TerrainMoveHours(terrain string) float64 {
	switch terrain {
	case "plains":
		return 0.75
	case "coast":
		return 0.8
	case "sea":
		return 0.5 // ships
	case "forest":
		return 1.5
	case "hills":
		return 1.25
	case "mountain":
		return 2.0
	default:
		return 1.0
	}
}
