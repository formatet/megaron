package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatPlacesNamesRightsKnownHexes(t *testing.T) {
	raw := []byte(`{"places":[{"kind":"river","id":"6,79","name":"Alpheios","hexes":[{"q":6,"r":80}]},{"kind":"massif","id":"1,2","can_name":true,"hexes":[{"q":1,"r":2}]},{"kind":"desert","id":"3,4","hexes":[]},{"kind":"forest","id":"5,6","name":"Dryads"}]}`)
	got, err := formatPlaces(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"river: Alpheios [river 6,79] — known hexes: (6,80)", "mountain massif: unnamed", `keryx places name massif 1,2 "<name>"`, "desert: unnamed", "cedar forest: Dryads"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "(6,79)") {
		t.Fatal("anchor id must not be presented as a known hex")
	}
	empty, err := formatPlaces([]byte(`{"places":[]}`))
	if err != nil || !strings.Contains(empty, "No places known") {
		t.Fatalf("empty: %s %v", empty, err)
	}
}

func TestPlacesCommandsActualRequestsAndJSON(t *testing.T) {
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	for _, tc := range []struct {
		args         []string
		method, path string
		admin        bool
	}{
		{[]string{}, "GET", "/api/v1/worlds/world/places", false},
		{[]string{"name", "--", "river", "-6,79", " Alpheios "}, "POST", "/api/v1/worlds/world/places/river/-6,79/name", false},
		{[]string{"rename", "forest", "5,6", "Dryads", "--key", "test-admin"}, "PUT", "/api/v1/admin/worlds/world/places/forest/5,6/name", true},
		{[]string{"unname", "massif", "1,2", "--key", "test-admin"}, "DELETE", "/api/v1/admin/worlds/world/places/massif/1,2/name", true},
	} {
		t.Run(tc.method, func(t *testing.T) {
			seen := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen++
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Errorf("request %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer token" {
					t.Error("Bearer missing")
				}
				if tc.admin && r.Header.Get("X-Admin-Key") != "test-admin" {
					t.Error("admin key missing")
				}
				if tc.method == "POST" || tc.method == "PUT" {
					var b map[string]string
					if json.NewDecoder(r.Body).Decode(&b) != nil || b["name"] == "" {
						t.Error("name body missing")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.method == "GET" {
					fmt.Fprint(w, `{"places":[],"future_field":true}`)
				} else {
					fmt.Fprint(w, `{"name":"Alpheios","future_field":true}`)
				}
			}))
			defer server.Close()
			cfg = &Config{Server: server.URL, WorldID: "world", Token: "token"}
			jsonMode = true
			cmd := placesCmd()
			cmd.SetArgs(tc.args)
			out, err := captureStdout(t, cmd.Execute)
			if err != nil {
				t.Fatal(err)
			}
			if seen != 1 {
				t.Fatalf("%d requests", seen)
			}
			if !strings.Contains(out, `"future_field"`) {
				t.Fatalf("--json dropped API fields: %s", out)
			}
		})
	}
}

func TestPlaceFirstReachedReadableNotificationAllKinds(t *testing.T) {
	for _, kind := range []string{"river", "massif", "desert", "forest"} {
		raw, _ := json.Marshal(map[string]any{"kind": kind, "unit_name": "Bronze Guard", "place_id": "6,79", "q": 6, "r": 80})
		out := capturePrint(t, func() { printNotificationDetail(nil, notificationItem{Kind: "PlaceFirstReached", Body: raw}) })
		want := fmt.Sprintf("Your Bronze Guard is the first to reach this %s", placeKindLabel(kind))
		if !strings.Contains(out, want) || !strings.Contains(out, fmt.Sprintf(`keryx places name %s 6,79 "<name>"`, kind)) {
			t.Fatalf("missing readable naming right: %s", out)
		}
		if strings.Contains(out, `{"`) || strings.Contains(out, "tick") {
			t.Fatalf("machine text: %s", out)
		}
	}
}

func TestPlacesNameHumanUsesServerNameAndRefusals(t *testing.T) {
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	for _, status := range []int{200, 400, 403, 409} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == 200 {
					fmt.Fprint(w, `{"name":"Alpheios"}`)
				} else {
					fmt.Fprint(w, `{"error":"naming right unavailable"}`)
				}
			}))
			defer server.Close()
			cfg = &Config{Server: server.URL, WorldID: "world"}
			jsonMode = false
			cmd := placesCmd()
			cmd.SetArgs([]string{"name", "river", "6,79", " Alpheios "})
			out, err := captureStdout(t, cmd.Execute)
			if status == 200 {
				if err != nil || !strings.Contains(out, "The river is named Alpheios.") {
					t.Fatalf("server-normalized name missing: %s %v", out, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "naming right unavailable") || strings.Contains(out, "is named") {
				t.Fatalf("refusal was not actionable: %s %v", out, err)
			}
		})
	}
}
