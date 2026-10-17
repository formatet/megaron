package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMarchPreviewNeverDispatches(t *testing.T) {
	for _, alias := range []bool{false, true} {
		for _, asJSON := range []bool{false, true} {
			for _, reason := range []string{"", "courier_required", "redirect", "unknown_terrain"} {
				t.Run(strings.Join([]string{map[bool]string{false: "unit", true: "alias"}[alias], map[bool]string{false: "text", true: "json"}[asJSON], reason}, "/"), func(t *testing.T) {
					calls := 0
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						if r.Method != "GET" || r.URL.Path != "/api/v1/worlds/world/units/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/march-preview" {
							t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						}
						for k, v := range map[string]string{"target_q": "5", "target_r": "-3", "intent": "land", "cargo_intent": "colonize", "name": "New Thapsos & coast", "stance": "sentry", "mode": "annex"} {
							if r.URL.Query().Get(k) != v {
								t.Errorf("%s=%q want %q", k, r.URL.Query().Get(k), v)
							}
						}
						if r.Header.Get("Authorization") != "Bearer token" {
							t.Error("missing bearer")
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"available": reason == "", "reason": reason, "arrival_tick": 42, "duration_ticks": 3, "arrives_at_utc": "2026-10-06T15:00:00Z", "upkeep_silver_home": 0.6, "upkeep_silver_field": 2, "silver_runway_ticks": 25})
					}))
					defer srv.Close()
					oldCfg, oldJSON := cfg, jsonMode
					defer func() { cfg, jsonMode = oldCfg, oldJSON }()
					cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world"}
					jsonMode = asJSON
					var cmd *cobra.Command
					if alias {
						cmd = marchAliasCmd()
					} else {
						cmd = unitCmd()
					}
					args := []string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--target", "5,-3", "--intent", "land", "--colonize", "--name", "New Thapsos & coast", "--stance", "sentry", "--mode", "annex", "--preview"}
					if !alias {
						args = append([]string{"march"}, args...)
					}
					cmd.SetArgs(args)
					out, err := captureStdout(t, cmd.Execute)
					if err != nil {
						t.Fatal(err)
					}
					if calls != 1 {
						t.Fatalf("calls=%d; preview must not dispatch", calls)
					}
					if asJSON {
						var got map[string]any
						if err := json.Unmarshal([]byte(out), &got); err != nil {
							t.Fatalf("invalid JSON %q: %v", out, err)
						}
						if got["available"] != (reason == "") || got["reason"] != reason {
							t.Fatalf("wrong forecast: %s", out)
						}
					} else {
						want := map[string]string{"": "day 42 (journey: 3 days)", "courier_required": "Runner must deliver", "redirect": "Runner must deliver", "unknown_terrain": "unknown terrain"}[reason]
						if !strings.Contains(out, want) || !strings.Contains(out, "No order sent") || !strings.Contains(out, "Pay: 0.6 → 2 silver per day in the field · treasury lasts 25 days") {
							t.Fatalf("wrong forecast %q", out)
						}
					}
				})
			}
		}
	}
}

func TestMarchPreviewErrorNeverDispatches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" {
			t.Errorf("unexpected mutation: %s", r.Method)
		}
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"unit cannot march"}`))
	}))
	defer srv.Close()
	old := cfg
	defer func() { cfg = old }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	cmd := unitMarchCmd()
	cmd.SetArgs([]string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--target", "0,0", "--intent", "colonize", "--preview"})
	_, err := captureStdout(t, cmd.Execute)
	if err == nil || !strings.Contains(err.Error(), "unit cannot march") || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestMarchSilverForecastZeroAndPaidRunway(t *testing.T) {
	for _, tt := range []struct{ payload, want string }{
		{`{"upkeep_silver_home":0,"upkeep_silver_field":0,"silver_runway_ticks":null}`, "Pay: 0 → 0 silver per day in the field · no silver drain"},
		{`{"upkeep_silver_home":0.6,"upkeep_silver_field":2,"silver_runway_ticks":25}`, "Pay: 0.6 → 2 silver per day in the field · treasury lasts 25 days"},
		{`{"upkeep_silver_home":0.6,"upkeep_silver_field":2,"silver_runway_ticks":0}`, "Pay: 0.6 → 2 silver per day in the field · treasury lasts 0 days"},
		{`{"upkeep_silver_home":1.2,"upkeep_silver_field":4,"silver_runway_ticks":12.5,"silver_payer_count":2}`, "Pay: 1.2 → 4 silver per day in the field · shortest treasury lasts 12.5 days"},
		{`{}`, ""},
	} {
		var f marchSilverForecast
		if err := json.Unmarshal([]byte(tt.payload), &f); err != nil {
			t.Fatal(err)
		}
		if got := marchSilverLine(f); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestMarchSilverForecastRoundsCirculationArithmetic(t *testing.T) {
	gross, share := 2.0, 0.7
	net := gross - gross*share
	runway := 25.0
	got := marchSilverLine(marchSilverForecast{UpkeepSilverHome: &net, UpkeepSilverField: &gross, SilverRunwayTicks: &runway})
	if want := "Pay: 0.6 → 2 silver per day in the field · treasury lasts 25 days"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
