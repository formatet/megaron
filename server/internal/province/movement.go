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
	if category == CategoryWarChariot {
		return chariotTerrainTicks(terrain)
	}
	if category == "naval" && terrain == "river_ford" {
		return 1
	}
	return TerrainMoveTicks(terrain)
}

// CategoryWarChariot is the price class of a war chariot (megaron_plan_vagar.md
// V3, Timothy 2026-10-12 "snabba på slätt och väg, långsamma i skog och berg").
// It is passable exactly like "land" (mountains stay closed) but has its own
// terrain table. Its string equals unit.MoveCategory(TypeWarChariot); province
// cannot import unit (G1), so a combat test pins that they agree.
const CategoryWarChariot = "war_chariot"

// chariotTerrainTicks is the ONE chariot terrain table: fast on the flat,
// slow in scrub, hills, forest and desert. Terrain the plan does not list
// (river_ford) is priced as for foot soldiers.
func chariotTerrainTicks(terrain string) float64 {
	switch terrain {
	case "plains", "river_valley", "river_delta":
		return 0.67
	case "forest_olive_grove", "scrub_maquis":
		return 2
	case "hills", "forest_cedar", "semi_desert":
		return 3
	}
	return TerrainMoveTicks(terrain)
}
