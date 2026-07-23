package unit

import "testing"

// TestGalleyCanonical pins galley as the sole canonical units.type value after
// namn-hygien A (mig 084): CategoryOf/CrewFor only recognize "galley" now —
// "ship" falls through to the land/0 defaults (intentional; the recruit/disband
// backward-compat path goes through Canonical(), not CategoryOf/CrewFor
// directly — see TestCanonical_LegacyAliases below).
func TestGalleyCanonical(t *testing.T) {
	if got := CategoryOf(TypeGalley); got != CategoryNaval {
		t.Errorf("CategoryOf(galley) = %q, want %q", got, CategoryNaval)
	}
	if got := CrewFor(TypeGalley); got != 20 {
		t.Errorf("CrewFor(galley) = %d, want 20", got)
	}
	if got := DisplayName("galley"); got != "Galley" {
		t.Errorf(`DisplayName("galley") = %q, want "Galley"`, got)
	}
}

// TestCanonical_LegacyAliases verifies the recruit/disband backward-compat
// normalization: old clients sending "ship"/"trireme"/"chariot" still resolve
// to the canonical units.type value.
func TestCanonical_LegacyAliases(t *testing.T) {
	cases := map[string]string{
		"ship":        "galley",
		"trireme":     "galley",
		"chariot":     "war_chariot",
		"galley":      "galley",
		"war_chariot": "war_chariot",
		"spearman":    "spearman",
	}
	for in, want := range cases {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestOtherNavalUnaffected guards against a regression where the alias change
// accidentally alters the other naval types or a land type.
func TestOtherNavalUnaffected(t *testing.T) {
	if CategoryOf(TypeWarGalley) != CategoryNaval || CrewFor(TypeWarGalley) != 50 {
		t.Errorf("war_galley: cat=%q crew=%d", CategoryOf(TypeWarGalley), CrewFor(TypeWarGalley))
	}
	if CategoryOf(TypeMerchantman) != CategoryNaval || CrewFor(TypeMerchantman) != 10 {
		t.Errorf("merchantman: cat=%q crew=%d", CategoryOf(TypeMerchantman), CrewFor(TypeMerchantman))
	}
	if CategoryOf(TypeSpearman) != CategoryLand || CrewFor(TypeSpearman) != 0 {
		t.Errorf("spearman should be land/crew0, got cat=%q crew=%d", CategoryOf(TypeSpearman), CrewFor(TypeSpearman))
	}
}

// TestDisplayName_ConsistentAcrossKnownTypes pins the decided display-name
// taxonomy (Namn-hygien C / A8, Timothy 2026-07-10): one canonical display
// string per DB unit type, moved here from cmd/keryx's now-retired local
// unitDisplayName so keryx, web/API, and notifications all read the same
// table. "Hoplites"/"Agema"/"Hiereus" are retired; "priest" (dead unit,
// mig 060) has no entry and falls back to its raw key.
func TestDisplayName_ConsistentAcrossKnownTypes(t *testing.T) {
	cases := map[string]string{
		"spearman":       "Spearmen",
		"war_chariot":    "War Chariot",
		"galley":         "Galley",
		"trireme":        "Galley",
		"elite_infantry": "Elite Infantry",
		"war_galley":     "War Galley",
		"merchantman":    "Emporos",
	}
	for dbType, want := range cases {
		if got := DisplayName(dbType); got != want {
			t.Errorf("DisplayName(%q) = %q, want %q", dbType, got, want)
		}
	}
}

// TestDisplayName_UnknownFallsBackToRawKey verifies that an unmapped type
// (e.g. a future new unit, or the retired "priest") degrades to showing its
// raw key rather than an empty string or a retired flavour name.
func TestDisplayName_UnknownFallsBackToRawKey(t *testing.T) {
	for _, dbType := range []string{"some_future_unit", "priest"} {
		if got := DisplayName(dbType); got != dbType {
			t.Errorf("DisplayName(%q) = %q, want fallback to the raw key", dbType, got)
		}
	}
}
