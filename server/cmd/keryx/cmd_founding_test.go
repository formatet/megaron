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

func TestFoundingStoreDaysAndWallClock(t *testing.T) {
	for _, days := range []int{1, 3, 1000} {
		got := foundingStoreLine("Food", foundingStore{Amount: 50, TicksLeft: &days}, 86400)
		if !strings.Contains(got, formatDays(float64(days), "%.0f")) || strings.Contains(got, "≈ ") || strings.Contains(got, ":") && strings.Count(got, ":") > 1 {
			t.Fatalf("world duration and clock/date: %q", got)
		}
		if strings.Contains(got, "real time") || strings.Contains(got, "tick") {
			t.Fatalf("mixed time units: %q", got)
		}
	}
}

func TestFoundingTexts(t *testing.T) {
	if err := errNeedYes("keryx founding settle", "the founding"); !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "keryx founding settle") {
		t.Fatalf("refusal must name --yes and the command: %v", err)
	}
	got := foundingStoresLine(1200, 476, 1200, 3716)
	if !strings.Contains(got, "476") || !strings.Contains(got, "3716") {
		t.Fatalf("line must show carried and held silver: %q", got)
	}
	if errNoCityYet("") == nil || errNoCityYet("abc") != nil {
		t.Fatal("errNoCityYet must fire only on an empty province ID")
	}
}
