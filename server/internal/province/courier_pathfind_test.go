package province

// CategoryCourier routing (temenos_orderlopare_plan.md Fas 4): hemerodromoi
// run land at half a land unit's terrain hours (2× spearman speed), cross sea
// at the flat boat rate CourierSeaHours, and route around mountains like land.

import "testing"

func TestFindPath_CourierCrossesSeaByBoat(t *testing.T) {
	// island — strait — island: impassable for land, boat passage for courier.
	tiles := map[[2]int]string{
		{0, 0}: "plains",
		{1, 0}: "coastal_sea",
		{2, 0}: "plains",
	}
	if _, _, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{2, 0}, "land"); ok {
		t.Fatal("land unit crossed open sea — passability broken")
	}
	path, cost, ok := findPath(tiles, MapPosition{0, 0}, MapPosition{2, 0}, CategoryCourier)
	if !ok {
		t.Fatal("courier found no route across the strait — boat passage broken")
	}
	if len(path) != 3 {
		t.Fatalf("courier path length = %d, want 3 (origin, sea, target)", len(path))
	}
	// Cost: enter sea (boat, 0.5) + enter plains at half land hours (0.75/2).
	want := CourierSeaHours + TerrainMoveHours("plains")/2
	if diff := cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("courier cost = %v, want %v (sea boat rate + half plains hours)", cost, want)
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
