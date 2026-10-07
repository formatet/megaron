package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRecallAllLoopsEveryMarchingUnit(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		for _, alias := range []bool{false, true} {
			t.Run(map[bool]string{true: "json", false: "text"}[asJSON]+map[bool]string{true: "alias", false: "unit"}[alias], func(t *testing.T) {
				var calls []string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer token" {
						t.Error("missing bearer")
					}
					if r.Method == "GET" && r.URL.Path == "/api/v1/worlds/world/units" {
						w.Write([]byte(`{"units":[{"id":"a","display_name":"Alpha","status":"marching"},{"id":"b","display_name":"Beta","status":"marching"},{"id":"home","status":"garrison"},{"id":"c","display_name":"Gamma","status":"marching"}]}`))
						return
					}
					if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/recall") {
						t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					}
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if len(body) != 0 {
						t.Error("must send existing empty recall body")
					}
					id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/worlds/world/units/"), "/recall")
					calls = append(calls, id)
					if id == "b" {
						w.WriteHeader(409)
						w.Write([]byte(`{"error":"already on its way"}`))
						return
					}
					w.WriteHeader(202)
					w.Write([]byte(`{"status":"order_dispatched"}`))
				}))
				defer srv.Close()
				oldCfg, oldJSON := cfg, jsonMode
				defer func() { cfg, jsonMode = oldCfg, oldJSON }()
				cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world"}
				jsonMode = asJSON
				cmd := unitRecallCmd()
				if alias {
					cmd = recallAliasCmd()
				}
				cmd.SetArgs([]string{"--all"})
				out, err := captureStdout(t, cmd.Execute)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(calls, []string{"a", "b", "c"}) {
					t.Fatalf("must recall every marching unit despite rejection: %v", calls)
				}
				if asJSON {
					var result struct {
						CalledHome int `json:"called_home"`
						Results    []struct {
							Status string `json:"status"`
						}
					}
					if err := json.Unmarshal([]byte(out), &result); err != nil {
						t.Fatal(err)
					}
					if result.CalledHome != 2 || len(result.Results) != 3 || result.Results[1].Status != "rejected" {
						t.Fatalf("bad partial report: %s", out)
					}
				} else {
					for _, word := range []string{"Alpha: recall sent.", "Beta: already on its way", "Gamma: recall sent.", "Two units are being called home."} {
						if !strings.Contains(out, word) {
							t.Errorf("missing %q: %s", word, out)
						}
					}
				}
			})
		}
	}
}

func TestRecallAllNoneAndInvalidFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("must not recall a garrison")
		}
		w.Write([]byte(`{"units":[{"id":"home","status":"garrison"}]}`))
	}))
	defer srv.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	jsonMode = false
	cmd := unitRecallCmd()
	cmd.SetArgs([]string{"--all"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || !strings.Contains(out, "No units are being called home.") {
		t.Fatalf("%s %v", out, err)
	}
	for _, args := range [][]string{{}, {"--all", "--unit", "a"}, {"--all", "--target", "1,2"}, {"--all", "--intent", "explore"}} {
		cmd := unitRecallCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("must reject %v", args)
		}
	}
}
