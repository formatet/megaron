package main

import "testing"

// P11 (soak 2026-07-18): loyalty sat at 1-2 with no visible raising mechanic.
// formatLoyaltyLog is the pure function `status` uses to surface the
// already-existing server-side loyalty mechanic (welfare/decay/colony/
// borrowed-army/gift/battle deltas) via the settlement's loyalty-log — this
// reproduces both the "nothing fired yet" and "here's what happened" cases
// without needing a live server.

func TestFormatLoyaltyLog_EmptyShowsLegend(t *testing.T) {
	lines := formatLoyaltyLog(nil)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (empty notice + legend), got %d: %v", len(lines), lines)
	}
	if !contains(lines[0], "Inga lojalitetshändelser") {
		t.Errorf("expected empty-state notice, got %q", lines[0])
	}
	if lines[1] != loyaltyLegend {
		t.Errorf("expected legend as final line, got %q", lines[1])
	}
	if !contains(loyaltyLegend, "Höjs av") || !contains(loyaltyLegend, "Sänks av") {
		t.Fatalf("legend must name both raising and lowering levers, got %q", loyaltyLegend)
	}
}

func TestFormatLoyaltyLog_ShowsRecentEventsThenLegend(t *testing.T) {
	entries := []loyaltyLogEntry{
		{EventType: "well_favoured", LoyaltyDelta: 1, Reason: "well favoured (kharis at or above bless threshold)", CreatedAt: "2026-07-18T10:00:00Z"},
		{EventType: "starving", LoyaltyDelta: -1, Reason: "starving (grain stock below the comfortable-buffer reference)", CreatedAt: "2026-07-17T10:00:00Z"},
	}
	lines := formatLoyaltyLog(entries)

	if lines[0] != "  Senaste lojalitetshändelser:" {
		t.Errorf("expected header line, got %q", lines[0])
	}
	if !contains(lines[1], "+1") || !contains(lines[1], "well_favoured") {
		t.Errorf("expected raising event with +1 delta, got %q", lines[1])
	}
	if !contains(lines[2], "-1") || !contains(lines[2], "starving") {
		t.Errorf("expected lowering event with -1 delta, got %q", lines[2])
	}
	if last := lines[len(lines)-1]; last != loyaltyLegend {
		t.Errorf("expected legend as final line even with events present, got %q", last)
	}
}

func TestFormatLoyaltyLog_CapsAtFive(t *testing.T) {
	entries := make([]loyaltyLogEntry, 8)
	for i := range entries {
		entries[i] = loyaltyLogEntry{EventType: "gift", LoyaltyDelta: 1, Reason: "wanax_gift", CreatedAt: "2026-07-18T10:00:00Z"}
	}
	lines := formatLoyaltyLog(entries)
	// header + 5 entries + legend = 7 lines.
	if len(lines) != 7 {
		t.Fatalf("expected header+5+legend = 7 lines, got %d", len(lines))
	}
}
