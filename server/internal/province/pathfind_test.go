package province

import (
	"math"
	"testing"
)

// TestFindPath_LandAroundMountain verifies that a land unit routes around a
// mountain wall rather than teleporting through it.
//
// Map layout (axial q,r):
//
//	(0,0) plains ── (1,0) mountain ── (2,0) plains
//	           ╲         (blocked)
//	         (0,1) plains ── (1,1) plains
//	                                    ╲
//	                                  (2,0) is also a neighbour of (1,1)
//
// Direct route 0,0 → 1,0 → 2,0 is blocked. The detour is (0,0)→(0,1)→(1,1)→(2,0).
func TestFindPath_LandAroundMountain(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "mountain_limestone", // wall
		{2, 0}: "plains",
		{0, 1}: "plains",
		{1, 1}: "plains",
		{2, 1}: "plains",
	}

	origin := MapPosition{Q: 0, R: 0}
	target := MapPosition{Q: 2, R: 0}

	path, cost, ok := findPath(tiles, origin, target, "land")
	if !ok {
		t.Fatal("expected a land path around the mountain; got ok=false")
	}
	if path[0] != origin {
		t.Errorf("path[0] should be origin %v, got %v", origin, path[0])
	}
	if path[len(path)-1] != target {
		t.Errorf("path[-1] should be target %v, got %v", target, path[len(path)-1])
	}
	// The path must not include the mountain tile.
	for _, p := range path {
		if p == (MapPosition{Q: 1, R: 0}) {
			t.Error("path must not traverse mountain_limestone at (1,0)")
		}
	}
	if cost <= 0 {
		t.Errorf("expected positive cost, got %f", cost)
	}
}

// TestFindPath_Naval verifies that a naval unit travels over sea tiles.
func TestFindPath_Naval(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "coastal_sea",
		{1, 0}: "coastal_sea",
		{2, 0}: "coastal_sea",
	}

	path, cost, ok := findPath(tiles, MapPosition{Q: 0, R: 0}, MapPosition{Q: 2, R: 0}, "naval")
	if !ok {
		t.Fatal("expected naval path over sea; got ok=false")
	}
	if path[0] != (MapPosition{Q: 0, R: 0}) {
		t.Errorf("expected path start (0,0), got %v", path[0])
	}
	if path[len(path)-1] != (MapPosition{Q: 2, R: 0}) {
		t.Errorf("expected path end (2,0), got %v", path[len(path)-1])
	}
	// Cost: entering (1,0) + entering (2,0) = 0.4 + 0.4 = 0.8
	expected := 2 * TerrainMoveTicks("coastal_sea")
	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("expected cost %f, got %f", expected, cost)
	}
}

// TestFindPath_LandBlockedBySea verifies that a land unit returns ok=false when
// the only connecting path between origin and target crosses sea tiles.
func TestFindPath_LandBlockedBySea(t *testing.T) {
	// (0,0) plains — (1,0) coastal_sea (blocks land) — (2,0) plains
	// There is no land bridge: in axial coords, the only neighbours of (0,0) that
	// exist in the tile map are (1,0) [sea]. So (2,0) is unreachable for land.
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "coastal_sea",
		{2, 0}: "plains",
	}

	_, _, ok := findPath(tiles, MapPosition{Q: 0, R: 0}, MapPosition{Q: 2, R: 0}, "land")
	if ok {
		t.Error("expected ok=false: land unit cannot cross sea")
	}
}

// TestFindPath_StraightLandCost verifies that the returned cost equals the sum of
// TerrainMoveTicks for each tile entered on a clear straight-line path.
func TestFindPath_StraightLandCost(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "plains",
		{2, 0}: "plains",
	}

	path, cost, ok := findPath(tiles, MapPosition{Q: 0, R: 0}, MapPosition{Q: 2, R: 0}, "land")
	if !ok {
		t.Fatal("expected straight land path to be found")
	}
	// Entering (1,0) and (2,0): 2 × 0.75 = 1.5
	expected := 2 * TerrainMoveTicks("plains")
	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("expected cost %f, got %f", expected, cost)
	}
	if len(path) != 3 {
		t.Errorf("expected path length 3 (origin + 2 steps), got %d: %v", len(path), path)
	}
	if path[0] != (MapPosition{Q: 0, R: 0}) || path[1] != (MapPosition{Q: 1, R: 0}) || path[2] != (MapPosition{Q: 2, R: 0}) {
		t.Errorf("unexpected path order: %v", path)
	}
}

// RÖ1 makes a longer deep-sea detour cheaper than the direct coastal route.
// The old coastal heuristic (1/3) pops the direct goal too early (cost 1).
func TestFindPath_HeuristicAdmissibility_NavalDetourCheaperThanDirect(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "coastal_sea", {1, 0}: "coastal_sea", {2, 0}: "coastal_sea", {3, 0}: "coastal_sea",
		{0, 1}: "deep_sea", {1, 1}: "deep_sea", {2, 1}: "deep_sea",
	}
	path, cost, ok := findPath(tiles, MapPosition{}, MapPosition{Q: 3}, "naval")
	if !ok {
		t.Fatal("expected naval detour")
	}
	const want = 3.0/5 + 1.0/3
	if math.Abs(cost-want) > 1e-9 {
		t.Fatalf("naval heuristic missed cheaper deep-sea detour: cost %v, want %v", cost, want)
	}
	if len(path) != 5 {
		t.Fatalf("want four-step deep-sea detour, got %v", path)
	}
}

// TestFindPath_RiverBlocksLand is megaron_floden_plan.md's A2: a river is a
// wall for land units and a lane for ships and couriers (Timothy 2026-07-29).
//
// Map layout (axial q,r) — a straight river with no land bridge anywhere in
// the tile set, so a land unit has no route at all, not merely a longer one:
//
//	(0,0) plains ── (1,0) river ── (2,0) river ── (3,0) plains
func TestFindPath_RiverBlocksLand(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "river",
		{2, 0}: "river",
		{3, 0}: "plains",
	}
	landOrigin := MapPosition{Q: 0, R: 0}
	landTarget := MapPosition{Q: 3, R: 0}

	if _, _, ok := findPath(tiles, landOrigin, landTarget, "land"); ok {
		t.Error("expected ok=false: a land unit cannot cross a river, and no route around exists in this tile set")
	}

	// Naval origin/target must themselves be water — a ship cannot stand on
	// the plains banks, only sail the river itself.
	riverOrigin := MapPosition{Q: 1, R: 0}
	riverTarget := MapPosition{Q: 2, R: 0}
	if _, _, ok := findPath(tiles, riverOrigin, riverTarget, "naval"); !ok {
		t.Error("expected a naval path straight up the river")
	}

	// A courier, unlike land or naval, can start and end on the land banks —
	// it commandeers a boat to cross the river in between, the same as it
	// would cross the sea.
	if _, _, ok := findPath(tiles, landOrigin, landTarget, CategoryCourier); !ok {
		t.Error("expected a courier path across the river — a runner commandeers a boat over a river same as over the sea")
	}
}

// TestFindPath_RiverBlocksLand_DetourAround covers the plan's "or a route
// around" alternative explicitly: when a land bridge DOES exist elsewhere on
// the map, a land unit takes it instead of failing outright — the river only
// blocks the direct crossing, not the whole map.
func TestFindPath_RiverBlocksLand_DetourAround(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "river", // direct crossing — blocked for land
		{2, 0}: "plains",
		{0, 1}: "plains",
		{1, 1}: "plains", // detour around the river's end
		{2, 1}: "plains",
	}
	origin := MapPosition{Q: 0, R: 0}
	target := MapPosition{Q: 2, R: 0}

	path, _, ok := findPath(tiles, origin, target, "land")
	if !ok {
		t.Fatal("expected a land route around the river")
	}
	for _, p := range path {
		if p == (MapPosition{Q: 1, R: 0}) {
			t.Error("path must not cross the river hex at (1,0)")
		}
	}
}

// TestStepHours_SumMatchesFindPathCost is R2 (megaron_plan_rorelse_sparad_vag.md):
// StepHours must not re-search a path, only price one already found — and its
// per-step sum must equal FindPath's own total cost for that exact path, since
// both use the same moveHoursFor.
func TestStepHours_SumMatchesFindPathCost(t *testing.T) {
	cases := []struct {
		name     string
		tiles    map[[2]int]string
		origin   MapPosition
		target   MapPosition
		category string
	}{
		{
			name: "land around a mountain",
			tiles: map[[2]int]string{
				{0, 0}: "plains", {1, 0}: "mountain_limestone", {2, 0}: "plains",
				{0, 1}: "plains", {1, 1}: "plains", {2, 1}: "plains",
			},
			origin: MapPosition{Q: 0, R: 0}, target: MapPosition{Q: 2, R: 0}, category: "land",
		},
		{
			name: "naval detour",
			tiles: map[[2]int]string{
				{0, 0}: "coastal_sea", {1, 0}: "deep_sea", {2, 0}: "deep_sea",
				{0, 1}: "coastal_sea", {1, 1}: "coastal_sea", {2, 1}: "coastal_sea",
			},
			origin: MapPosition{Q: 0, R: 0}, target: MapPosition{Q: 2, R: 0}, category: "naval",
		},
		{
			name: "courier over a river",
			tiles: map[[2]int]string{
				{0, 0}: "plains", {1, 0}: "river", {2, 0}: "plains",
			},
			origin: MapPosition{Q: 0, R: 0}, target: MapPosition{Q: 2, R: 0}, category: CategoryCourier,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := TileGraph(c.tiles)
			path, wantCost, ok := g.FindPath(c.origin, c.target, c.category, nil)
			if !ok {
				t.Fatalf("FindPath did not find a route")
			}
			steps := g.StepHours(path, c.category, nil)
			if len(steps) != len(path)-1 {
				t.Fatalf("StepHours returned %d steps, want %d (len(path)-1)", len(steps), len(path)-1)
			}
			var sum float64
			for _, h := range steps {
				sum += h
			}
			if math.Abs(sum-wantCost) > 1e-9 {
				t.Errorf("sum(StepHours) = %f, want FindPath cost %f", sum, wantCost)
			}
		})
	}
}

// TestStepHours_TooShortPath: a path with fewer than two hexes (nothing to
// enter) returns nil, not a search or an error.
func TestStepHours_TooShortPath(t *testing.T) {
	g := TileGraph{{0, 0}: "plains"}
	if got := g.StepHours([]MapPosition{{Q: 0, R: 0}}, "land", nil); got != nil {
		t.Errorf("StepHours(single-hex path) = %v, want nil", got)
	}
	if got := g.StepHours(nil, "land", nil); got != nil {
		t.Errorf("StepHours(nil path) = %v, want nil", got)
	}
}
