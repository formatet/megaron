package province

// CategoryCourier routing (temenos_orderlopare_plan.md Fas 4): runners
// run land at half a land unit's terrain hours (2× spearman speed) and route
// around mountains like land. Sea used to be a boat crossing at the flat
// CourierSeaTicks rate — the "abstract boat" — removed in
// megaron_plan_ordna_passage.md slice 3b-4 R3: a courier may only cross open
// sea aboard a real carrier now, never on its own.

import "testing"

// TestFindPath_CourierCannotCrossSeaAnymore is 3b-4's R3: a courier is walled
// out of sea exactly like a land unit — no more abstract boat. Was
// TestFindPath_CourierCrossesSeaByBoat before this slice, which pinned the
// now-removed behaviour.
func TestFindPath_CourierCannotCrossSeaAnymore(t *testing.T) {
	// island — strait — island: impassable for land, and now for courier too.
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "coastal_sea",
		{2, 0}: "plains",
	}
	if _, _, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{2, 0}, "land"); ok {
		t.Fatal("land unit crossed open sea — passability broken")
	}
	if _, _, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{2, 0}, CategoryCourier); ok {
		t.Fatal("courier crossed open sea on its own — the abstract boat must be gone (3b-4 R3); " +
			"only a real carrier (internal/messenger's sea-lift) may cross the sea now")
	}
}

// TestFindPath_CourierStillCrossesRiverByBoat: rivers are untouched by 3b-4
// (megaron_floden_plan.md, a separate, pre-existing design) — a courier still
// commandeers a boat over a plain river hex, at the same flat rate as before.
func TestFindPath_CourierStillCrossesRiverByBoat(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "river",
		{2, 0}: "plains",
	}
	if _, _, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{2, 0}, "land"); ok {
		t.Fatal("land unit crossed a plain river hex — passability broken")
	}
	path, cost, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{2, 0}, CategoryCourier)
	if !ok {
		t.Fatal("courier found no route across the river — river-boat passage broken")
	}
	if len(path) != 3 {
		t.Fatalf("courier path length = %d, want 3 (origin, river, target)", len(path))
	}
	// Cost: enter river (boat, 0.5) + enter plains at half land hours (0.75/2).
	want := CourierSeaTicks + TerrainMoveTicks("plains")/2
	if diff := cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("courier cost = %v, want %v (river boat rate + half plains hours)", cost, want)
	}
}

func TestFindPath_CourierRoutesAroundMountains(t *testing.T) {
	// Straight line blocked by a mountain; a plains detour exists below it.
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "mountain_limestone",
		{2, 0}: "plains",
		{0, 1}: "plains",
		{1, 1}: "plains", // detour via (0,1)? axial neighbours: (1,0)+(0,1) reach (1,1)
		{2, 1}: "plains",
	}
	path, _, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{2, 0}, CategoryCourier)
	if !ok {
		t.Fatal("courier found no route around the mountain")
	}
	for _, p := range path {
		if tiles[[2]int{p.Q, p.R}] == "mountain_limestone" {
			t.Fatalf("courier path enters a mountain hex (%d,%d) — mountains must be routed around", p.Q, p.R)
		}
	}
	if _, _, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{1, 0}, CategoryCourier); ok {
		t.Fatal("courier entered a mountain target — mountains must be impassable for couriers")
	}
}
