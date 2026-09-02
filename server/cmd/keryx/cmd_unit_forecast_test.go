package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// captureForecastOutput runs renderCatchmentForecast against p and returns
// everything it printed to stdout.
func captureForecastOutput(t *testing.T, title string, p *colonizePreview) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	renderCatchmentForecast(title, p)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

// netForecast builds a minimal colonizePreview whose grain net-per-tick is
// exactly netPerTick, with consPerTick held fixed so the 10%-of-consumption
// marginal threshold is deterministic across the three cases.
func netForecast(consPerTick, netPerTick float64) *colonizePreview {
	p := &colonizePreview{}
	p.Grain.BasePerTick = consPerTick + netPerTick
	p.Grain.EstNetPerTick = netPerTick
	p.Grain.Seed = 600
	// WithFarmPerTick == BasePerTick: no farm terrain known in catchment, so
	// the farmNote branch fires deterministically instead of depending on
	// unset zero values.
	p.Grain.WithFarmPerTick = p.Grain.BasePerTick
	return p
}

// TestRenderCatchmentForecast_ThreeStates is megaron_plan_grundningsprognosen.md
// §4's red-before test: three constructed forecasts (netto −5 · +0,4 · +900)
// must render three DIFFERENT words for the city's state, and none of them
// may ever print the misleading "+0" that %+.0f produces for any netto in
// (-0.5, 0.5).
func TestRenderCatchmentForecast_ThreeStates(t *testing.T) {
	cases := []struct {
		name       string
		cons       float64
		net        float64
		wantWord   string
		forbidWord string
	}{
		// −5/tick: starving outright, well below zero.
		{name: "starving", cons: 20, net: -5, wantWord: "starves"},
		// +0.4/tick with consumption 10 → marginal ceiling is 1.0, so 0.4 is
		// "on the margin", one missed hex from starving — NOT self-sufficient.
		{name: "marginal", cons: 10, net: 0.4, wantWord: "marginal"},
		// +900/tick with consumption 50 → marginal ceiling is 5.0, so 900 is
		// unambiguously self-sufficient.
		{name: "self-sufficient", cons: 50, net: 900, wantWord: "self-sufficient"},
	}

	var outputs []string
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := captureForecastOutput(t, "Test", netForecast(c.cons, c.net))
			outputs = append(outputs, out)
			if !strings.Contains(out, c.wantWord) {
				t.Errorf("net=%.1f cons=%.1f: output missing %q\noutput:\n%s", c.net, c.cons, c.wantWord, out)
			}
			if strings.Contains(out, "+0/tick") || strings.Contains(out, "-0/tick") {
				t.Errorf("net=%.1f: output prints the misleading +0/-0 rounding\noutput:\n%s", c.net, out)
			}
		})
	}

	// The three renders must use three DIFFERENT state words — not just two
	// branches with the marginal case silently sharing wording with either
	// neighbour.
	seen := map[string]bool{}
	for i, c := range cases {
		seen[c.wantWord] = true
		_ = outputs[i]
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct state words across the three forecasts, got %d: %v", len(seen), seen)
	}
}
