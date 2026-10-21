package main

import (
	"strings"
	"testing"
	"time"
)

func TestArrivalClockUsesDatesRegardlessOfCadence(t *testing.T) {
	for _, c := range []*Client{{tickSecondsFetched: true}, {tickSeconds: 21600, tickSecondsFetched: true}} {
		for _, delta := range []time.Duration{-time.Hour, 0, 2 * time.Hour, 13 * time.Hour, 72 * time.Hour} {
			at := time.Now().Add(delta).Truncate(time.Second)
			got := arrivalETA(c, at.Format(time.RFC3339))
			if got != wallClock(at) || strings.Contains(got, "days") {
				t.Fatalf("got %q, want %q", got, wallClock(at))
			}
		}
	}
	for _, raw := range []string{"not-a-timestamp", ""} {
		if got := arrivalETA(nil, raw); got != "arrival time unavailable" {
			t.Fatal(got)
		}
	}
}

func TestDayFormattingPreservesDisplayedNumberAndPlural(t *testing.T) {
	for _, tc := range []struct {
		value           float64
		precision, want string
	}{
		{1, "%.0f", "1 day"}, {3, "%.0f", "3 days"}, {1.04, "%.1f", "1.0 day"}, {2.5, "%.1f", "2.5 days"},
	} {
		if got := formatDays(tc.value, tc.precision); got != tc.want {
			t.Fatalf("%v: %q, want %q", tc.value, got, tc.want)
		}
	}
	if got := formatDays(1, "%d", "uncontested"); got != "1 uncontested day" {
		t.Fatal(got)
	}
}
