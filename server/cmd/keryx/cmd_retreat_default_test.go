package main

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDescribeRetreatDefault(t *testing.T) {
	for _, c := range []struct {
		hold bool
		want string
	}{{true, "Hold to the last man"}, {false, "Retreat when it goes badly"}} {
		if got := describeRetreatDefault(c.hold); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}
func TestRetreatCommandsOnlyTwoModes(t *testing.T) {
	const id = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	for _, unit := range []bool{false, true} {
		for _, mode := range []string{"hold-to-last-man", "when-losing"} {
			t.Run(map[bool]string{false: "realm", true: "unit"}[unit]+"/"+mode, func(t *testing.T) {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					method, path := "PUT", "/api/v1/worlds/world/retreat-default"
					if unit {
						method, path = "POST", "/api/v1/worlds/world/units/"+id+"/standing-orders"
					}
					if r.Method != method || r.URL.Path != path {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					key := strings.ReplaceAll(mode, "-", "_")
					if len(body) != 1 || body[key] != true {
						t.Errorf("retreat command leaked percentage or wrong mode: %v", body)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"hold_to_last_man": mode == "hold-to-last-man", "when_losing": mode == "when-losing"})
				}))
				defer srv.Close()
				oldCfg, oldJSON := cfg, jsonMode
				defer func() { cfg, jsonMode = oldCfg, oldJSON }()
				cfg = &Config{Server: srv.URL, WorldID: "world", Token: "token"}
				jsonMode = false
				cmd := retreatDefaultCmd()
				args := []string{"--" + mode}
				if unit {
					cmd = unitStandingOrdersCmd()
					args = append(args, "--unit", id)
				}
				cmd.SetArgs(args)
				out, err := captureStdout(t, cmd.Execute)
				if err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Errorf("calls=%d want1", calls)
				}
				if !strings.Contains(out, describeRetreatDefault(mode == "hold-to-last-man")) || strings.Contains(out, "retreat_at_loss") {
					t.Errorf("unreadable mode %q", out)
				}
			})
		}
	}
}
func TestRetreatCommandsRejectRetiredAndAmbiguousChoicesBeforeHTTP(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer srv.Close()
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	for _, makeCmd := range []func() *cobra.Command{retreatDefaultCmd, unitStandingOrdersCmd} {
		for _, args := range [][]string{{"--by-loyalty"}, {"--retreat-at-loss", "0.5"}, {"--hold-to-last-man", "--when-losing"}, {"--hold-to-last-man=false"}, {"--when-losing=false"}} {
			cmd := makeCmd()
			if cmd.Name() == "retreat-order" {
				args = append(append([]string{}, args...), "--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			}
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs(args)
			if _, err := captureStdout(t, cmd.Execute); err == nil {
				t.Fatalf("retired or ambiguous retreat mode accepted: %v", args)
			}
		}
	}
	if calls != 0 {
		t.Fatalf("invalid retreat choice sent %d HTTP requests", calls)
	}
}
