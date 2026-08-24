package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Rad L, megaron_plan_cli_sanning.md: `wants` only ever covers settlements
// the player has actually contacted (market_snapshots, gated by messenger/
// caravan reach) — an empty response is often TRUE, not broken. The old
// single message ("send a messenger... to observe markets") told a Wanax
// who had already contacted several cities to do something he'd already
// done. These tests lock the two fixed behaviours: the empty case no longer
// asserts zero prior contact as fact, and a non-empty response states how
// many settlements it covers.

// runWantsCmd points cfg at ts and captures wantsCmd()'s stdout.
func runWantsCmd(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	prevCfg, prevJSON := cfg, jsonMode
	cfg = &Config{Server: ts.URL, WorldID: "world-1", Token: "t"}
	jsonMode = false
	t.Cleanup(func() { cfg, jsonMode = prevCfg, prevJSON })

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := wantsCmd().RunE(nil, nil)
	w.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatalf("RunE: %v", runErr)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestWantsCmd_EmptyResponseNamesBothCauses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"wants":[],"surplus":[]}`))
	}))
	defer ts.Close()

	out := runWantsCmd(t, ts)

	// Must NOT claim "you haven't sent a messenger" as settled fact —
	// that's the exact lie: a Wanax who has already contacted several
	// cities gets told to do what he already did.
	if strings.Contains(out, "send a messenger or trade offer to observe markets") {
		t.Errorf("output still asserts zero prior contact as fact:\n%s", out)
	}
	// Must name BOTH real causes, not just one.
	wantPhrases := []string{
		"haven't sent a messenger", // cause 1: no contact yet
		"simply show",              // cause 2: contacted, but nothing to report
		"keryx settlements",        // action for cause 1 (see what's known)
		"keryx messenger",          // action: reach a new city (covers both causes)
	}
	for _, phrase := range wantPhrases {
		if !strings.Contains(out, phrase) {
			t.Errorf("output missing %q:\n%s", phrase, out)
		}
	}
}

func TestWantsCmd_NonEmptyResponseCountsContactedSettlements(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Lato appears in BOTH wants and surplus — must be counted once, not twice.
		_, _ = w.Write([]byte(`{
			"wants": [{"name":"Lato","observed_at":"2026-08-24T00:00:00Z","goods":[{"good":"tin","stock":0,"rate":-1.0}]}],
			"surplus": [
				{"name":"Lato","goods":[{"good":"wine","stock":500,"rate":10.0}]},
				{"name":"Chersonesos","goods":[{"good":"fish","stock":900,"rate":40.0}]}
			]
		}`))
	}))
	defer ts.Close()

	out := runWantsCmd(t, ts)

	if !strings.Contains(out, "Market signal from 2 contacted settlement(s)") {
		t.Errorf("output does not state the distinct settlement count (want 2 — Lato+Chersonesos, Lato counted once):\n%s", out)
	}
	if !strings.Contains(out, "SHORTAGES") || !strings.Contains(out, "SURPLUS") {
		t.Errorf("output missing shortage/surplus sections:\n%s", out)
	}
}
