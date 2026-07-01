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
	expected := 2 * TerrainMoveHours("coastal_sea")
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
// TerrainMoveHours for each tile entered on a clear straight-line path.
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
	expected := 2 * TerrainMoveHours("plains")
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

// TestFindPath_HeuristicAdmissibility_NavalDetourCheaperThanDirect is a regression
// test for R2 (megaron_todo §9): the A* heuristic must use HexDistance × minCost
// (0.4 for naval, the cost of coastal_sea) rather than HexDistance × 1.0.
//
// This map is adversarial against the un-scaled heuristic: a straight 3-hex route
// crosses deep_sea (0.7/hex, total 1.8 incl. the shared target hex) while a longer
// 4-hex detour stays on coastal_sea (0.4/hex, total 1.6). The detour is truly
// cheaper despite being longer. With heuristic × 1.0 (the a2daf11 regression),
// A* pops the direct route's goal node (f=1.8) before ever expanding the detour
// far enough to discover it costs less — it returns the suboptimal 1.8 route.
// With the admissible × minCost heuristic, A* correctly returns the 1.6 detour.
func TestFindPath_HeuristicAdmissibility_NavalDetourCheaperThanDirect(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "coastal_sea", // origin
		{1, 0}: "deep_sea",    // direct route
		{2, 0}: "deep_sea",    // direct route
		{3, 0}: "coastal_sea", // target (shared final hex)
		{0, 1}: "coastal_sea", // detour
		{1, 1}: "coastal_sea", // detour
		{2, 1}: "coastal_sea", // detour
	}

	origin := MapPosition{Q: 0, R: 0}
	target := MapPosition{Q: 3, R: 0}

	path, cost, ok := findPath(tiles, origin, target, "naval")
	if !ok {
		t.Fatal("expected a naval path to be found")
	}

	const wantCost = 1.6 // 3×0.4 (detour intermediates) + 0.4 (target) — NOT 1.8 (2×0.7 + 0.4 direct)
	if math.Abs(cost-wantCost) > 1e-9 {
		t.Errorf("expected optimal cost %.2f (cheaper detour), got %.2f — heuristic is not admissible", wantCost, cost)
	}
	if len(path) != 5 {
		t.Errorf("expected the 5-hex detour path, got length %d: %v", len(path), path)
	}
}
