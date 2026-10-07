package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecallBatchClientParity(t *testing.T) {
	for _, alias := range []bool{false, true} {
		for _, raw := range []bool{false, true} {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if r.Method != "POST" || r.URL.Path != "/api/v1/worlds/world/units/recall" || r.Header.Get("Authorization") != "Bearer test-token" || body["all"] != true || body["target_q"] != float64(5) || body["target_r"] != float64(-3) || body["intent"] != "explore" {
					t.Errorf("request %s %s %+v", r.Method, r.URL, body)
				}
				w.Write([]byte(`{"groups":[{"unit_ids":["a","b"],"q":2,"r":0,"courier_due_tick":9},{"unit_ids":["c"],"q":4,"r":1,"courier_due_tick":10}],"results":[{"name":"Alpha","status":"order_dispatched"},{"name":"Beta","status":"order_dispatched"},{"name":"Gamma","status":"order_dispatched"},{"name":"Delta","status":"rejected","reason":"not marching"}]}`))
			}))
			oldCfg, oldJSON := cfg, jsonMode
			cfg = &Config{Server: srv.URL, WorldID: "world", Token: "test-token"}
			jsonMode = raw
			cmd := unitRecallCmd()
			if alias {
				cmd = recallAliasCmd()
			}
			cmd.SetArgs([]string{"--all", "--target", "5,-3", "--intent", "explore"})
			out, err := captureStdout(t, cmd.Execute)
			cfg, jsonMode = oldCfg, oldJSON
			srv.Close()
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if raw {
				var d recallBatchResponse
				if err := json.Unmarshal([]byte(out), &d); err != nil || len(d.Results) != 4 {
					t.Fatalf("report %s %v", out, err)
				}
			} else {
				for _, want := range []string{"Group:", "Single:", "game day 9", "Alpha:", "Beta:", "Gamma:", "Delta: rejected — not marching"} {
					if !strings.Contains(out, want) {
						t.Fatalf("missing %q: %s", want, out)
					}
				}
			}
		}
	}
}
func TestRecallSelectionValidation(t *testing.T) {
	old := cfg
	cfg = &Config{}
	defer func() { cfg = old }()
	for _, args := range [][]string{{}, {"--all", "--unit", "x"}, {"--unit", "x", "--intent", "explore"}, {"--all", "--target", "oops"}} {
		cmd := unitRecallCmd()
		cmd.SetArgs(args)
		if _, err := captureStdout(t, cmd.Execute); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
