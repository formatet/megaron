package handlers

import (
	"os"
	"strings"
	"testing"
)

// TestAvailablePrayersNeverExposesOdds is a regression guard for the hard design
// invariant (Timothy 2026-07-11, Plan A / A7, megaron_kult_legibilitet_plan.md):
// available_prayers (province.go, the prayerRow struct) must never carry a
// computed success percentage — the gods are not machines; gynnsamhet is read
// via the sibling kharis_mood field, not odds. prayerRow is function-local (no
// reflect target), so this is a static source scan rather than a DB-backed
// handler test — cheap, and it fails loudly if anyone re-adds the field.
func TestAvailablePrayersNeverExposesOdds(t *testing.T) {
	src, err := os.ReadFile("province.go")
	if err != nil {
		t.Fatalf("could not read province.go: %v", err)
	}
	text := string(src)

	start := strings.Index(text, "type prayerRow struct")
	if start == -1 {
		t.Fatal("prayerRow struct not found in province.go — update this guard if it was renamed or moved")
	}
	end := strings.Index(text[start:], "\n\t\t}\n")
	if end == -1 {
		t.Fatal("could not find end of prayerRow struct — update this guard if the block shape changed")
	}
	block := text[start : start+end]

	for _, forbidden := range []string{"success_chance", "Chance", "\"chance\"", "odds"} {
		if strings.Contains(block, forbidden) {
			t.Errorf("prayerRow struct contains forbidden token %q — never expose a computed success odds "+
				"(Timothy 2026-07-11 hard invariant, Plan A)", forbidden)
		}
	}
}

// TestRiteResponseNeverExposesChance is the same guard applied to the Rite POST
// handler's response map (settlement.go) — the one place riteSuccessChance is
// computed, and the one place it must never leak past the roll itself.
func TestRiteResponseNeverExposesChance(t *testing.T) {
	src, err := os.ReadFile("settlement.go")
	if err != nil {
		t.Fatalf("could not read settlement.go: %v", err)
	}
	text := string(src)

	start := strings.Index(text, "resp := map[string]any{")
	if start == -1 {
		t.Fatal("Rite response map literal not found in settlement.go — update this guard if it was moved")
	}
	end := strings.Index(text[start:], "\n\t}\n")
	if end == -1 {
		t.Fatal("could not find end of resp map literal — update this guard if the block shape changed")
	}
	block := text[start : start+end]

	if strings.Contains(block, "\"chance\"") {
		t.Error("Rite response map contains \"chance\" — never expose a computed success odds " +
			"(Timothy 2026-07-11 hard invariant, Plan A)")
	}
}
