package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCorrespondenceMemoryAndCitationCommands(t *testing.T) {
	for _, mode := range []string{"memory", "cite", "remove"} {
		t.Run(mode, func(t *testing.T) {
			saved, savedJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = saved, savedJSON }()
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				base := "/api/v1/worlds/w/correspondence/cases/c"
				switch mode {
				case "memory":
					if r.Method != "GET" || r.URL.Path != base+"/memory" || r.URL.Query().Get("cursor") != "opaque+/=" {
						t.Error(r.Method, r.URL)
					}
					fmt.Fprint(w, `{"transactions":[{"case_id":"source","subject":"Copper","sender_name":"Sender","recipient_name":"Recipient","terms":{"kind":"sell","offer_good":"copper","offer_qty":2,"want_silver":5}}],"citations":[{"id":"cite","case_id":"source","source":"message","quote":"My exact note"}],"next_cursor":"next+/="}`)
				case "cite":
					if r.Method != "POST" || r.URL.Path != base+"/citations" {
						t.Error(r.Method, r.URL)
					}
					var body map[string]string
					json.NewDecoder(r.Body).Decode(&body)
					if body["letter_id"] != "letter" || body["source"] != "reply" || body["quote"] != "Exact words" {
						t.Error(body)
					}
					fmt.Fprint(w, `{"id":"cite"}`)
				case "remove":
					if r.Method != "DELETE" || r.URL.Path != base+"/citations/cite" {
						t.Error(r.Method, r.URL)
					}
					w.WriteHeader(204)
				}
			}))
			defer srv.Close()
			cfg = &Config{Server: srv.URL, WorldID: "w"}
			jsonMode = false
			var run func() error
			switch mode {
			case "memory":
				cmd := correspondenceMemoryCmd()
				cmd.SetArgs([]string{"--case", "c", "--cursor", "opaque+/="})
				run = cmd.Execute
			case "cite":
				cmd := correspondenceCiteCmd()
				cmd.SetArgs([]string{"--case", "c", "--letter", "letter", "--source", "reply", "--quote", "Exact words"})
				run = cmd.Execute
			case "remove":
				cmd := correspondenceCiteRemoveCmd()
				cmd.SetArgs([]string{"--case", "c", "--id", "cite"})
				run = cmd.Execute
			}
			out, err := captureStdout(t, run)
			if err != nil || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
			if mode == "memory" {
				for _, part := range []string{"Accepted agreements", "My exact note", "delivery outcome unavailable", "next+/="} {
					if !strings.Contains(out, part) {
						t.Fatalf("missing %q: %s", part, out)
					}
				}
			}
		})
	}
}

func TestCorrespondenceExactSourceLetterCommand(t *testing.T) {
	saved, savedJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = saved, savedJSON }()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/v1/worlds/w/correspondence/cases/source-case/letters/old-letter" {
			t.Error(r.Method, r.URL)
		}
		fmt.Fprint(w, `{"id":"source-case","subject":"Old terms","items":[{"id":"old-letter","message_text":"An old exact source","sender_name":"Sender","origin_name":"Old city"}]}`)
	}))
	defer srv.Close()
	cfg = &Config{Server: srv.URL, WorldID: "w"}
	jsonMode = false
	cmd := correspondenceCmd()
	cmd.SetArgs([]string{"--case", "source-case", "--letter", "old-letter"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || calls != 1 || !strings.Contains(out, "An old exact source") {
		t.Fatalf("calls=%d err=%v output=%s", calls, err, out)
	}
	for _, args := range [][]string{{"--letter", "old"}, {"--case", "case", "--letter", "old", "--cursor", "cursor"}} {
		cmd := correspondenceCmd()
		cmd.SetArgs(args)
		if _, err := captureStdout(t, cmd.Execute); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
