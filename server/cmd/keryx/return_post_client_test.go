package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReturnPostOrdinaryInboxHidesClosedReplyWithoutSuppressingLegacy(t *testing.T) {
	saved, savedJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = saved, savedJSON }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			fmt.Fprint(w, `{"read":true}`)
			return
		}
		fmt.Fprint(w, `[{"id":"closed","from_name":"Kyme","message":"Closed letter","status":"delivered","read_at":"read","reply_window_closed":true},{"id":"legacy","from_name":"Kyme","message":"No timer","status":"delivered","read_at":"read"}]`)
	}))
	defer srv.Close()
	cfg = &Config{Server: srv.URL, WorldID: "w"}
	jsonMode = false
	for _, args := range [][]string{nil, {"--history"}} {
		cmd := inboxCmd()
		cmd.SetArgs(args)
		out, err := captureStdout(t, cmd.Execute)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Return post closed") || !strings.Contains(out, "reply --id legacy") || strings.Contains(out, "reply --id closed") {
			t.Fatal(out)
		}
	}
}
