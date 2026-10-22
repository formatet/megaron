package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const cityTargetsFixture = `[
 {"settlement_id":"own","name":"Knossos","own":true,"knowledge":"known","q":0,"r":0},
 {"settlement_id":"known","name":"Pylos","owner":"Nestor","knowledge":"known","q":4,"r":-2},
 {"settlement_id":"rumour","name":"Troy","knowledge":"rumour","q":9,"r":9},
 {"settlement_id":"no-point","name":"Missing point","knowledge":"known"},
 {"settlement_id":"dup1","name":"Argos","own":true,"knowledge":"known","q":2,"r":3},
 {"settlement_id":"dup2","name":"Argos","owner":"Other ruler","knowledge":"known","q":5,"r":6}]`

func TestCityTargetsCommandAndKnowledge(t *testing.T) {
	for _, factory := range []struct {
		name string
		new  func() *cobra.Command
		path string
	}{
		{"march", unitMarchCmd, "/march"}, {"alias", marchAliasCmd, "/march"}, {"post", unitPostCmd, "/march"}, {"redirect", unitRedirectCmd, "/recall"},
	} {
		for _, scenario := range []struct {
			name    string
			args    []string
			wantErr string
			q, r    int
			label   string
		}{
			{"own", []string{"--to", "Knossos"}, "", 0, 0, "Knossos"},
			{"known", []string{"--to", " pylos "}, "", 4, -2, "Pylos"},
			{"coordinates", []string{"--target", "4,-2"}, "", 4, -2, "Pylos"},
			{"unknown", []string{"--to", "Atlantis"}, `you know no city called "Atlantis"`, 0, 0, ""},
			{"rumour", []string{"--to", "Troy"}, `you know no city called "Troy"`, 0, 0, ""},
			{"missing point", []string{"--to", "Missing point"}, `you know no city called`, 0, 0, ""},
			{"ambiguous", []string{"--to", "Argos"}, `ambiguous`, 0, 0, ""},
			{"mixed", []string{"--to", "Pylos", "--q", "4", "--r", "-2"}, `either --to`, 0, 0, ""},
			{"mixed target", []string{"--to", "Pylos", "--target", "4,-2"}, `either --to`, 0, 0, ""},
			{"empty", []string{"--to", " "}, `--to needs`, 0, 0, ""},
		} {
			t.Run(factory.name+"/"+scenario.name, func(t *testing.T) {
				posts := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if testWorldTimeResponse(w, r) {
						return
					}
					if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/cities") {
						w.Write([]byte(cityTargetsFixture))
						return
					}
					if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, factory.path) {
						t.Errorf("unexpected %s %s", r.Method, r.URL)
						http.Error(w, "unexpected", 400)
						return
					}
					posts++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["target_q"] != float64(scenario.q) || body["target_r"] != float64(scenario.r) {
						t.Errorf("destination changed: %+v", body)
					}
					if factory.name == "post" && (body["stance"] != "sentry" || body["intent"] != nil) {
						t.Errorf("post changed meaning: %+v", body)
					}
					w.Write([]byte(`{}`))
				}))
				defer srv.Close()
				oldCfg, oldJSON := cfg, jsonMode
				defer func() { cfg, jsonMode = oldCfg, oldJSON }()
				cfg = &Config{Server: srv.URL, WorldID: "world"}
				jsonMode = false
				cmd := factory.new()
				cmd.SetArgs(append([]string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, scenario.args...))
				out, err := captureStdout(t, cmd.Execute)
				if scenario.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), scenario.wantErr) || posts != 0 {
						t.Fatalf("err=%v posts=%d", err, posts)
					}
					if scenario.name == "ambiguous" {
						for _, label := range []string{"yours (2,3)", "Other ruler (5,6)", "use --target q,r"} {
							if !strings.Contains(err.Error(), label) {
								t.Fatal(err)
							}
						}
					}
				} else if err != nil || posts != 1 || !strings.Contains(out, scenario.label) {
					t.Fatalf("err=%v posts=%d output=%s", err, posts, out)
				}
			})
		}
	}
}

func TestCityTargetsNoPatrolAndReceiptOutage(t *testing.T) {
	if unitPatrolCmd().Flags().Lookup("to") != nil {
		t.Fatal("patrol cannot target a land city")
	}
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			w.Write([]byte(`{}`))
			return
		}
		http.Error(w, "city list unavailable", 503)
	}))
	defer srv.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	jsonMode = false
	cmd := unitMarchCmd()
	cmd.SetArgs([]string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--target", "4,-2"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || posts != 1 || !strings.Contains(out, "(4,-2)") {
		t.Fatalf("sent order reported failed: %v %s posts=%d", err, out, posts)
	}
}

func TestCityTargetMarchPreviewKeepsCityAndCoordinates(t *testing.T) {
	for _, factory := range []func() *cobra.Command{unitMarchCmd, marchAliasCmd} {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if testWorldTimeResponse(w, r) {
				return
			}
			if strings.HasSuffix(r.URL.Path, "/cities") {
				w.Write([]byte(cityTargetsFixture))
				return
			}
			if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/march-preview") || r.URL.Query().Get("target_q") != "4" || r.URL.Query().Get("target_r") != "-2" {
				t.Errorf("preview mutated or changed destination: %s %s", r.Method, r.URL)
			}
			calls++
			w.Write([]byte(`{"available":true,"arrival_tick":104,"duration_ticks":4}`))
		}))
		oldCfg, oldJSON := cfg, jsonMode
		cfg = &Config{Server: srv.URL, WorldID: "world"}
		jsonMode = false
		cmd := factory()
		cmd.SetArgs([]string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--to", "Pylos", "--preview"})
		out, err := captureStdout(t, cmd.Execute)
		cfg, jsonMode = oldCfg, oldJSON
		srv.Close()
		if err != nil || calls != 1 || !strings.Contains(out, "destination Pylos (4,-2)") || !strings.Contains(out, "No order sent") {
			t.Fatalf("%v %d %s", err, calls, out)
		}
	}
}
