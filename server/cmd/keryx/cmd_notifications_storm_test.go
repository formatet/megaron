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
