package main

import (
	"strings"
	"testing"
)

func TestFormatSizeShowsYardSharing(t *testing.T) {
	forming := unitRow{Category: "naval", Status: "forming", Crew: 10, ShipyardWorkers: 3, ShipyardSharing: 2}
	repairing := unitRow{Category: "naval", Status: "repairing", ShipyardWorkers: 3, ShipyardSharing: 2}
	for _, u := range []unitRow{forming, repairing} {
		if got := formatSize(nil, u); !strings.Contains(got, "3 workers shared by 2 ships") {
			t.Errorf("%s: %q lacks the sharing note", u.Status, got)
		}
		u.ShipyardSharing = 1
		if got := formatSize(nil, u); strings.Contains(got, "shared by") {
			t.Errorf("%s alone: %q", u.Status, got)
		}
	}
}
