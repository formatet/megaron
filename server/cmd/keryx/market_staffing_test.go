package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestStaffMarketHasNoWorkplacesOrPlacementRequests(t *testing.T) {
	for _, delta := range []string{"", "+1", "-1"} {
		t.Run(delta, func(t *testing.T) {
			mutations := 0
			round3Client(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					mutations++
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				fmt.Fprint(w, `{"total_gubbar":10,"pool_size":10,"hexes":[],"buildings":[]}`)
			})
			cmd := staffCmd()
			args := []string{"market"}
			if delta != "" {
				args = append(args, delta)
			}
			cmd.SetArgs(args)
			out, err := captureStdout(t, cmd.Execute)
			if delta == "" {
				if err != nil || !strings.Contains(out, "no workplaces") || !strings.Contains(out, "needs no workers") {
					t.Fatalf("out=%s err=%v", out, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "no workplaces") || !strings.Contains(err.Error(), "needs no workers") {
				t.Fatalf("err=%v", err)
			}
			if mutations != 0 {
				t.Fatalf("mutations=%d", mutations)
			}
		})
	}
}

func TestStaffMarketJSONExplainsNoWorkplaces(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("market staffing needs no request: %s", r.URL.Path)
	})
	jsonMode = true
	cmd := staffCmd()
	cmd.SetArgs([]string{"market"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || !strings.Contains(out, `"workplaces": 0`) || !strings.Contains(out, "needs no workers") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}
