package main

import (
	"strings"
	"testing"
)

// A V2 sighting says who and which way — never where to. Toward your lands it
// names the city and the tick it would get there if that is its goal.
func TestPrintForeignMarchSightedV2Line(t *testing.T) {
	warn := capturePrint(t, func() {
		printForeignMarchSightedV2Line(notificationItem{Kind: "ForeignMarchSightedV2", Body: []byte(
			`{"owner":"Minos","unit_type":"spearman","size":100,"stance":"aggressive","q":15,"r":0,` +
				`"heading":"south-east","threatens_name":"Mycenae","eta_if_tick":3017}`)})
	})
	want := "Minos' spearman (100, aggressive) seen at (15,0), heading south-east — toward YOUR CITY Mycenae's lands; there by tick 3017 if that is its goal"
	if !strings.Contains(warn, want) {
		t.Errorf("warning line = %q\nwant it to contain %q", warn, want)
	}
	pass := capturePrint(t, func() {
		printForeignMarchSightedV2Line(notificationItem{Kind: "ForeignMarchSightedV2", Body: []byte(
			`{"owner":"Ariadne","unit_type":"galley","size":1,"q":3,"r":4,"heading":"north"}`)})
	})
	if !strings.Contains(pass, "Ariadne's galley (1) seen at (3,4), heading north") || strings.Contains(pass, "toward") {
		t.Errorf("passing line = %q", pass)
	}
}
