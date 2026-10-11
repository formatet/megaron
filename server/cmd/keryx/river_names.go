package main

func namedRiverTerrain(terrain, name string) string {
	if name == "" {
		return terrain
	}
	switch terrain {
	case "river":
		return "River " + name
	case "river_ford":
		return "Ford of " + name
	case "river_delta":
		return "Delta of " + name
	default:
		return terrain
	}
}
