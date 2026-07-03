package main

import "testing"

// TestUnitDisplayName_ConsistentAcrossKnownTypes pins Fas 2g: one canonical
// display string per DB unit type, shared by status/unit-list, so recruiting
// via an alias (--unit hoplites) and later listing the unit never disagree
// on what to call it (unit list used to print the raw DB key "spearman").
func TestUnitDisplayName_ConsistentAcrossKnownTypes(t *testing.T) {
	cases := map[string]string{
		"spearman":       "Hoplites",
		"war_chariot":    "War Chariot",
		"priest":         "Hiereus",
		"ship":           "Galley",
		"galley":         "Galley",
		"elite_infantry": "Agema",
		"war_galley":     "War Galley",
		"merchantman":    "Merchantman",
	}
	for dbType, want := range cases {
		if got := unitDisplayName(dbType); got != want {
			t.Errorf("unitDisplayName(%q) = %q, want %q", dbType, got, want)
		}
	}
}

// TestUnitDisplayName_UnknownFallsBackToRawKey verifies that an unmapped type
// (e.g. a future new unit) degrades to showing its raw key rather than an
// empty string.
func TestUnitDisplayName_UnknownFallsBackToRawKey(t *testing.T) {
	if got := unitDisplayName("some_future_unit"); got != "some_future_unit" {
		t.Errorf("unitDisplayName(unknown) = %q, want fallback to the raw key", got)
	}
}
