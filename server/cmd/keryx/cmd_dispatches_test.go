package main

import "testing"

func TestDescribeMutedKinds(t *testing.T) {
	if got, want := describeMutedKinds(nil), "Every notification kind arrives as a dispatch — none muted."; got != want {
		t.Errorf("describeMutedKinds(nil) = %q, want %q", got, want)
	}
	if got, want := describeMutedKinds([]string{"ScoutReport", "ColonyFounded"}),
		"Muted as dispatches (still kept in keryx notifications): ScoutReport, ColonyFounded"; got != want {
		t.Errorf("describeMutedKinds = %q, want %q", got, want)
	}
}
