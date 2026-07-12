package unit

import "testing"

// TestGalleyShipAlias pins the fix for the "ship"/"galley" split: the canonical
// API/UnitSpecs/CLI value is "ship" while the unit-model constant is "galley".
// Before the fix CategoryOf("ship")→land and CrewFor("ship")→0, which made the
// Recruit handler build a broken forming land-unit (crew 0, never garrison) for
// the standard galley — blocking naval transport / colonisation. Both spellings
// must resolve to the same naval galley until the full rename→galley (D-stream).
func TestGalleyShipAlias(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  Type
	}{
		{"canonical galley", TypeGalley},
		{"legacy ship alias", TypeShip},
		{"raw ship string", Type("ship")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CategoryOf(tc.typ); got != CategoryNaval {
				t.Errorf("CategoryOf(%q) = %q, want %q", tc.typ, got, CategoryNaval)
			}
			if got := CrewFor(tc.typ); got != 20 {
				t.Errorf("CrewFor(%q) = %d, want 20", tc.typ, got)
			}
		})
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
// string per DB unit type, moved here from cmd/poleia's now-retired local
// unitDisplayName so keryx, web/API, and notifications all read the same
// table. "Hoplites"/"Agema"/"Hiereus" are retired; "priest" (dead unit,
// mig 060) has no entry and falls back to its raw key.
func TestDisplayName_ConsistentAcrossKnownTypes(t *testing.T) {
	cases := map[string]string{
		"spearman":       "Spearmen",
		"war_chariot":    "War Chariot",
		"ship":           "Galley",
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
