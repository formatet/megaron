package province

// Water classification — the one owner (insjöar slice 1, 2026-10-30). Every
// "is this hex water?" question outside mapgen asks here, or the SQL mirror
// terrain_is_sea / terrain_is_navigable (migration 231), or the client mirror
// web/static/js/megaron/terrain.js. A new water terrain is added in those three
// places and nowhere else; water_guard_test.go fails on a new hardcoded list.
// IsSea (open water) lives in hex.go.

// IsThalassa reports whether terrain is the open sea itself: coastal or deep
// water connected to the world's edge. A lake is not — no storm is born on
// one or drifts into one, and a shipyard needs a waterway to the Thalassa.
func IsThalassa(terrain string) bool {
	return terrain == "coastal_sea" || terrain == "deep_sea"
}

// IsShallowWater reports whether terrain is shallow open water: coastal water
// or a lake. A patrol holds only here, and an exploring ship maps it first.
func IsShallowWater(terrain string) bool {
	return terrain == "coastal_sea" || terrain == "lake"
}

// IsNavigable reports whether a ship can float on terrain: open sea, river and
// ford. The same set is never worked as land — a catchment hex of it yields fish only.
func IsNavigable(terrain string) bool {
	return IsSea(terrain) || terrain == "river" || terrain == "river_ford"
}

// IsWaterBarrier reports whether terrain is water a land unit cannot step into:
// open sea and river. river_ford is the one deliberate gap in the wall
// (megaron_plan_flodbudget_och_vadstalle.md, Timothy 2026-08-02).
func IsWaterBarrier(terrain string) bool {
	return IsSea(terrain) || terrain == "river"
}
