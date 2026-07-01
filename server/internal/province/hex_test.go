package province

import "testing"

// TestLiveRadius_LandByEyeKind verifies the base land radius per eye kind from
// temenos_synlighet.md: settlement 3, land-unit 2, ship 1 (on non-mountain land).
func TestLiveRadius_LandByEyeKind(t *testing.T) {
	cases := []struct {
		kind string
		want int
	}{
		{EyeSettlement, 3},
		{EyeLandUnit, 2},
		{EyeShip, 1},
	}
	for _, c := range cases {
		if got := LiveRadius(c.kind, "plains"); got != c.want {
			t.Errorf("LiveRadius(%q, plains) = %d, want %d", c.kind, got, c.want)
		}
	}
}

// TestLiveRadius_SeaIsAlwaysFour verifies every eye kind sees 4 hexes over open
// water (open water hides nothing), regardless of the eye's own vantage.
func TestLiveRadius_SeaIsAlwaysFour(t *testing.T) {
	for _, kind := range []string{EyeSettlement, EyeLandUnit, EyeShip} {
		for _, terrain := range []string{"coastal_sea", "deep_sea"} {
			if got := LiveRadius(kind, terrain); got != 4 {
				t.Errorf("LiveRadius(%q, %q) = %d, want 4", kind, terrain, got)
			}
		}
	}
}

// TestLiveRadius_MountainAddsTwo verifies mountains are landmarks visible +2 hexes
// further than the eye's base land radius (settlement 5, land-unit 4, ship 3).
func TestLiveRadius_MountainAddsTwo(t *testing.T) {
	cases := []struct {
		kind string
		want int
	}{
		{EyeSettlement, 5},
		{EyeLandUnit, 4},
		{EyeShip, 3},
	}
	for _, terrain := range []string{"mountain_limestone", "mountain_red"} {
		for _, c := range cases {
			if got := LiveRadius(c.kind, terrain); got != c.want {
				t.Errorf("LiveRadius(%q, %q) = %d, want %d", c.kind, terrain, got, c.want)
			}
		}
	}
}

// TestAnyEyeSees_ShipSeesFarOverSeaButNotInland verifies the ship's asymmetric
// vision from the design table: 4 hexes out to sea, only 1 hex inland.
func TestAnyEyeSees_ShipSeesFarOverSeaButNotInland(t *testing.T) {
	ship := Eye{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeShip}
	eyes := []Eye{ship}

	seaTile := MapPosition{Q: 4, R: 0} // distance 4
	if !AnyEyeSees(eyes, seaTile, "coastal_sea") {
		t.Error("ship should see 4 hexes out over sea")
	}

	landNear := MapPosition{Q: 1, R: 0} // distance 1
	if !AnyEyeSees(eyes, landNear, "plains") {
		t.Error("ship should see 1 hex inland")
	}

	landFar := MapPosition{Q: 2, R: 0} // distance 2
	if AnyEyeSees(eyes, landFar, "plains") {
		t.Error("ship should NOT see 2 hexes inland")
	}
}

// TestAnyEyeSees_SettlementLandRadiusThreeNotFour is the exact regression the
// flat radius-6 model previously collapsed: a settlement sees land at 3, not 4.
func TestAnyEyeSees_SettlementLandRadiusThreeNotFour(t *testing.T) {
	eyes := []Eye{{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeSettlement}}

	within := MapPosition{Q: 3, R: 0}
	if !AnyEyeSees(eyes, within, "plains") {
		t.Error("settlement should see land at distance 3")
	}

	beyond := MapPosition{Q: 4, R: 0}
	if AnyEyeSees(eyes, beyond, "plains") {
		t.Error("settlement should NOT see land at distance 4")
	}
}

// TestAnyEyeSees_LandUnitRadiusTwo verifies the locked default (land-unit = 2, not 1).
func TestAnyEyeSees_LandUnitRadiusTwo(t *testing.T) {
	eyes := []Eye{{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeLandUnit}}

	within := MapPosition{Q: 2, R: 0}
	if !AnyEyeSees(eyes, within, "plains") {
		t.Error("land-unit should see land at distance 2")
	}

	beyond := MapPosition{Q: 3, R: 0}
	if AnyEyeSees(eyes, beyond, "plains") {
		t.Error("land-unit should NOT see land at distance 3")
	}
}

// TestHexNeighbors_ReturnsSixAdjacent sanity-checks the frontier helper: all 6
// neighbours are at hex distance 1.
func TestHexNeighbors_ReturnsSixAdjacent(t *testing.T) {
	pos := MapPosition{Q: 5, R: 5}
	neighbors := HexNeighbors(pos)
	if len(neighbors) != 6 {
		t.Fatalf("expected 6 neighbours, got %d", len(neighbors))
	}
	seen := map[MapPosition]bool{}
	for _, n := range neighbors {
		if HexDistance(pos, n) != 1 {
			t.Errorf("neighbour %+v is at distance %d, want 1", n, HexDistance(pos, n))
		}
		if seen[n] {
			t.Errorf("duplicate neighbour %+v", n)
		}
		seen[n] = true
	}
}
