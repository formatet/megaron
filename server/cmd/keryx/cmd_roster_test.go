package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// runRosterCmd points cfg at a stub serving /settlements/placement-roster and
// captures rosterCmd()'s stdout — same harness shape as runCityCmd.
func runRosterCmd(t *testing.T, body string, json bool) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	prevCfg, prevJSON := cfg, jsonMode
	cfg = &Config{Server: ts.URL, WorldID: "world-1", ProvinceID: "prov-1", Token: "t"}
	jsonMode = json
	t.Cleanup(func() { cfg, jsonMode = prevCfg, prevJSON })

	old := os.Stdout
	rp, wp, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wp
	runErr := rosterCmd().RunE(nil, nil)
	wp.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatalf("RunE: %v", runErr)
	}
	var buf bytes.Buffer
	buf.ReadFrom(rp)
	return buf.String()
}

const rosterBody = `[
	{"id":"s1","province_id":"p1","name":"Knossos","is_capital":true,"total_gubbar":8,"placed":5,"idle":3,
	 "assignments":[
		{"good_key":"stone","target_kind":"building","building_type":"stonequarry","count":1},
		{"good_key":"grain","target_kind":"hex","hex_ordinal":1,"hex_q":-2,"hex_r":0,"count":3},
		{"good_key":"fish","target_kind":"hex","hex_ordinal":2,"hex_q":-2,"hex_r":1,"count":1}
	 ]},
	{"id":"s2","province_id":"p2","name":"Kommos","is_capital":false,"total_gubbar":8,"placed":0,"idle":8,
	 "assignments":[]}
]`

// TestRosterCmd_RendersRealmWideWithIdle is the surface the report asked for:
// one command, every settlement, idle pool per settlement, and where each
// gubbe works — without opening each city.
func TestRosterCmd_RendersRealmWideWithIdle(t *testing.T) {
	out := runRosterCmd(t, rosterBody, false)

	// Realm summary: 5 placed of 16, 11 idle across 2 settlements.
	if !strings.Contains(out, "across 2 settlements") || !strings.Contains(out, "5/16 placed, 11 idle") {
		t.Errorf("missing/incorrect realm summary line:\n%s", out)
	}
	// Capital marked, its idle count named.
	if !strings.Contains(out, "Knossos (capital)") || !strings.Contains(out, "5/8 placed, 3 idle") {
		t.Errorf("Knossos header wrong:\n%s", out)
	}
	// Assignments show count, good and a resolved hex ordinal / building name.
	if !strings.Contains(out, "3× grain") || !strings.Contains(out, "hex #1") {
		t.Errorf("grain-on-hex#1 assignment not rendered:\n%s", out)
	}
	if !strings.Contains(out, "1× stone") || !strings.Contains(out, "stonequarry") {
		t.Errorf("stone-in-quarry assignment not rendered:\n%s", out)
	}
	// A fully-idle settlement says so instead of showing an empty block.
	if !strings.Contains(out, "Kommos") || !strings.Contains(out, "every citizen is idle") {
		t.Errorf("fully-idle settlement not rendered clearly:\n%s", out)
	}
}

// TestRosterCmd_EmptyRealm handles the founder phase (no settlements yet).
func TestRosterCmd_EmptyRealm(t *testing.T) {
	out := runRosterCmd(t, `[]`, false)
	if !strings.Contains(out, "No settlements yet") {
		t.Errorf("empty realm should point the player at founding:\n%s", out)
	}
}

// TestRosterCmd_JSONPassthrough: --json emits the server payload verbatim (for
// scripts and the MCP agent surface), not the rendered table.
func TestRosterCmd_JSONPassthrough(t *testing.T) {
	out := runRosterCmd(t, rosterBody, true)
	if !strings.Contains(out, `"idle"`) || !strings.Contains(out, `"stonequarry"`) {
		t.Errorf("json mode should pass the raw payload through:\n%s", out)
	}
	if strings.Contains(out, "across 2 settlements") {
		t.Errorf("json mode must not render the human table:\n%s", out)
	}
}
