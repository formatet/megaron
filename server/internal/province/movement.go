package province

// TerrainMoveTicks returns base ticks per entered hex (RÖ1, Timothy 2026-10-12).
// Passability is owned by isPassable; mountains remain impassable.
// Unit speed and caravan factors are applied by their respective planners.
func TerrainMoveTicks(terrain string) float64 {
	switch terrain {
	case "plains", "river_valley", "river_delta":
		return 1
	case "coastal_sea":
		return 1.0 / 3
	case "deep_sea":
		return 1.0 / 5
	case "river":
		return 0.5
	case "river_ford":
		return 2.5 // land crossing; ships use TerrainMoveTicksFor
	case "forest_olive_grove", "scrub_maquis":
		return 1.5
	case "hills", "forest_cedar":
		return 2
	case "semi_desert":
		return 3
	case "mountain_limestone", "mountain_red":
		return 2 // never traversed: the passability graph excludes mountains
	default:
		return 1
	}
}

// TerrainMoveTicksFor is the category cost shared by A*, saved steps and
// defensive return estimates. A ford has one land cost and one ship branch.
func TerrainMoveTicksFor(terrain, category string) float64 {
	if category == "naval" && terrain == "river_ford" {
		return 1
	}
	return TerrainMoveTicks(terrain)
}
