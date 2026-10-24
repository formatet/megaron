package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCorrespondencePrivateReadSaveAndConflict(t *testing.T) {
	for _, mode := range []string{"read", "save", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			saved, savedJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = saved, savedJSON }()
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/v1/worlds/w/correspondence/cases/c/private" {
					t.Error(r.URL)
				}
				if mode == "read" {
					if r.Method != "GET" {
						t.Error(r.Method)
					}
				} else {
					if r.Method != "PUT" {
						t.Error(r.Method)
					}
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["draft"] != "My letter" || body["note"] != "My secret" || body["expected_version"] != float64(3) {
						t.Error(body)
					}
				}
				if mode == "conflict" {
					w.WriteHeader(409)
					fmt.Fprint(w, `{"error":"newer saved text; reload"}`)
				} else {
					fmt.Fprint(w, `{"draft":"My letter","note":"My secret","version":4}`)
				}
			}))
			defer srv.Close()
			cfg = &Config{Server: srv.URL, WorldID: "w"}
			jsonMode = false
			cmd := correspondencePrivateCmd()
			args := []string{"--case", "c"}
			if mode != "read" {
				args = append(args, "--draft", "My letter", "--note", "My secret", "--version", "3")
			}
			cmd.SetArgs(args)
			out, err := captureStdout(t, cmd.Execute)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if mode == "conflict" {
				if err == nil || !strings.Contains(err.Error(), "reload") {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range []string{"version 4", "My letter", "My secret"} {
				if !strings.Contains(out, part) {
					t.Fatal(out)
				}
			}
		})
	}
}
func TestCorrespondencePrivateRequiresExplicitCompleteSave(t *testing.T) {
	for _, args := range [][]string{{}, {"--case", "c", "--draft", "letter"}, {"--case", "c", "--note", "note", "--version", "0"}, {"--case", "c", "--draft", "", "--note", "", "--version", "-1"}} {
		cmd := correspondencePrivateCmd()
		cmd.SetArgs(args)
		if _, err := captureStdout(t, cmd.Execute); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
