package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReturnPostQueueUsesSavedTickAndServerClosedFlag(t *testing.T) {
	saved, savedJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = saved, savedJSON }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/messengers/queue") {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `{"items":[{"id":"open","from_name":"Kyme","to_name":"Nostos","message":"Open","queue_group":"waiting_you","actions":["read","reply"],"reply_before":"2000-01-01T00:00:00Z","reply_deadline_tick":102,"reply_window_closed":false},{"id":"closed","from_name":"Kyme","to_name":"Nostos","message":"Closed","queue_group":"completed","actions":["reply"],"reply_window_closed":true},{"id":"legacy","from_name":"Kyme","to_name":"Nostos","message":"No saved timer","queue_group":"waiting_you","actions":["reply"]},{"id":"answered","queue_group":"completed","reply_text":"Already answered","reply_window_closed":true,"actions":[]}]}`)
	}))
	defer srv.Close()
	cfg = &Config{Server: srv.URL, WorldID: "w"}
	jsonMode = false
	cmd := inboxCmd()
	cmd.SetArgs([]string{"--queue"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Return post before " + testTickTime(102), "Return post closed", "Return post deadline unavailable", "reply --id open", "reply --id legacy", "Reply dispatched"} {
		if !strings.Contains(out, text) {
			t.Fatalf("missing %q: %s", text, out)
		}
	}
	if strings.Contains(out, "reply --id closed") || strings.Contains(out, "2000") || strings.Contains(out, "day 102") {
		t.Fatal(out)
	}
}

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
