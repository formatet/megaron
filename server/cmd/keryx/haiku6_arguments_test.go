package main

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHaiku6MissingArgumentsNeverSendAnOrder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		make        func() *cobra.Command
		args, hints []string
	}{
		{"building mistaken for city", buildCmd, []string{"harbour"}, []string{"--type", "--list"}},
		{"missing workplace", staffCmd, nil, []string{"building", "keryx city"}},
		{"report uses explicit text", reportCmd, []string{"a report with spaces"}, []string{"--text", "'a report with spaces'"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				json.NewEncoder(w).Encode([]any{})
			}))
			defer srv.Close()
			saved := cfg
			defer func() { cfg = saved }()
			cfg = &Config{Server: srv.URL, WorldID: "world", ProvinceID: "source"}
			cmd := tc.make()
			cmd.SetArgs(tc.args)
			_, err := captureStdout(t, cmd.Execute)
			if err == nil {
				t.Fatal("missing argument returned success")
			}
			for _, hint := range tc.hints {
				if !strings.Contains(err.Error(), hint) {
					t.Errorf("error %q lacks %q", err, hint)
				}
			}
			if calls != 0 {
				t.Errorf("invalid order made %d HTTP calls", calls)
			}
		})
	}
}

func TestHaiku6ExplicitReportPostsItsExactText(t *testing.T) {
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/worlds/world/reports" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["body"] != "a report with spaces" || body["kind"] != "confused" {
			t.Errorf("wrong report %v", body)
		}
		posts++
		json.NewEncoder(w).Encode(map[string]any{"tick": 42})
	}))
	defer srv.Close()
	saved, mode := cfg, jsonMode
	defer func() { cfg = saved; jsonMode = mode }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	jsonMode = false
	cmd := reportCmd()
	cmd.SetArgs([]string{"--kind", "confused", "--text", "a report with spaces"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || posts != 1 || !strings.Contains(out, "Report sent") {
		t.Fatalf("posts=%d err=%v output=%s", posts, err, out)
	}
}

func TestHaiku6TransferSelectedColonyAndExplicitSource(t *testing.T) {
	const colony = "colony-province"
	markers := []map[string]any{{"id": colony, "name": "West", "own": true, "settlement_id": "west-settlement"}, {"id": "capital-province", "name": "Mallia", "own": true, "is_capital": true, "settlement_id": "capital-settlement"}}
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/worlds/world/provinces":
			json.NewEncoder(w).Encode(markers)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/trade/destinations"):
			if strings.Contains(r.URL.Path, "/"+colony+"/") {
				json.NewEncoder(w).Encode([]map[string]any{{"name": "Mallia", "settlement_id": "capital-settlement"}})
			} else {
				json.NewEncoder(w).Encode([]map[string]any{{"name": "West", "settlement_id": "west-settlement"}})
			}
		case r.Method == "POST":
			if r.URL.Path != "/api/v1/worlds/world/provinces/capital-province/trade" {
				t.Errorf("wrong source %s", r.URL.Path)
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["destination_id"] != "west-settlement" {
				t.Errorf("wrong destination %v", body)
			}
			posts++
			json.NewEncoder(w).Encode(map[string]any{"arrival_tick": 42, "travel_ticks": 3})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer srv.Close()
	saved, mode := cfg, jsonMode
	defer func() { cfg = saved; jsonMode = mode }()
	cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: colony}
	jsonMode = false
	args := []string{"--good", "grain", "--qty", "5", "--dest", "West"}
	cmd := transferCmd()
	cmd.SetArgs(args)
	_, err := captureStdout(t, cmd.Execute)
	if err == nil || !strings.Contains(err.Error(), "sending city") || !strings.Contains(err.Error(), "--from") {
		t.Fatalf("self destination error=%v", err)
	}
	if posts != 0 || cfg.ProvinceID != colony {
		t.Fatal("self transfer sent an order or changed selected city")
	}
	cmd = transferCmd()
	cmd.SetArgs(append(args, "--from", "Mallia"))
	_, err = captureStdout(t, cmd.Execute)
	if err != nil || posts != 1 || cfg.ProvinceID != colony {
		t.Fatalf("explicit source err=%v posts=%d selected=%s", err, posts, cfg.ProvinceID)
	}
}
