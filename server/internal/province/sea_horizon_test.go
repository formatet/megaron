package province

import "testing"

// The open horizon belongs to whoever stands at the water (Timothy 2026-08-04:
// "där har vi ett designfel idag"). Before 2026-08-05 LiveRadius returned 4 for
// any sea tile before it read the eye at all, so a land unit deep inland saw
// every sea hex within 4. These tests pin the amended rule; the sea radius
// itself (4) and the land vantages (3/2/1) are unchanged and live in hex_test.go.

// TestLiveRadius_InlandEyeReadsSeaAtItsLandVantage is the exact regression: an
// eye that does NOT stand at water gets no open horizon over the sea.
func TestLiveRadius_InlandEyeReadsSeaAtItsLandVantage(t *testing.T) {
	cases := []struct {
		kind string
		want int
	}{
		{EyeSettlement, 3},
		{EyeLandUnit, 2},
		{EyeNomadicHost, 1},
	}
	for _, c := range cases {
		for _, terrain := range []string{"coastal_sea", "deep_sea"} {
			if got := LiveRadius(c.kind, false, terrain); got != c.want {
				t.Errorf("inland %s looking at %s: LiveRadius = %d, want %d (its land vantage, not the sea's 4)",
					c.kind, terrain, got, c.want)
			}
		}
	}
}

// TestLiveRadius_EyeAtWaterKeepsTheOpenHorizon verifies the rule did not simply
// shrink everyone: a coastal city or a unit on the shore still sees 4 over sea.
func TestLiveRadius_EyeAtWaterKeepsTheOpenHorizon(t *testing.T) {
	for _, kind := range []string{EyeSettlement, EyeLandUnit, EyeNomadicHost, EyeShip} {
		for _, terrain := range []string{"coastal_sea", "deep_sea"} {
			if got := LiveRadius(kind, true, terrain); got != 4 {
				t.Errorf("%s at the water looking at %s: LiveRadius = %d, want 4", kind, terrain, got)
			}
		}
	}
}

// TestLiveRadius_ShipCarriesItsOwnWater verifies a naval eye needs no lookup: it
// floats on the sea by definition, so even a hand-built Eye{Kind: EyeShip} with
// the zero AtWater still gets the open horizon.
func TestLiveRadius_ShipCarriesItsOwnWater(t *testing.T) {
	if got := LiveRadius(EyeShip, false, "deep_sea"); got != 4 {
		t.Errorf("ship with AtWater unset: LiveRadius = %d, want 4", got)
	}
}

// TestLiveRadius_AtWaterDoesNotWidenLandOrMountain verifies the condition is
// scoped to sea targets only — standing on a beach does not help you see inland.
func TestLiveRadius_AtWaterDoesNotWidenLandOrMountain(t *testing.T) {
	for _, terrain := range []string{"plains", "mountain_limestone"} {
		dry := LiveRadius(EyeLandUnit, false, terrain)
		wet := LiveRadius(EyeLandUnit, true, terrain)
		if dry != wet {
			t.Errorf("target %s: inland eye sees %d but eye at water sees %d — AtWater must only affect sea targets",
				terrain, dry, wet)
		}
	}
}

// TestAnyEyeSees_InlandArmyDoesNotSeeDistantSea is the player-visible form of the
// regression, through the gate the handlers actually call.
func TestAnyEyeSees_InlandArmyDoesNotSeeDistantSea(t *testing.T) {
	inland := []Eye{{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeLandUnit}} // AtWater false
	seaFar := MapPosition{Q: 4, R: 0}                                  // distance 4

	if AnyEyeSees(inland, seaFar, "coastal_sea") {
		t.Error("an inland army must not see a sea hex 4 away")
	}

	shore := []Eye{{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeLandUnit, AtWater: true}}
	if !AnyEyeSees(shore, seaFar, "coastal_sea") {
		t.Error("an army on the shore must still see 4 hexes out over the sea")
	}
}

// --- markAtWater (the pure half of the map lookup) ---------------------------

// TestMarkAtWater_OnSeaAndBesideSea verifies both ways an eye stands at water:
// on a sea hex (a ship) and on a land hex with a sea neighbour (a coastal city).
func TestMarkAtWater_OnSeaAndBesideSea(t *testing.T) {
	// (0,0) is sea; (1,0) is its land neighbour; (5,5) is far inland.
	sea := map[[2]int]bool{{0, 0}: true}

	eyes := []Eye{
		{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeShip},
		{Pos: MapPosition{Q: 1, R: 0}, Kind: EyeSettlement},
		{Pos: MapPosition{Q: 5, R: 5}, Kind: EyeLandUnit},
	}
	markAtWater(eyes, sea)

	if !eyes[0].AtWater {
		t.Error("an eye standing on a sea hex is at water")
	}
	if !eyes[1].AtWater {
		t.Error("a settlement with a sea neighbour is at water (coast is a property, not a terrain)")
	}
	if eyes[2].AtWater {
		t.Error("an eye 5 hexes inland is not at water")
	}
}

// TestMarkAtWater_RiverIsNotAnOpenHorizon verifies the river exclusion holds at
// the marking step too: the sea set carries only sea, so a river-side eye stays
// dry. This is why the query asks terrain for sea rather than reading
// map_tiles.coastal, which migration 101 widened to include river neighbours.
func TestMarkAtWater_RiverIsNotAnOpenHorizon(t *testing.T) {
	sea := map[[2]int]bool{} // the river hex at (1,0) is deliberately absent
	eyes := []Eye{{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeSettlement}}
	markAtWater(eyes, sea)
	if eyes[0].AtWater {
		t.Error("a river neighbour must not grant the open horizon")
	}
}

// TestMarkAtWater_EmptySeaLeavesEveryEyeDry pins the fail-closed default: if the
// lookup returns nothing (including a failed query), vision may shrink, never grow.
func TestMarkAtWater_EmptySeaLeavesEveryEyeDry(t *testing.T) {
	eyes := []Eye{
		{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeSettlement},
		{Pos: MapPosition{Q: 9, R: 3}, Kind: EyeLandUnit},
	}
	markAtWater(eyes, nil)
	for _, e := range eyes {
		if e.AtWater {
			t.Errorf("eye %+v: with no sea known, AtWater must stay false (fail closed)", e)
		}
	}
}
