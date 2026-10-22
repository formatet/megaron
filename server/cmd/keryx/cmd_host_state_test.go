package main

import (
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFoundingHostPhysicalStatusText(t *testing.T) {
	for _, tc := range []struct {
		status, reason, want string
		can                  bool
	}{
		{"marching", "", "Marching", true}, {"waiting", "This ground is occupied.", "Waiting — This ground is occupied.", false}, {"can_found_here", "", "Can found here", true},
	} {
		q, r := 2, 0
		fp := &foundingStatusResp{Active: true, Q: &q, R: &r, HostState: &foundingHostState{Q: 2, R: 0, Status: tc.status, Reason: tc.reason, CanFoundHere: tc.can}}
		out, err := captureStdout(t, func() error { return printFoundingStatus(fp) })
		if err != nil || !strings.Contains(out, tc.want+" · physical position (2,0)") {
			t.Fatalf("%v %s", err, out)
		}
	}
}

func TestFoundingAndStatusCommandsAgreeOnPhysicalHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/founding/status") {
			w.Write([]byte(`{"active":true,"host_unit_id":"host","q":2,"r":0,"host_state":{"q":2,"r":0,"status":"marching","can_found_here":true},"population":1000}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	jsonMode = false
	for _, cmd := range []*cobra.Command{foundingStatusCmd(), statusCmd()} {
		out, err := captureStdout(t, cmd.Execute)
		if err != nil || !strings.Contains(out, "Marching · physical position (2,0)") {
			t.Fatalf("%v %s", err, out)
		}
	}
}
