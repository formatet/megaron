package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture is a payload the server really persisted (combat sea_storm_test
// checks its keys against the DB row).
func TestPrintSeaStormLine_FounderedNamesCargo(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "static", "js", "megaron", "ui", "testdata", "ship_foundered.json"))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := captureStdout(t, func() error {
		printSeaStormLine(notificationItem{Kind: "ShipFoundered", Body: raw})
		return nil
	})
	want := "Your Galley foundered in a storm at (6,0). Lost with her: 40 silver."
	if !strings.Contains(out, want) {
		t.Fatalf("keryx line = %q, want it to contain %q", out, want)
	}
}

func TestPrintSeaStormLine_GodsHand(t *testing.T) {
	out, _ := captureStdout(t, func() error {
		printSeaStormLine(notificationItem{Kind: "ShipStormSpared", Body: []byte(`{"name":"Ariadne","q":6,"r":0,"hull":5,"hull_max":5,"divine":"spared"}`)})
		printSeaStormLine(notificationItem{Kind: "ShipStormDamaged", Body: []byte(`{"name":"Ariadne","q":6,"r":0,"hull":2,"hull_max":5,"divine":"wrath"}`)})
		return nil
	})
	for _, want := range []string{"Poseidon held back the waves — your Ariadne sails on unharmed, hull 5/5", "hull 2/5. The gods' wrath: the storm struck harder."} {
		if !strings.Contains(out, want) {
			t.Fatalf("keryx = %q, want %q", out, want)
		}
	}
}

func TestPrintBattleReportLine_GodsHand(t *testing.T) {
	out, _ := captureStdout(t, func() error {
		printBattleReportLine(notificationItem{Kind: "BattleWon", Body: []byte(`{"role":"attacker","outcome":"attacker_wins","opponent_name":"Nestor","place":"Pylos","divine":"favor"}`)})
		printBattleReportLine(notificationItem{Kind: "BattleLost", Body: []byte(`{"role":"defender","outcome":"attacker_wins","opponent_name":"Nestor","place":"Pylos","divine":"wrath"}`)})
		return nil
	})
	for _, want := range []string{"The field was taken. The gods stood with your men.", "The field fell. The gods' wrath weighed on your men."} {
		if !strings.Contains(out, want) {
			t.Fatalf("keryx = %q, want %q", out, want)
		}
	}
}
