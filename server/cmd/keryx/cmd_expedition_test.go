package main

import "testing"

// Same expectations as web's format.test.mjs — the two surfaces must say the
// same thing about an expedition.
func TestExpeditionTexts(t *testing.T) {
	tick := 11
	if got, want := expeditionTurnedText(expeditionBody{Name: "Spearmen I", AreaQ: 9, Reason: "half_time", ArriveTick: &tick}),
		"Spearmen I turns home from the land around (9, 0): half its time is spent — home by day 11"; got != want {
		t.Errorf("turned:\n got %q\nwant %q", got, want)
	}
	got := expeditionReportText(expeditionBody{
		Name: "Spearmen I", AreaQ: 5, TicksOut: 9, Furthest: 7, HexesSeen: 40,
		Finds: []expeditionFind{
			{Kind: "copper", Q: 6, R: -1}, {Kind: "copper", Q: 7, R: 2},
			{Kind: "city", Q: 8, Name: "Mycenae", Owner: "Atreus"},
		},
	})
	want := "Spearmen I is home after 9 days, 7 hexes out at the furthest — saw 40 hexes around (5, 0): " +
		"copper at (6, -1), (7, 2); cities: Mycenae (Atreus) at (8, 0)"
	if got != want {
		t.Errorf("report:\n got %q\nwant %q", got, want)
	}
	if got := expeditionReportText(expeditionBody{AreaQ: 1, AreaR: 1}); got[len(got)-len("nothing of value"):] != "nothing of value" {
		t.Errorf("empty report = %q, want it to end in \"nothing of value\"", got)
	}
}
