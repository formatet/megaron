package handlers

import (
	"os"
	"strings"
	"testing"
)

// braceBlock returns the substring of the {…} block whose opening brace is the
// first '{' at or after `from`, matched to its balancing close brace. Used by the
// rite-odds guards to scan a struct/composite-literal block WITHOUT depending on
// its indentation depth — the old guards anchored on a literal "\n\t\t}\n" close
// pattern and silently broke (finding no block, so guarding nothing) as soon as
// the surrounding code changed nesting. Braces inside string/rune literals would
// fool this, but the blocks it scans (a struct decl, a map[string]any literal of
// string→identifier pairs) contain none, so plain counting is safe and cheap.
func braceBlock(text string, from int) (string, bool) {
	open := strings.IndexByte(text[from:], '{')
	if open == -1 {
		return "", false
	}
	open += from
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[open : i+1], true
			}
		}
	}
	return "", false
}

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
	block, ok := braceBlock(text, start)
	if !ok {
		t.Fatal("could not brace-match the prayerRow struct — update this guard if the block shape changed")
	}

	for _, forbidden := range []string{"success_chance", "Chance", "\"chance\"", "odds"} {
		if strings.Contains(block, forbidden) {
			t.Errorf("prayerRow struct contains forbidden token %q — never expose a computed success odds "+
				"(Timothy 2026-07-11 hard invariant, Plan A)", forbidden)
		}
	}
}

// TestRiteResponseNeverExposesChance is the same guard applied to the Rite POST
// handler's response — the one place riteSuccessChance is computed, and the one
// place it must never leak past the roll itself. It anchors on the Rite func
// (settlement.go has several `resp := map[string]any{` literals — an earlier
// version of this guard grabbed the FIRST, an unrelated handler's, and so guarded
// nothing) and checks both the response literal and any later `resp[...]=`
// assignments inside that function.
func TestRiteResponseNeverExposesChance(t *testing.T) {
	src, err := os.ReadFile("settlement.go")
	if err != nil {
		t.Fatalf("could not read settlement.go: %v", err)
	}
	text := string(src)

	fn := strings.Index(text, "func (h *SettlementHandler) Rite(")
	if fn == -1 {
		t.Fatal("Rite handler not found in settlement.go — update this guard if it was renamed or moved")
	}
	// Bound the scan to the Rite function body (up to the next top-level func) so
	// the local `chance`/`successChance` roll variables — which legitimately exist
	// and must NOT trip this — are only flagged if they reach the response.
	body := text[fn:]
	if next := strings.Index(body[1:], "\nfunc "); next != -1 {
		body = body[:next+1]
	}

	respStart := strings.Index(body, "resp := map[string]any{")
	if respStart == -1 {
		t.Fatal("Rite response map literal not found in Rite handler — update this guard if it was moved")
	}
	block, ok := braceBlock(body, respStart)
	if !ok {
		t.Fatal("could not brace-match the Rite response map — update this guard if the block shape changed")
	}
	if strings.Contains(block, "\"chance\"") {
		t.Error("Rite response map literal contains \"chance\" — never expose a computed success odds " +
			"(Timothy 2026-07-11 hard invariant, Plan A)")
	}
	// Also catch a chance smuggled in after the literal via resp["chance"] = …
	for _, line := range strings.Split(body[respStart:], "\n") {
		if strings.Contains(line, "resp[") &&
			(strings.Contains(line, "\"chance\"") || strings.Contains(line, "\"odds\"")) {
			t.Errorf("Rite handler assigns a forbidden odds key to the response: %s", strings.TrimSpace(line))
		}
	}
}
