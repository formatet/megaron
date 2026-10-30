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

func TestKharisDecliningLine_PunishmentCurve(t *testing.T) {
	out, _ := captureStdout(t, func() error {
		printNotificationSummary(nil, notificationItem{Kind: "KharisDeclining", Body: []byte(`{"kharis":28.9,"net_per_tick":-1.2,"wrath_threshold":30,"wrath_probability":0.02,"days_to_wrathful":20,"wrathful_threshold":5,"wrathful_probability":0.04}`)})
		printNotificationSummary(nil, notificationItem{Kind: "KharisDeclining", Body: []byte(`{"kharis":28.9,"net_per_tick":-1.2,"days_to_15":12,"wrath_threshold":15}`)})
		return nil
	})
	for _, want := range []string{"Below 30 the gods may punish you, with or without a temple: about 2% each game day, 4% below 5. At this rate it falls below 5 in 20", "At this rate it reaches 15 in 12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("keryx = %q, want %q", out, want)
		}
	}
}
