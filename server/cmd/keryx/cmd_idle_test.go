package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// runIdleCmd points cfg at ts, mocking both endpoints `idle` reads
// (/placement-options and /provinces/{id}), and captures idleCmd()'s stdout
// — same harness shape as runWantsCmd (cmd_wants_test.go) / runCityCmd
// (cmd_city_test.go).
func runIdleCmd(t *testing.T, placementOptionsBody, provinceBody string) string {
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
	runErr := idleCmd().RunE(nil, nil)
	w.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatalf("RunE: %v", runErr)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

// TestIdleCmd_ZeroIdleButFoodSurplus_StillNamesTheMisallocation is the exact
// blind spot the soak finding named (megaron_plan_omfordelningsmatningen.md):
// a city can show zero idle citizens and still be 5-10x overstaffed on food
// because every gubbe is PLACED, just on the wrong thing. `idle` must not
// let "0 idle" read as "well allocated".
func TestIdleCmd_ZeroIdleButFoodSurplus_StillNamesTheMisallocation(t *testing.T) {
	placementOptions := `{"hexes":[],"buildings":[],"total_gubbar":70,"pool_size":0}`
	province := `{"settlement":{
		"food_gubbar_required": 7,
		"food_gubbar_placed": 70,
		"food_self_sufficient": true
	}}`

	out := runIdleCmd(t, placementOptions, province)

	if !strings.Contains(out, "All 70 citizens are placed.") {
		t.Errorf("output missing the zero-idle line:\n%s", out)
	}
	if !strings.Contains(out, "63") {
		t.Errorf("output must name the 63-citizen surplus even with zero idle:\n%s", out)
	}
	if !strings.Contains(out, "keryx city") {
		t.Errorf("output missing a pointer to `keryx city`:\n%s", out)
	}
}

// TestIdleCmd_ZeroIdleAndCorrectlyStaffed_NoSpuriousClaim is the negative
// pin: a genuinely well-placed city with zero idle must keep its original,
// unqualified "All N citizens are placed." message.
func TestIdleCmd_ZeroIdleAndCorrectlyStaffed_NoSpuriousClaim(t *testing.T) {
	placementOptions := `{"hexes":[],"buildings":[],"total_gubbar":7,"pool_size":0}`
	province := `{"settlement":{
		"food_gubbar_required": 7,
		"food_gubbar_placed": 7,
		"food_self_sufficient": true
	}}`

	out := runIdleCmd(t, placementOptions, province)

	want := "All 7 citizens are placed.\n"
	if out != want {
		t.Errorf("output = %q, want exactly %q (no spurious surplus claim)", out, want)
	}
}

// TestIdleCmd_SomeIdleAndFoodSurplus_ReportsBoth checks the non-zero-idle
// branch also surfaces the food surplus on top of the existing idle count,
// rather than only patching the zero-idle blind spot.
func TestIdleCmd_SomeIdleAndFoodSurplus_ReportsBoth(t *testing.T) {
	placementOptions := `{"hexes":[],"buildings":[],"total_gubbar":80,"pool_size":10}`
	province := `{"settlement":{
		"food_gubbar_required": 7,
		"food_gubbar_placed": 70,
		"food_self_sufficient": true
	}}`

	out := runIdleCmd(t, placementOptions, province)

	if !strings.Contains(out, "10 of 80 citizens are idle.") {
		t.Errorf("output missing the idle line:\n%s", out)
	}
	if !strings.Contains(out, "63") {
		t.Errorf("output must also name the 63-citizen food surplus:\n%s", out)
	}
}
