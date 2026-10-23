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

func TestFormatSizeFreightingProvisionsParity(t *testing.T) {
	for _, tc := range []struct {
		days   int
		aboard bool
		want   string
	}{{7, true, "food for 7 days"}, {0, true, "food for less than a day LOW"}, {0, false, "OUT OF FOOD"}} {
		u := unitRow{Category: "naval", Type: "merchantman", Status: "freighting", Crew: 10, Hull: 5, ProvisionDays: tc.days, ProvisionsAboard: tc.aboard}
		if got := formatSize(nil, u); !strings.Contains(got, tc.want) {
			t.Errorf("%+v got%q want%q", tc, got, tc.want)
		}
	}
}
