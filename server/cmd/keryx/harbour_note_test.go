package main

import (
	"strings"
	"testing"
)

func TestRenderCatchmentForecast_PrintsHarbourNote(t *testing.T) {
	p := &colonizePreview{HarbourNote: "Inland: a city here can never build a harbour or ships — to reach the sea you will need a coastal colony."}
	out, err := captureStdout(t, func() error { renderCatchmentForecast("Settle", p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "can never build a harbour or ships") {
		t.Fatalf("forecast must show the harbour note, got:\n%s", out)
	}
	out, _ = captureStdout(t, func() error { renderCatchmentForecast("Settle", &colonizePreview{}); return nil })
	if strings.Contains(out, "harbour") {
		t.Fatalf("no note from server, none printed:\n%s", out)
	}
}
