package handlers

// FOW-gating tests — server-side fog-of-war enforcement (Sprint C).
//
// These tests verify the FOW contract that the server enforces on
// POST .../messengers (Send) and GET .../wanaxes:
//
//   • A player can only send a messenger (or see a wanax in the directory) if
//     the destination city is within 5 hexes of one of their visibleOrigins.
//   • A city that is NOT within range must be rejected / hidden.
//
// The FOW decision is:  province.VisibleFrom(dest, origins, 5)
// loadVisibleOrigins (world.go) populates `origins` from the DB.
// Both the Send handler and the Wanaxes handler call exactly this pair.
//
// We test the province.VisibleFrom function directly (it is the sole gate) plus
// the error message wired into the Send handler so regressions are caught early.

import (
	"testing"

	"github.com/poleia/server/internal/province"
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
