package main

import (
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPursuitDispatchReceiptsUseMeetingEstimate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			w.Write([]byte(`{"status":"order_dispatched","following_unit":true,"pursuit_legs":0,"courier_arrives_at":"2030-01-01T12:30:00Z","catch_up":"at_destination"}`))
			return
		}
		if testWorldTimeResponse(w, r) {
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	jsonMode = false
	uid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	for _, tc := range []struct {
		name string
		make func() *cobra.Command
		args []string
	}{
		{"march", unitMarchCmd, []string{"--unit", uid, "--target", "4,0"}},
		{"recall", unitRecallCmd, []string{"--unit", uid}},
		{"redirect", unitRedirectCmd, []string{"--unit", uid, "--target", "4,0"}},
		{"stance", unitStanceCmd, []string{"--unit", uid, "--stance", "sentry"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.make()
			cmd.SetArgs(tc.args)
			got, err := captureStdout(t, cmd.Execute)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"Runner following the unit", "next meeting"} {
				if !strings.Contains(got, want) {
					t.Errorf("receipt%q lacks%q", got, want)
				}
			}
			for _, untrue := range []string{"the runner reaches it", "Runner arrives", "if it is still present", "receives a redirect order"} {
				if strings.Contains(got, untrue) {
					t.Errorf("receipt promises early delivery%q", got)
				}
			}
		})
	}
}
