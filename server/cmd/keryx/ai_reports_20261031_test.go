package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- 1. founding status shows the same forecast as settle -----------------

// Lykon's case: the host stands on (1,78), inland; (0,78) is coastal ground one
// step west. `founding status` must print the forecast (including the harbour
// rule and the nearby coast) without founding anything.
func TestFoundingStatus_PrintsForecastAndNearbyCoast(t *testing.T) {
	var previewQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/founding/status"):
			_, _ = w.Write([]byte(`{"active":true,"q":1,"r":78,"population":3000,"grain":{"amount":900},"silver":{"amount":100},"host_state":{"status":"can_found_here","can_found_here":true}}`))
		case strings.HasSuffix(r.URL.Path, "/colonize-preview"):
			previewQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"catchment":[
				{"q":1,"r":78,"known":true,"terrain":"plains","coastal":false},
				{"q":0,"r":78,"known":true,"terrain":"plains","coastal":true},
				{"q":2,"r":78,"known":true,"terrain":"plains"}],
				"harbour_note":"Inland: this hex touches no water, so a city here can never build a harbour or ships. Only a city founded ON a coastal hex (one that touches sea, lake or river) gets them — move there before settling, or found a coastal colony later.",
				"grain":{"seed":900}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	withTestConfig(t, srv.URL)
	out, err := captureStdout(t, func() error { return foundingStatusCmd().RunE(nil, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(previewQuery, "q=1") || !strings.Contains(previewQuery, "r=78") || !strings.Contains(previewQuery, "pop=3000") || !strings.Contains(previewQuery, "seed=900") {
		t.Errorf("status must ask the preview with the settle parameters, got %q", previewQuery)
	}
	for _, want := range []string{"catchment forecast", "Inland: this hex touches no water", "Only a city founded ON a coastal hex", "Coastal ground is 1 step away at (0,78) — march the host there", "a neighbouring coast does not count"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A coastal centre needs no "coast is near" line, and a coast beyond two steps
// or in fog is not claimed.
func TestCoastalGroundLine(t *testing.T) {
	mk := func(cs ...[4]int) *colonizePreview {
		var rows []map[string]any
		for _, c := range cs {
			rows = append(rows, map[string]any{"q": c[0], "r": c[1], "known": c[2] == 1, "coastal": c[3] == 1})
		}
		raw, _ := json.Marshal(map[string]any{"catchment": rows})
		p := &colonizePreview{}
		if err := json.Unmarshal(raw, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got := coastalGroundLine(mk([4]int{0, 0, 1, 1}, [4]int{1, 0, 1, 1})); got != "" {
		t.Errorf("coastal centre: %q", got)
	}
	if got := coastalGroundLine(mk([4]int{0, 0, 1, 0}, [4]int{2, 0, 1, 1})); !strings.Contains(got, "2 steps away at (2,0)") {
		t.Errorf("two steps: %q", got)
	}
	if got := coastalGroundLine(mk([4]int{0, 0, 1, 0}, [4]int{1, 0, 0, 1})); got != "" {
		t.Errorf("fogged coast must not be claimed: %q", got)
	}
}

func TestFoundingSettleHelpPointsAtStatusForecast(t *testing.T) {
	long := foundingSettleCmd().Long
	for _, want := range []string{"keryx founding status", "without --yes", "stops"} {
		if !strings.Contains(long, want) {
			t.Errorf("settle --help missing %q", want)
		}
	}
}

// ---- 3. small keryx fixes ----------------------------------------------------

func TestLoginHelpNamesPasswordEnv(t *testing.T) {
	if !strings.Contains(loginCmd().Long, "POLEIA_PASSWORD") {
		t.Error("login --help must name POLEIA_PASSWORD")
	}
}

func TestMeBeforeJoinNamesJoinVerb(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not joined to this world"}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	withTestConfig(t, srv.URL)
	err := meCmd().RunE(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "keryx join") {
		t.Fatalf("me before join must name keryx join, got %v", err)
	}
}

func TestMapFrontierMarksHexesNextToSea(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/provinces/home"):
			_ = json.NewEncoder(w).Encode(map[string]any{"map_tile": map[string]int{"q": 0, "r": 0}})
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			_ = json.NewEncoder(w).Encode([]any{})
		case strings.HasSuffix(r.URL.Path, "/map"):
			// (1,0) is known sea; fog (2,0) borders it, fog (-1,0) borders only land.
			_, _ = w.Write([]byte(`[{"q":0,"r":0,"terrain":"plains","visible":true},
				{"q":0,"r":1,"terrain":"plains","visible":true},
				{"q":1,"r":0,"terrain":"coastal_sea","visible":true},
				{"q":2,"r":0,"terrain":"fog","visible":false,"frontier":true},
				{"q":-1,"r":0,"terrain":"fog","visible":false,"frontier":true}]`))
		}
	}))
	defer srv.Close()
	withTestConfig(t, srv.URL)
	cfg.ProvinceID = "home"
	out, err := captureStdout(t, mapCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	var seaLine, landLine string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "(  2,  0)") && strings.Contains(l, "d2") {
			seaLine = l
		}
		if strings.Contains(l, "( -1,  0)") && strings.Contains(l, "d1") {
			landLine = l
		}
	}
	if !strings.Contains(seaLine, "sea") {
		t.Errorf("frontier hex next to known sea must say so: %q\n%s", seaLine, out)
	}
	if strings.Contains(landLine, "sea") || landLine == "" {
		t.Errorf("frontier hex away from sea must not mention sea: %q", landLine)
	}
}

// ---- 4. next game day ---------------------------------------------------------

func TestNextGameDayLine(t *testing.T) {
	current := 100
	anchor := time.Date(2026, 10, 31, 17, 0, 0, 0, keryxTZ)
	c := &Client{tickSecondsFetched: true, tickSeconds: 1800, worldTick: &current, tickAnchor: anchor}
	now := anchor.Add(18 * time.Minute)
	got := nextGameDayLine(c, now)
	if got != "Next game day: today 17:30 (in 12 min)" {
		t.Errorf("got %q", got)
	}
	if strings.Contains(got, "tick") {
		t.Errorf("player text must not say tick: %q", got)
	}
	// An hour or more away reads in hours and minutes.
	far := &Client{tickSecondsFetched: true, tickSeconds: 86400, worldTick: &current, tickAnchor: anchor}
	if got := nextGameDayLine(far, now); !strings.Contains(got, "(in 23 h 42 min)") {
		t.Errorf("far: %q", got)
	}
	// No anchor means no invented time.
	if got := nextGameDayLine(&Client{}, now); got != "" {
		t.Errorf("no anchor: %q", got)
	}
}

// Goods on one hex are always listed alphabetically, whatever order the
// server sends them in (Timothy 2026-10-31: oil was top on one hex, middle on
// another).
func TestCityCmd_GoodsAlphabeticalPerHex(t *testing.T) {
	placementOptions := `{
		"hexes": [
			{"hex_q":0,"hex_r":0,"hex_ordinal":1,"terrain":"plains","goods":[
				{"good_key":"olive_oil","rate_per_tick":2.0,"cap":3,"placed":0,"marginal_yield":2.0},
				{"good_key":"barley","rate_per_tick":2.0,"cap":3,"placed":0,"marginal_yield":2.0},
				{"good_key":"grain","rate_per_tick":2.0,"cap":3,"placed":0,"marginal_yield":2.0}
			]}
		],
		"buildings": [],
		"total_gubbar": 0,
		"pool_size": 0
	}`
	out := runCityCmd(t, placementOptions, `{"settlement":{}}`)
	b, g, o := strings.Index(out, "barley"), strings.Index(out, "grain"), strings.Index(out, "olive_oil")
	if b < 0 || g < 0 || o < 0 || !(b < g && g < o) {
		t.Fatalf("goods not alphabetical:\n%s", out)
	}
}
