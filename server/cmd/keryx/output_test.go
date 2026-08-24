package main

import (
	"strings"
	"testing"
	"time"
)

func TestArrivalETA_FutureTimestampBecomesCountdown(t *testing.T) {
	// A server RFC3339 timestamp ~3h out should render as a relative countdown
	// ("in 3h Xm"), never the raw UTC string — that is the whole point of the
	// helper (dispatch receipts used to echo the nanosecond UTC stamp).
	iso := time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
	got := arrivalETA(iso)
	if !strings.HasPrefix(got, "in ") {
		t.Fatalf("arrivalETA(%q) = %q, want an \"in …\" countdown", iso, got)
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
	for _, raw := range []string{"not-a-timestamp", ""} {
		if got := arrivalETA(raw); got != raw {
			t.Errorf("arrivalETA(%q) = %q, want the raw string back on parse failure", raw, got)
		}
	}
}
