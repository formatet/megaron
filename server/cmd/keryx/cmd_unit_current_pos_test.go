package main

import (
	"strings"
	"testing"
)

// Movement 2a, R8 (megaron_plan_rorelse_sparad_vag.md): `unit list` shows a
// marching unit's saved-route position ("now at (q,r)") next to its target,
// when the server sent current_q/current_r. Absent them (pre-153 march, or
// the straight-line fallback), the line is unchanged from before this slice.

func TestLocationStr_Marching_WithCurrentPos_ShowsNowAt(t *testing.T) {
	u := unitRow{
		Category: "land",
		Status:   "marching",
		Q:        intPtr(0),
		R:        intPtr(0),
		TargetQ:  intPtr(2),
		TargetR:  intPtr(0),
		CurrentQ: intPtr(1),
		CurrentR: intPtr(0),
	}
	got := locationStr(nil, u, nil)
	if !strings.Contains(got, "now at (1,0)") {
		t.Errorf("raden visar inte den sparade väg-positionen: %q", got)
	}
}

func TestLocationStr_Marching_WithoutCurrentPos_Unchanged(t *testing.T) {
	u := unitRow{
		Category: "land",
		Status:   "marching",
		Q:        intPtr(0),
		R:        intPtr(0),
		TargetQ:  intPtr(2),
		TargetR:  intPtr(0),
	}
	got := locationStr(nil, u, nil)
	if strings.Contains(got, "now at") {
		t.Errorf("raden ska inte gissa en nuvarande position utan sparad väg: %q", got)
	}
	want := "(0,0)→(2,0)"
	if got != want {
		t.Errorf("locationStr = %q, want %q (unchanged from before this slice)", got, want)
	}
}
