package main

import "testing"

func TestGameDurationUsesActualWorldCadence(t *testing.T) {
	for _, tc := range []struct {
		cadence float64
		want    string
	}{
		{1800, "12 game days · about 6 hours"}, {3600, "12 game days · about 12 hours"}, {6, "12 game days · about 1.2 minutes"}, {0, "12 game days"},
	} {
		c := &Client{tickSecondsFetched: true, tickSeconds: tc.cadence}
		if got := formatDuration(c, 12); got != tc.want {
			t.Fatalf("cadence%g: %s want%s", tc.cadence, got, tc.want)
		}
	}
	if got := formatDuration(nil, 12); got != "12 game days" {
		t.Fatal(got)
	}
}
