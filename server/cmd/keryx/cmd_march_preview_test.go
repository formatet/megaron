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
			for _, reason := range []string{"", "courier_required", "unknown_terrain"} {
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
						_ = json.NewEncoder(w).Encode(map[string]any{"available": reason == "", "reason": reason, "arrival_tick": 42, "duration_ticks": 3, "arrives_at_utc": "2026-10-06T15:00:00Z"})
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
						want := map[string]string{"": "day 42 (journey: 3 days)", "courier_required": "Runner must deliver", "unknown_terrain": "unknown terrain"}[reason]
						if !strings.Contains(out, want) || !strings.Contains(out, "No order sent") {
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
