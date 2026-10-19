package main

import (
	"strings"
	"testing"
)

// Haiku round 6: a galley at sea read "OUT OF FOOD" while it still had a part
// of a day's ration aboard — ProvisionDaysLeft floors that to 0.
func TestFormatSize_ShipWithLessThanADayIsNotOutOfFood(t *testing.T) {
	ship := unitRow{Category: "naval", Status: "marching", Crew: 20, Hull: 5}

	ship.ProvisionsAboard = true
	if got := formatSize(nil, ship); !strings.Contains(got, "food for less than a day LOW") || strings.Contains(got, "OUT OF FOOD") {
		t.Errorf("part of a day aboard: %q", got)
	}
	ship.ProvisionsAboard = false
	if got := formatSize(nil, ship); !strings.Contains(got, "OUT OF FOOD") {
		t.Errorf("nothing aboard: %q, want OUT OF FOOD", got)
	}
}
