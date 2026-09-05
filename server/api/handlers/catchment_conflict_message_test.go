package handlers

// Delad-catchment-grind (Timothy 2026-07-27/28): catchmentConflictMessage is
// the pure half of the FOW-safe founding-blocked message — the DB-backed
// gate itself is proven in internal/province (SettlementCatchmentOverlap) and
// internal/combat (StartMarch / resolve wiring); this only checks the
// wording contract: the blocker's name appears if and only if `known` is
// true, and the message always states how many more hexes to move.

import (
	"strings"
	"testing"

	"formatet/megaron/server/internal/province"
)

func TestCatchmentConflictMessage_NamesTheBlockerOnlyWhenKnown(t *testing.T) {
	conflict := &province.CatchmentConflict{Name: "Mykene", Q: 5, R: 5, Terrain: "plains"}

	known := catchmentConflictMessage(true, 6, 5, conflict)
	if !strings.Contains(known, "Mykene") {
		t.Errorf("known conflict: expected the message to name the blocker, got %q", known)
	}

	unknown := catchmentConflictMessage(false, 6, 5, conflict)
	if strings.Contains(unknown, "Mykene") {
		t.Errorf("unknown conflict: expected NO name leak, got %q", unknown)
	}
	if !strings.Contains(unknown, "another settlement") {
		t.Errorf("unknown conflict: expected a generic phrase, got %q", unknown)
	}
}

func TestCatchmentConflictMessage_ReportsMinimumMoveDistance(t *testing.T) {
	cases := []struct {
		name       string
		q, r       int
		conflict   province.CatchmentConflict
		wantNeeded string
	}{
		// minSettlementCentreDistance = 4 (§3, own design number post-§2/§2b).
		{"adjacent (distance 1) needs 3 more hexes", 6, 5, province.CatchmentConflict{Q: 5, R: 5}, "3 hex"},
		{"distance 2 needs 2 more hexes", 7, 4, province.CatchmentConflict{Q: 5, R: 5}, "2 hex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := catchmentConflictMessage(false, tc.q, tc.r, &tc.conflict)
			if !strings.Contains(got, tc.wantNeeded) {
				t.Errorf("expected message to contain %q, got %q", tc.wantNeeded, got)
			}
		})
	}
}
