package province

import (
	"math"
	"testing"
)

// TestFindPath_RiverFordIsTheOnlyLandCrossing is A2 of
// megaron_plan_flodbudget_och_vadstalle.md (Timothy 2026-08-02): the river's
// port. Without a ford the river is a total wall for land units (as
// TestFindPath_RiverBlocksLand already covers); with exactly one river_ford
// hex added to the same wall, a land unit gets a route THROUGH that one hex,
// paying its steep TerrainMoveTicks cost, and a naval unit still gets a route
// through both the plain river hexes and the ford (a ford is water too — a
// ship does not need the ford, but must not be blocked by it either).
//
// Map layout (axial q,r) — a straight 3-hex-wide river with no land bridge
// anywhere in the tile set except through the ford at (1,0):
//
//	(0,0) plains ── (1,0) river_ford ── (2,0) plains
//	(0,1) plains ── (1,1) river      ── (2,1) plains
//	(0,2) plains ── (1,2) river      ── (2,2) plains
func TestFindPath_RiverFordIsTheOnlyLandCrossing(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains", {1, 0}: "river_ford", {2, 0}: "plains",
		{0, 1}: "plains", {1, 1}: "river", {2, 1}: "plains",
		{0, 2}: "plains", {1, 2}: "river", {2, 2}: "plains",
	}

	// Without the ford (river only, no bridge anywhere): land has no route at
	// all — same invariant TestFindPath_RiverBlocksLand already proves, cross-
	// checked here on a wider wall so this test's own fixture is trusted.
	riverOnly := map[[2]int]string{
		{0, 0}: "plains", {1, 0}: "river", {2, 0}: "plains",
		{0, 1}: "plains", {1, 1}: "river", {2, 1}: "plains",
		{0, 2}: "plains", {1, 2}: "river", {2, 2}: "plains",
	}
	if _, _, ok := findPath(riverOnly, MapPosition{Q: 0, R: 1}, MapPosition{Q: 2, R: 1}, "land"); ok {
		t.Fatal("fixture check failed: a river wall with no ford must block land entirely")
	}

	// With the ford: land gets a route, and it goes THROUGH the ford hex,
	// paying TerrainMoveTicks("river_ford") for it.
	origin := MapPosition{Q: 0, R: 1}
	target := MapPosition{Q: 2, R: 1}
	path, cost, ok := findPath(tiles, origin, target, "land")
	if !ok {
		t.Fatal("expected a land route through the river_ford at (1,0)")
	}
	crossedFord := false
	for _, p := range path {
		if p == (MapPosition{Q: 1, R: 0}) {
			crossedFord = true
		}
		if p == (MapPosition{Q: 1, R: 1}) || p == (MapPosition{Q: 1, R: 2}) {
			t.Errorf("path must never cross a plain river hex, got %v in path %v", p, path)
		}
	}
	if !crossedFord {
		t.Errorf("expected the route to pass through the ford at (1,0), got path %v", path)
	}
	fordCost := TerrainMoveTicks("river_ford")
	if fordCost <= TerrainMoveTicks("hills") {
		t.Errorf("river_ford (%.2f) must cost at least as much as hills/scrub (%.2f) — the port is a chokepoint, not a shortcut",
			fordCost, TerrainMoveTicks("hills"))
	}
	// The path must actually pay the ford's cost somewhere in its total —
	// a path shorter than "detour to the ford and back" would mean the
	// pathfinder found some other, illegitimate crossing.
	if cost < fordCost {
		t.Errorf("path cost %.2f is less than the ford's own entry cost %.2f — impossible for a route that must cross it", cost, fordCost)
	}

	// Naval: a ship sails through both plain river hexes and the ford — the
	// ford is deliberately NOT a naval-only shortcut (isPassable has no
	// special case for it), just ordinary passable water.
	if _, _, ok := findPath(tiles, MapPosition{Q: 1, R: 0}, MapPosition{Q: 1, R: 2}, "naval"); !ok {
		t.Error("expected a naval path down the river column, through the ford")
	}
}

// TestFindPath_RiverFordShipPaysSameCostAsLand is the plan's decision (a)
// (megaron_todo.md, Timothy: "bygg (a) om ingen säger annat" — canon table in
// the plan): a ship crossing a ford pays the SAME steep rate a land unit does,
// because moveHoursFor has no naval branch for river_ford — a ford is shallow
// and narrow water, a galley crawls through it too.
func TestFindPath_RiverFordShipPaysSameCostAsLand(t *testing.T) {
	landCost := moveHoursFor("river_ford", "land")
	navalCost := moveHoursFor("river_ford", "naval")
	if math.Abs(landCost-navalCost) > 1e-9 {
		t.Errorf("expected river_ford to cost the same for land and naval (decision a), got land=%.2f naval=%.2f", landCost, navalCost)
	}
}

// TestFindPath_RiverFordCourierWadesNotSails: a courier crossing a ford falls
// through to ordinary TerrainMoveTicks/2 (land rate), NOT the flat
// CourierSeaTicks boat rate — a runner wades a ford, he does not commandeer a
// boat for it (pathfind.go's moveHoursFor doc comment).
func TestFindPath_RiverFordCourierWadesNotSails(t *testing.T) {
	got := moveHoursFor("river_ford", CategoryCourier)
	want := TerrainMoveTicks("river_ford") / 2
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected courier river_ford cost = TerrainMoveTicks/2 = %.3f (wading), got %.3f (looks like the flat boat rate %.3f was used instead)",
			want, got, CourierSeaTicks)
	}
	if math.Abs(got-CourierSeaTicks) < 1e-9 {
		t.Error("courier paid the flat sea-boat rate for a ford crossing — it should wade, not sail")
	}
}
