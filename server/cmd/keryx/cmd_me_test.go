package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMeOwnWorldTextAndJSON(t *testing.T) {
	for _, founded := range []any{nil, 0, 37} {
		for _, raw := range []bool{false, true} {
			payload := map[string]any{"wanax_name": "Nestor", "founded_tick": founded, "cities": 2, "population": 320, "kharis": 4.5}
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/api/v1/worlds/my-world/me" || r.Header.Get("Authorization") != "Bearer own-token" {
					t.Errorf("wrong own-world request %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(payload)
			}))
			oldCfg, oldJSON := cfg, jsonMode
			cfg = &Config{Server: srv.URL, WorldID: "my-world", Token: "own-token"}
			jsonMode = raw
			out, err := captureStdout(t, meCmd().Execute)
			cfg, jsonMode = oldCfg, oldJSON
			srv.Close()
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if raw {
				var got map[string]any
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatal(err)
				}
				if len(got) != 5 || got["wanax_name"] != "Nestor" {
					t.Fatalf("wrong JSON profile %s", out)
				}
			} else {
				if len(strings.Split(strings.TrimSpace(out), "\n")) != 5 {
					t.Fatalf("profile must have five readable rows: %q", out)
				}
				want := "Not yet founded"
				if founded == 0 {
					want = "Since day 0"
				}
				if founded == 37 {
					want = "Since day 37"
				}
				for _, text := range []string{"Wanax: Nestor", want, "Cities: 2", "Population: 320", "Kharis: 4.5"} {
					if !strings.Contains(out, text) {
						t.Errorf("missing %q in %q", text, out)
					}
				}
			}
		}
	}
}
