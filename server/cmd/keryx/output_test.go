package main

import (
	"strings"
	"testing"
	"time"
)

// TestArrivalETA_KnownTickSeconds_RendersGameDaysFirst is rad K's core
// acceptance case (megaron_plan_cli_sanning.md): with a known tick cadence,
// an arrival must show speldygn FIRST, wall clock only as parenthetical
// support — never a raw UTC/nanosecond stamp. Before this slice, arrivalETA
// had no concept of a tick cadence at all and always rendered a wall-clock
// countdown (see TestArrivalETA_UnknownTickSeconds_DegradesToWallClockCountdown,
// which pins that old contract as the explicit degrade path).
func TestArrivalETA_KnownTickSeconds_RendersGameDaysFirst(t *testing.T) {
	c := &Client{tickSeconds: 21600, tickSecondsFetched: true} // 6h/tick
	iso := time.Now().Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339)
	got := arrivalETA(c, iso)
	if !strings.HasPrefix(got, "om ") || !strings.Contains(got, "speldygn") {
		t.Fatalf("arrivalETA(%q) = %q, want a game-day-first \"om N speldygn (...)\" string", iso, got)
	}
	if !strings.Contains(got, "om 12 speldygn") {
		t.Errorf("arrivalETA(%q) = %q, want 12 speldygn (3 days / 6h-per-tick = 12 ticks)", iso, got)
	}
	if strings.Contains(got, "T") || strings.Contains(got, "Z") {
		t.Fatalf("arrivalETA(%q) = %q leaked the raw RFC3339 string", iso, got)
	}
	if !strings.Contains(got, "(") {
		t.Errorf("arrivalETA(%q) = %q, want a parenthetical wall-clock support", iso, got)
	}
}

// TestArrivalETA_UnknownTickSeconds_DegradesToWallClockCountdown pins the
// degrade path: when the server's tick cadence can't be established, fall
// back to the old wall-clock-relative countdown rather than showing a
// nonsense or zero game-day figure.
func TestArrivalETA_UnknownTickSeconds_DegradesToWallClockCountdown(t *testing.T) {
	c := &Client{tickSecondsFetched: true} // fetched, but no usable cadence
	iso := time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
	got := arrivalETA(c, iso)
	if !strings.HasPrefix(got, "in ") {
		t.Fatalf("arrivalETA(%q) = %q, want the old \"in …\" countdown when tick cadence is unknown", iso, got)
	}
	if !strings.Contains(got, "h") {
		t.Fatalf("arrivalETA(%q) = %q, want an hours component for a 3h ETA", iso, got)
	}
	if strings.Contains(got, "T") || strings.Contains(got, "Z") {
		t.Fatalf("arrivalETA(%q) = %q leaked the raw RFC3339 string", iso, got)
	}
}

func TestArrivalETA_UnparseableFallsBackToRaw(t *testing.T) {
	// If the server ever changes the timestamp format, degrade to the old
	// behaviour (show the raw value) rather than dropping the ETA entirely.
	c := &Client{tickSeconds: 21600, tickSecondsFetched: true}
	for _, raw := range []string{"not-a-timestamp", ""} {
		if got := arrivalETA(c, raw); got != raw {
			t.Errorf("arrivalETA(%q) = %q, want the raw string back on parse failure", raw, got)
		}
	}
}

// TestGameETA_RoundsDaysUp: a Wanax plans in whole speldygn, and "0 speldygn
// left" would misread as "already here" — a few hours left must still show
// as 1 speldygn away, not 0.
func TestGameETA_RoundsDaysUp(t *testing.T) {
	c := &Client{tickSeconds: 21600, tickSecondsFetched: true} // 6h/tick
	got := gameETA(c, time.Now().Add(2*time.Hour))
	if !strings.Contains(got, "om 1 speldygn") {
		t.Errorf("gameETA(2h out, 6h/tick) = %q, want \"om 1 speldygn\" (rounded up)", got)
	}
}
