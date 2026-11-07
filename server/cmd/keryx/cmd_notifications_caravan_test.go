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

// Cargo lists are written alphabetically by good, whatever the payload order.
func TestCargoListsAlphabetical(t *testing.T) {
	goods := `[{"good_key":"wine","quantity":5},{"good_key":"grain","quantity":20},{"good_key":"oil","quantity":7}]`
	raided := capturePrint(t, func() {
		printCaravanSeizureLine(notificationItem{Kind: "CaravanRaided", Body: []byte(`{"q":1,"r":2,"goods":` + goods + `}`)})
	})
	if !strings.Contains(raided, "carrying 20 grain, 7 oil, 5 wine was") {
		t.Errorf("caravan line = %q", raided)
	}
	storm := capturePrint(t, func() {
		printSeaStormLine(notificationItem{Kind: "ShipFoundered", Body: []byte(`{"name":"A","q":1,"r":2,"cargo":` + goods + `}`)})
	})
	if !strings.Contains(storm, "Lost with her: 20 grain, 7 oil, 5 wine.") {
		t.Errorf("storm line = %q", storm)
	}
	transfer := capturePrint(t, func() {
		printNotificationSummary(nil, notificationItem{Kind: "TransferDelivered", Body: []byte(`{"dest_name":"X","goods":` + goods + `}`)})
	})
	if !strings.Contains(transfer, "20 grain, 7 oil, 5 wine") {
		t.Errorf("transfer line = %q", transfer)
	}
}
