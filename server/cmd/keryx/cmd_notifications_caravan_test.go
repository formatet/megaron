package main

import (
	"strings"
	"testing"
)

// A seized caravan's notice names its cargo — the same text the web gives.
func TestPrintCaravanSeizureLine_NamesCargo(t *testing.T) {
	body := []byte(`{"transport_id":"x","q":7,"r":0,"goods":[{"good_key":"tin","quantity":30}]}`)
	raided := capturePrint(t, func() { printCaravanSeizureLine(notificationItem{Kind: "CaravanRaided", Body: body}) })
	if !strings.Contains(raided, "Your caravan carrying 30 tin was raided at (7, 0)") {
		t.Errorf("raided line = %q", raided)
	}
	seized := capturePrint(t, func() { printCaravanSeizureLine(notificationItem{Kind: "CaravanSeized", Body: body}) })
	if !strings.Contains(seized, "You seized an enemy caravan carrying 30 tin at (7, 0)") {
		t.Errorf("seized line = %q", seized)
	}
	old := capturePrint(t, func() {
		printCaravanSeizureLine(notificationItem{Kind: "CaravanRaided", Body: []byte(`{"q":1,"r":2}`)})
	})
	if !strings.Contains(old, "Your caravan was raided at (1, 2)") {
		t.Errorf("pre-manifest line = %q", old)
	}
}
