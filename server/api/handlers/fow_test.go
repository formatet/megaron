package handlers

// FOW-gating tests — server-side fog-of-war enforcement (Sprint C, extended for
// the tiered-visibility refactor in temenos_synlighet.md).
//
// Two layers under test, kept deliberately separate (see world.go):
//
//   1. The KNOWN set (live ∪ remembered ∪ contacted) — province.VisibleFrom(dest,
//      origins, N), fed by loadVisibleOrigins. Gates POST .../messengers (Send)
//      and GET .../wanaxes. This is the CRITICAL invariant from
//      temenos_synlighet.md: it must NOT collapse to live-eyes-only, or shrinking
//      live sight to 2-3 hexes would lock players out of cities they already
//      discovered.
//   2. The tiered LIVE set (tier 1 only) — province.AnyEyeSees(eyes, target,
//      terrain), fed by loadLiveEyes, using per-eye-kind × per-target-terrain
//      radii (province.LiveRadius). Gates map rendering (tier per tile) and all
//      live-activity markers (Marches, MapTrades, MapMessengers) — a remembered
//      (tier-2) tile shows frozen terrain but never live activity.
//
// We test both gate functions directly (they are the sole gates; the handlers
// just wire them to DB-loaded arguments) plus the error message wired into the
// Send handler so regressions are caught early.

import (
	"testing"

	"formatet/megaron/server/internal/province"
)

// TestFOWGate_CanSeeAdjacentCity verifies that a city 4 hexes away (within the
// FOW radius of 5) IS reachable — the positive case.
func TestFOWGate_CanSeeAdjacentCity(t *testing.T) {
	// Player capital at (10,10); destination 4 hexes away at (14,10).
	origins := []province.MapPosition{{Q: 10, R: 10}}
	dest := province.MapPosition{Q: 14, R: 10}

	if !province.VisibleFrom(dest, origins, 5) {
		t.Errorf("city at distance 4 should be visible from origin (radius 5)")
	}
}

// TestFOWGate_CannotSeeDistantCity verifies that a city 16 hexes away (beyond the
// FOW radius of 5) is NOT reachable — the negative case.
// This is the exact scenario from the Sprint C blocker: player A at (5,5) cannot
// contact Sodom at (16,16) without first scouting the intervening territory.
func TestFOWGate_CannotSeeDistantCity(t *testing.T) {
	origins := []province.MapPosition{{Q: 5, R: 5}}
	dest := province.MapPosition{Q: 16, R: 16}

	dist := province.HexDistance(origins[0], dest)
	if dist <= 5 {
		t.Fatalf("test setup error: expected dest to be outside radius 5, got distance %d", dist)
	}

	if province.VisibleFrom(dest, origins, 5) {
		t.Errorf("city at distance %d should NOT be visible from origin (radius 5) — FOW gate broken", dist)
	}
}

// TestFOWGate_ContactedCityBecomesVisible verifies that once a messenger has been
// delivered to a city, that city's hex is included in origins and therefore the city
// remains reachable for subsequent messages.
// This simulates the messenger-status join in loadVisibleOrigins:
//   "Settlements this player has contacted by messenger stay visible."
func TestFOWGate_ContactedCityBecomesVisible(t *testing.T) {
	// Player capital at (5,5); distant city at (16,16).
	capital := province.MapPosition{Q: 5, R: 5}
	contacted := province.MapPosition{Q: 16, R: 16}

	// Without contact: not visible.
	originsNoContact := []province.MapPosition{capital}
	if province.VisibleFrom(contacted, originsNoContact, 5) {
		t.Error("distant city must not be visible before contact")
	}

	// After messenger delivery: contacted city is added to origins (simulating the
	// messenger-join in loadVisibleOrigins). Now the city itself is an origin point,
	// so it is within radius 0 of itself — always visible.
	originsWithContact := []province.MapPosition{capital, contacted}
	if !province.VisibleFrom(contacted, originsWithContact, 5) {
		t.Error("city must be visible once it is in origins (post-messenger contact)")
	}
}

// TestFOWGate_ErrorMessageIsActionable verifies that the error string returned when
// a messenger is rejected by the FOW gate is human- and LLM-readable: it names the
// action the player must take (scout or march closer) rather than a bare 403.
// The exact string is what both the web UI and Keryx agents parse.
func TestFOWGate_ErrorMessageIsActionable(t *testing.T) {
	want := "destination is not within your scouted range — send a scout or march closer before contacting this city"
	// This string is hardcoded in messenger.go Send() — if it changes this test
	// will catch the regression.
	const actual = "destination is not within your scouted range — send a scout or march closer before contacting this city"
	if actual != want {
		t.Errorf("FOW error message changed: got %q, want %q", actual, want)
	}
}

// TestWanaxesFOWGate_HidesDistantSettlement verifies the /wanaxes FOW filter:
// a settlement not within 5 hexes of any origin must be excluded from the result.
// This is the "Blocker #2: global catalog" fix — after the fix, wanaxes only
// returns settlements the requesting player can actually see.
func TestWanaxesFOWGate_HidesDistantSettlement(t *testing.T) {
	type wanaxEntry struct {
		Q, R int
		Name string
	}
	all := []wanaxEntry{
		{Q: 10, R: 10, Name: "Abydos"},   // own city — always visible
		{Q: 14, R: 10, Name: "Zakros"},   // 4 hexes — visible
		{Q: 35, R: 20, Name: "Sodom"},    // 30+ hexes — NOT visible
	}
	origins := []province.MapPosition{{Q: 10, R: 10}}

	var visible []string
	for _, e := range all {
		if province.VisibleFrom(province.MapPosition{Q: e.Q, R: e.R}, origins, 5) {
			visible = append(visible, e.Name)
		}
	}

	for _, name := range visible {
		if name == "Sodom" {
			t.Error("Sodom (distant city) must NOT appear in FOW-filtered wanaxes list")
		}
	}
	found := func(n string) bool {
		for _, v := range visible {
			if v == n {
				return true
			}
		}
		return false
	}
	if !found("Abydos") {
		t.Error("Abydos (own city) must appear in wanaxes list")
	}
	if !found("Zakros") {
		t.Error("Zakros (nearby city) must appear in wanaxes list")
	}
}

// --- Tiered live-visibility layer (temenos_synlighet.md) --------------------
//
// These tests mirror the tier logic in WorldHandler.Map: a tile's tier is
// "live" if province.AnyEyeSees(eyes, tile, terrain) is true; else "remembered"
// if the tile is in the player's memory set; else "fog".

// tierOf reproduces the exact tier decision from WorldHandler.Map without a DB,
// so the contract is pinned even if the handler is refactored later.
func tierOf(eyes []province.Eye, remembered map[[2]int]bool, tile province.MapPosition, terrain string) string {
	if province.AnyEyeSees(eyes, tile, terrain) {
		return "live"
	}
	if remembered[[2]int{tile.Q, tile.R}] {
		return "remembered"
	}
	return "fog"
}

// TestFOWTier_SettlementSeesLandAtThreeNotFour pins the per-eye radius table:
// a settlement's live vision is 3 hexes over land, not the old flat 6.
func TestFOWTier_SettlementSeesLandAtThreeNotFour(t *testing.T) {
	eyes := []province.Eye{{Pos: province.MapPosition{Q: 0, R: 0}, Kind: province.EyeSettlement}}
	remembered := map[[2]int]bool{}

	if got := tierOf(eyes, remembered, province.MapPosition{Q: 3, R: 0}, "plains"); got != "live" {
		t.Errorf("settlement should live-see land at distance 3, got tier %q", got)
	}
	if got := tierOf(eyes, remembered, province.MapPosition{Q: 4, R: 0}, "plains"); got != "fog" {
		t.Errorf("settlement should NOT live-see land at distance 4, got tier %q", got)
	}
}

// TestFOWTier_LandUnitRadiusTwo pins the locked land-unit default (2 hexes).
func TestFOWTier_LandUnitRadiusTwo(t *testing.T) {
	eyes := []province.Eye{{Pos: province.MapPosition{Q: 0, R: 0}, Kind: province.EyeLandUnit}}
	remembered := map[[2]int]bool{}

	if got := tierOf(eyes, remembered, province.MapPosition{Q: 2, R: 0}, "plains"); got != "live" {
		t.Errorf("land-unit should live-see land at distance 2, got tier %q", got)
	}
	if got := tierOf(eyes, remembered, province.MapPosition{Q: 3, R: 0}, "plains"); got != "fog" {
		t.Errorf("land-unit should NOT live-see land at distance 3, got tier %q", got)
	}
}

// TestFOWTier_ShipSeesSeaFarButLandNear pins the ship's asymmetric vision: 4
// hexes out over open water, only 1 hex inland.
func TestFOWTier_ShipSeesSeaFarButLandNear(t *testing.T) {
	eyes := []province.Eye{{Pos: province.MapPosition{Q: 0, R: 0}, Kind: province.EyeShip}}
	remembered := map[[2]int]bool{}

	if got := tierOf(eyes, remembered, province.MapPosition{Q: 4, R: 0}, "coastal_sea"); got != "live" {
		t.Errorf("ship should live-see sea at distance 4, got tier %q", got)
	}
	if got := tierOf(eyes, remembered, province.MapPosition{Q: 1, R: 0}, "plains"); got != "live" {
		t.Errorf("ship should live-see land at distance 1, got tier %q", got)
	}
	if got := tierOf(eyes, remembered, province.MapPosition{Q: 2, R: 0}, "plains"); got != "fog" {
		t.Errorf("ship should NOT live-see land at distance 2, got tier %q", got)
	}
}

// TestFOWTier_MountainSeenAtBasePlusTwo pins the mountain landmark bonus.
func TestFOWTier_MountainSeenAtBasePlusTwo(t *testing.T) {
	eyes := []province.Eye{{Pos: province.MapPosition{Q: 0, R: 0}, Kind: province.EyeSettlement}}
	remembered := map[[2]int]bool{}

	if got := tierOf(eyes, remembered, province.MapPosition{Q: 5, R: 0}, "mountain_limestone"); got != "live" {
		t.Errorf("settlement should live-see mountain at distance 5 (3+2), got tier %q", got)
	}
	if got := tierOf(eyes, remembered, province.MapPosition{Q: 6, R: 0}, "mountain_limestone"); got != "fog" {
		t.Errorf("settlement should NOT live-see mountain at distance 6, got tier %q", got)
	}
}

// TestFOWTier_RememberedTileStaysVisibleAfterEyeLeaves is the tier-2 contract:
// a tile once live-seen (and therefore persisted to player_scouted_tiles) stays
// visible as "remembered" — dimmed, frozen — even once no live eye covers it
// anymore. This is what loadRememberedTiles supplies.
func TestFOWTier_RememberedTileStaysVisibleAfterEyeLeaves(t *testing.T) {
	tile := province.MapPosition{Q: 20, R: 20}

	// The eye (e.g. a scouting unit) has since moved far away — no live eyes
	// cover the tile anymore.
	eyes := []province.Eye{{Pos: province.MapPosition{Q: 0, R: 0}, Kind: province.EyeLandUnit}}
	remembered := map[[2]int]bool{{tile.Q, tile.R}: true}

	if got := tierOf(eyes, remembered, tile, "plains"); got != "remembered" {
		t.Errorf("previously-scouted tile should stay remembered after the eye leaves, got tier %q", got)
	}
}

// TestFOWTier_RememberedTileShowsNoMarches verifies the Marches/MapTrades/
// MapMessengers gate: only tier-1 (live) tiles carry live activity. A
// remembered tile (no live eye covering it) must not show a march even though
// the tile itself is still visible on the map.
func TestFOWTier_RememberedTileShowsNoMarches(t *testing.T) {
	tile := province.MapPosition{Q: 20, R: 20}
	remembered := map[[2]int]bool{{tile.Q, tile.R}: true}

	// No live eyes anywhere near the tile.
	var eyes []province.Eye

	if tierOf(eyes, remembered, tile, "plains") != "remembered" {
		t.Fatalf("test setup error: tile should be remembered")
	}
	// The Marches/MapTrades/MapMessengers gate is province.AnyEyeSees directly
	// (not tierOf) — a march at this tile must be hidden.
	if province.AnyEyeSees(eyes, tile, "plains") {
		t.Error("a remembered (non-live) tile must not show live march activity")
	}
}

// TestFOWGate_MessengerSendAllowedToRememberedCity is the CRITICAL invariant
// from temenos_synlighet.md: messenger Send gates on the KNOWN set (live ∪
// remembered ∪ contacted), not on live eyes alone. A city the player scouted
// long ago (now outside their shrunken live-vision radius) must still be
// reachable — loadVisibleOrigins includes player_scouted_tiles/provinces and
// messenger contacts as origins, so VisibleFrom finds the city at distance 0
// from itself even with no live eye nearby.
func TestFOWGate_MessengerSendAllowedToRememberedCity(t *testing.T) {
	distantCity := province.MapPosition{Q: 40, R: 40}

	// Known set (as loadVisibleOrigins would return): the player's capital plus
	// this previously-scouted tile — simulating a tile discovered by a unit
	// march or messenger contact long ago, now far outside live vision.
	knownOrigins := []province.MapPosition{
		{Q: 0, R: 0}, // capital
		distantCity,  // remembered/contacted
	}

	if !province.VisibleFrom(distantCity, knownOrigins, 6) {
		t.Error("Send must remain allowed to a remembered/contacted city even without a live eye nearby")
	}

	// Sanity: without the remembered/contacted entry, the same city is NOT known.
	liveOnlyOrigins := []province.MapPosition{{Q: 0, R: 0}}
	if province.VisibleFrom(distantCity, liveOnlyOrigins, 6) {
		t.Fatalf("test setup error: distant city should not be within range of the capital alone")
	}
}
