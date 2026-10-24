package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Same expectations as web's format.test.mjs — the two surfaces must say the
// same thing about an expedition.
func TestExpeditionTexts(t *testing.T) {
	tick := 11
	if got, want := expeditionTurnedText(expeditionBody{Name: "Spearmen I", AreaQ: 9, Reason: "half_time", ArriveTick: &tick}, "today 23:12"),
		"Mission complete — Spearmen I is returning home. Expected home around today 23:12."; got != want {
		t.Errorf("turned:\n got %q\nwant %q", got, want)
	}
	got := expeditionReportText(expeditionBody{
		Name: "Spearmen I", AreaQ: 5, TicksOut: 9, Furthest: 7, HexesSeen: 40,
		Finds: []expeditionFind{
			{Kind: "copper", Q: 6, R: -1}, {Kind: "copper", Q: 7, R: 2},
			{Kind: "city", Q: 8, Name: "Mycenae", Owner: "Atreus"},
		},
	})
	want := "Spearmen I is home after 9 game days, 7 hexes out at the furthest — saw 40 hexes around (5, 0): " +
		"copper at (6, -1), (7, 2); cities: Mycenae (Atreus) at (8, 0)"
	if got != want {
		t.Errorf("report:\n got %q\nwant %q", got, want)
	}
	if got := expeditionReportText(expeditionBody{AreaQ: 1, AreaR: 1}); got[len(got)-len("no ore deposits, cedar or foreign cities recorded"):] != "no ore deposits, cedar or foreign cities recorded" {
		t.Errorf("empty report = %q, want it to end in \"no ore deposits, cedar or foreign cities recorded\"", got)
	}
}

func TestExpeditionClockETAUsesWorldAnchor(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"current_tick":250,"tick_seconds":60,"tick_anchor_at":"2026-10-20T23:10:00+02:00"}`)
	}))
	defer server.Close()
	c := newClient(&Config{Server: server.URL, WorldID: "world"})
	at, ok := c.WorldTickETA(252)
	if !ok || at.Format(time.RFC3339) != "2026-10-20T23:12:00+02:00" {
		t.Fatalf("clock ETA=%v ok=%v", at, ok)
	}
	if _, ok := c.WorldTickETA(253); !ok || calls != 1 {
		t.Fatalf("world clock fetched%d times; ok=%v", calls, ok)
	}
	tick := 252
	text := capturePrint(t, func() {
		printExpeditionLine(c, notificationItem{Kind: "ExpeditionTurnedHome", Body: []byte(`{"name":"Ship","arrive_tick":252}`)})
	})
	if !strings.Contains(text, "23:12") || strings.Contains(text, "game day 252") {
		t.Fatal(text)
	}
	if text := expeditionTurnedText(expeditionBody{ArriveTick: &tick}); !strings.Contains(text, "Return time unavailable") {
		t.Fatal(text)
	}
}
