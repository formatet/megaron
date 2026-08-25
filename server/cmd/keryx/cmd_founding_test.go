package main

import (
	"strings"
	"testing"
)

// TestFoundingSettleHelpMatchesJSONBehavior guards against the help text
// overclaiming what --json mode does. The Long description once said the
// forecast is "ALWAYS shown before the confirmation" while the RunE only
// fetches and renders it in the non-JSON branch — an agent reading --help
// had no way to know the forecast is skipped under --json. See
// megaron_todo.md NU/BESLUT: "founding settle --json hoppar över
// grundningsprognosen — hjälptexten säger motsatsen."
func TestFoundingSettleHelpMatchesJSONBehavior(t *testing.T) {
	cmd := foundingSettleCmd()

	if strings.Contains(cmd.Long, "ALWAYS shown") {
		t.Error("Long claims the forecast is ALWAYS shown, but RunE skips it in --json mode (machine caller) — text overclaims")
	}
	if !strings.Contains(cmd.Long, "json") {
		t.Error("Long does not mention --json behavior at all — an agent has no way to learn the forecast is skipped")
	}

	yesFlag := cmd.Flags().Lookup("yes")
	if yesFlag == nil {
		t.Fatal("--yes flag missing")
	}
	if strings.Contains(yesFlag.Usage, "the forecast is still printed") && !strings.Contains(yesFlag.Usage, "json") {
		t.Error("--yes usage claims the forecast is still printed without qualifying that this excludes --json mode")
	}
}
