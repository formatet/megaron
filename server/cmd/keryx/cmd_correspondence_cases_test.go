package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestPersonalCorrespondenceUsesCasesAndPreservesJSONWithoutRead(t *testing.T) {
	for _, detail := range []bool{false, true} {
		for _, raw := range []bool{false, true} {
			t.Run(fmt.Sprintf("detail=%v/json=%v", detail, raw), func(t *testing.T) {
				saved, savedJSON := cfg, jsonMode
				defer func() { cfg, jsonMode = saved, savedJSON }()
				calls := 0
				payload := `{"subject":"Copper terms","items":[{"id":"stable-case","subject":"Copper terms","counterpart_name":"Former Wanax","city_name":"Old Kyme","preview":"My letter","letter_count":2,"sender_name":"Former Wanax","recipient_name":"Recipient","origin_name":"Old Kyme","destination_name":"Former port","message_text":"Private letter","reply_text":"Returned reply"}],"next_cursor":"opaque+/=","future":42}`
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					want := "/api/v1/worlds/w/correspondence/cases"
					if detail {
						want += "/case-id"
					}
					if r.Method != "GET" || r.URL.Path != want || r.URL.Query().Get("cursor") != "old+/=" || r.URL.Query().Get("limit") != "2" {
						t.Errorf("unexpected %s %s", r.Method, r.URL)
					}
					if !detail && r.URL.Query().Get("q") != "Old Kyme & copper" {
						t.Error(r.URL)
					}
					fmt.Fprint(w, payload)
				}))
				defer srv.Close()
				cfg = &Config{Server: srv.URL, WorldID: "w"}
				jsonMode = raw
				cmd := correspondenceCmd()
				args := []string{"--limit", "2", "--cursor", "old+/="}
				if detail {
					args = append(args, "--case", "case-id")
				} else {
					args = append(args, "--search", "Old Kyme & copper")
				}
				cmd.SetArgs(args)
				out, err := captureStdout(t, cmd.Execute)
				if err != nil || calls != 1 {
					t.Fatalf("err=%v calls=%d", err, calls)
				}
				if raw {
					var got, want any
					json.Unmarshal([]byte(out), &got)
					json.Unmarshal([]byte(payload), &want)
					if !reflect.DeepEqual(got, want) {
						t.Fatal(out)
					}
					return
				}
				for _, part := range []string{"Old Kyme", "Former Wanax", "opaque+/="} {
					if !strings.Contains(out, part) {
						t.Fatalf("missing %q: %s", part, out)
					}
				}
				if detail {
					if !strings.Contains(out, "Private letter") || !strings.Contains(out, "Returned reply") || !strings.Contains(out, "--case \"case-id\"") {
						t.Fatal(out)
					}
				} else if !strings.Contains(out, "--search \"Old Kyme & copper\"") {
					t.Fatal(out)
				}
			})
		}
	}
}
func TestPersonalCorrespondenceRejectsAmbiguousFlags(t *testing.T) {
	for _, args := range [][]string{{"--limit", "0"}, {"--limit", "101"}, {"--case", "id", "--search", "name"}} {
		cmd := correspondenceCmd()
		cmd.SetArgs(args)
		if _, err := captureStdout(t, cmd.Execute); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestMessagePreservesExplicitCaseAndSubject(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(fmt.Sprint(host), func(t *testing.T) {
			saved, savedJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = saved, savedJSON }()
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["case_id"] != "stable-case" || body["subject"] != "Copper terms" || body["destination_id"] != "dest" || body["message"] != "Letter" {
						t.Error(body)
					}
					want := "/api/v1/worlds/w/settlements/own/messengers"
					if host {
						want = "/api/v1/worlds/w/founding/messengers"
					}
					if r.URL.Path != want {
						t.Error(r.URL)
					}
					fmt.Fprint(w, `{"id":"letter","case_id":"stable-case"}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/provinces") {
					fmt.Fprint(w, `[{"own":true,"id":"own-province","settlement_id":"own","name":"Own"},{"own":false,"settlement_id":"dest","name":"Kyme"}]`)
				} else {
					fmt.Fprint(w, `[]`)
				}
			}))
			defer srv.Close()
			cfg = &Config{Server: srv.URL, WorldID: "w", ProvinceID: "own-province"}
			jsonMode = true
			cmd := messageCmd()
			args := []string{"--to", "Kyme", "--text", "Letter", "--case", "stable-case", "--subject", "Copper terms"}
			if host {
				args = append(args, "--from-host")
			}
			cmd.SetArgs(args)
			if _, err := captureStdout(t, cmd.Execute); err != nil {
				t.Fatal(err)
			}
			if posts != 1 {
				t.Fatalf("posts=%d", posts)
			}
		})
	}
}
