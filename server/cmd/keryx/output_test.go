package main

import (
	"strings"
	"testing"
	"time"
)

// TestArrivalETA_KnownTickSeconds_RendersGameDaysFirst is rad K's core
// acceptance case (megaron_plan_cli_sanning.md): with a known tick cadence,
// an arrival must show game-days FIRST, wall clock only as parenthetical
// support — never a raw UTC/nanosecond stamp, and never a bare "days" that
// could be misread as real days. Before this slice, arrivalETA had no
// concept of a tick cadence at all and always rendered a wall-clock
// countdown (see TestArrivalETA_UnknownTickSeconds_DegradesToWallClockCountdown,
// which pins that old contract as the explicit degrade path).
func TestArrivalETA_KnownTickSeconds_RendersGameDaysFirst(t *testing.T) {
	c := &Client{tickSeconds: 21600, tickSecondsFetched: true} // 6h/tick
	iso := time.Now().Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339)
	got := arrivalETA(c, iso)
	if !strings.HasPrefix(got, "in ") || !strings.Contains(got, "game-days") {
		t.Fatalf("arrivalETA(%q) = %q, want a game-day-first \"in N game-days (...)\" string", iso, got)
	}
	if !strings.Contains(got, "in 12 game-days") {
		t.Errorf("arrivalETA(%q) = %q, want 12 game-days (3 days / 6h-per-tick = 12 ticks)", iso, got)
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

// TestGameETA_RoundsDaysUp: a Wanax plans in whole game-days, and "0
// game-days left" would misread as "already here" — a few hours left must
// still show as 1 game-day away, not 0.
func TestGameETA_RoundsDaysUp(t *testing.T) {
	c := &Client{tickSeconds: 21600, tickSecondsFetched: true} // 6h/tick
	got := gameETA(c, time.Now().Add(2*time.Hour))
	// Singular at exactly one: "in 1 game-days" is the plural bug this test
	// would otherwise have pinned into place.
	if !strings.Contains(got, "in 1 game-day (") {
		t.Errorf("gameETA(2h out, 6h/tick) = %q, want \"in 1 game-day (…)\" (rounded up, singular)", got)
	}
}

// TestGameETA_PluralAboveOne guards the other side of the singular branch:
// two or more game-days must NOT lose the plural s.
func TestGameETA_PluralAboveOne(t *testing.T) {
	c := &Client{tickSeconds: 21600, tickSecondsFetched: true} // 6h/tick
	got := gameETA(c, time.Now().Add(13*time.Hour))
	if !strings.Contains(got, "in 3 game-days (") {
		t.Errorf("gameETA(13h out, 6h/tick) = %q, want \"in 3 game-days (…)\"", got)
	}
}

// TestGameETA_ArrivedOrPast_SaysAnyMoment: math.Ceil already guarantees
// every remaining-time>0 case renders as at least 1 game-day, so the days<=0
// branch only ever fires for an arrival that is NOW or already PAST. "in 0
// game-days" or "less than 1 game-day" would both read as future — wrong for
// something that has already landed. countdown's own word for this state
// ("any moment") already exists in this file and means exactly this.
func TestGameETA_ArrivedOrPast_SaysAnyMoment(t *testing.T) {
	c := &Client{tickSeconds: 21600, tickSecondsFetched: true} // 6h/tick
	for _, got := range []string{
		gameETA(c, time.Now()),
		gameETA(c, time.Now().Add(-time.Hour)),
	} {
		if !strings.Contains(got, "any moment") {
			t.Errorf("gameETA(now-or-past) = %q, want \"any moment\", not a days figure", got)
		}
		if strings.Contains(got, "game-days") {
			t.Errorf("gameETA(now-or-past) = %q, must not claim a future game-days count", got)
		}
	}
}
