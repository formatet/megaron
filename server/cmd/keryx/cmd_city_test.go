package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIsFoodGood(t *testing.T) {
	tests := []struct {
		good string
		want bool
	}{
		{"grain", true},
		{"fish", true},
		{"timber", false},
		{"livestock", false}, // wider diet-variety set, deliberately NOT counted here
		{"cult", false},
	}
	for _, tt := range tests {
		if got := isFoodGood(tt.good); got != tt.want {
			t.Errorf("isFoodGood(%q) = %v, want %v", tt.good, got, tt.want)
		}
	}
}

// runCityCmd points cfg at ts, mocking both endpoints `city` reads
// (/placement-options and /provinces/{id}), and captures cityCmd()'s stdout
// — same harness shape as runWantsCmd (cmd_wants_test.go).
func runCityCmd(t *testing.T, placementOptionsBody, provinceBody string) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "/placement-options") {
			_, _ = w.Write([]byte(placementOptionsBody))
			return
		}
		_, _ = w.Write([]byte(provinceBody))
	}))
	defer ts.Close()

	prevCfg, prevJSON := cfg, jsonMode
	cfg = &Config{Server: ts.URL, WorldID: "world-1", ProvinceID: "prov-1", Token: "t"}
	jsonMode = false
	t.Cleanup(func() { cfg, jsonMode = prevCfg, prevJSON })

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := cityCmd().RunE(nil, nil)
	w.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatalf("RunE: %v", runErr)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

// TestCityCmd_FoodCounts_NoSpeculativeMoveAdvice reproduces the soak
// finding directly (megaron_plan_omfordelningsmatningen.md): a city with 7
// required / 70 placed on grain must show BOTH the surplus figure and WHICH
// catchment hex carries it, so the next step is a `keryx place ... -n` away
// instead of a re-read of `keryx status` plus mental subtraction.
func TestCityCmd_FoodCounts_NoSpeculativeMoveAdvice(t *testing.T) {
	placementOptions := `{
		"hexes": [
			{"hex_q":0,"hex_r":0,"hex_ordinal":1,"terrain":"plains","goods":[
				{"good_key":"grain","rate_per_tick":12.0,"placed":70,"marginal_yield":12.0}
			]},
			{"hex_q":1,"hex_r":0,"hex_ordinal":2,"terrain":"hills","goods":[
				{"good_key":"timber","rate_per_tick":6.0,"cap":4,"placed":0,"marginal_yield":6.0}
			]}
		],
		"buildings": [],
		"total_gubbar": 70,
		"pool_size": 0
	}`
	province := `{"settlement":{
		"food_gubbar_required": 7,
		"food_gubbar_placed": 70,
		"food_self_sufficient": true
	}}`

	out := runCityCmd(t, placementOptions, province)

	if !strings.Contains(out, "7 citizens needed for people on best food slots, 70 on food") || strings.Contains(out, "could move") || strings.Contains(out, "★") {
		t.Fatal(out)
	}
}

// TestCityCmd_NoSurplus_NoMarkerNoClaim is the negative pin: a correctly
// staffed city must print required/placed but never the "could move" text
// or the marker on any row — the informational line must not manufacture a
// surplus that doesn't exist.
func TestCityCmd_NoSurplus_NoMarkerNoClaim(t *testing.T) {
	placementOptions := `{
		"hexes": [
			{"hex_q":0,"hex_r":0,"hex_ordinal":1,"terrain":"plains","goods":[
				{"good_key":"grain","rate_per_tick":12.0,"placed":7,"marginal_yield":12.0}
			]}
		],
		"buildings": [],
		"total_gubbar": 7,
		"pool_size": 0
	}`
	province := `{"settlement":{
		"food_gubbar_required": 7,
		"food_gubbar_placed": 7,
		"food_self_sufficient": true
	}}`

	out := runCityCmd(t, placementOptions, province)

	if strings.Contains(out, "could move") {
		t.Errorf("correctly staffed city must not claim a surplus:\n%s", out)
	}
	if strings.Contains(out, "★") {
		t.Errorf("correctly staffed city must not mark any row as surplus:\n%s", out)
	}
	if !strings.Contains(out, "7 citizens needed for people on best food slots, 7 on food") {
		t.Errorf("output missing the plain required/placed line:\n%s", out)
	}
}
