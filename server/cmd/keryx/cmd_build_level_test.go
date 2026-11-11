package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestBuildListUsesSelectedCityNextPrices(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/worlds/world/provinces":
			fmt.Fprint(w, `[{"id":"petras","name":"Petras"}]`)
		case "/api/v1/worlds/world/provinces/petras":
			fmt.Fprint(w, `{"settlement":{"name":"Petras","build_quotes":[{"type":"shipyard","current_level":1,"next_level":2,"costs":{"cedar":4,"timber":48},"duration_game_days":24}]}}`)
		case "/api/v1/buildings":
			fmt.Fprint(w, `[{"type":"shipyard","costs":{"timber":24},"purpose":"Builds ships"}]`)
		default:
			t.Fatalf("wrong city %s", r.URL.Path)
		}
	})
	cmd := buildCmd()
	cmd.SetArgs([]string{"--list", "Petras"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Petras", "L1", "L2", "cedar×4", "timber×48", "24"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing%s: %s", want, out)
		}
	}
	if strings.Contains(out, "Costs (L1)") {
		t.Fatal(out)
	}
}
func TestBuildErrorKeepsAuthoritativeLevelMessage(t *testing.T) {
	msg := "Raising Shipyard to level 2 needs cedar (need 4, have 0). Petras already has Shipyard level 1."
	err := apiError([]byte(`{"error":"insufficient_goods","message":"`+msg+`","build_context":{"type":"shipyard","current_level":1,"next_level":2},"missing":[{"good":"cedar","need":4,"have":0}]}`), 422)
	if !strings.Contains(err.Error(), msg) {
		t.Fatal(err)
	}
}

func TestBuildListDefaultsToCapitalAndKeepsJSONCatalogue(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/worlds/world/provinces/default":
			fmt.Fprint(w, `{"settlement":{"name":"Capital","build_quotes":[{"type":"shipyard","current_level":1,"next_level":2,"costs":{"cedar":4},"duration_game_days":24}]}}`)
		case "/api/v1/buildings":
			fmt.Fprint(w, `[{"type":"shipyard","costs":{"timber":24}}]`)
		default:
			t.Fatalf("wrong default %s", r.URL.Path)
		}
	})
	jsonMode = true
	cmd := buildCmd()
	cmd.SetArgs([]string{"--list"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || !strings.Contains(out, `"build_quotes"`) || !strings.Contains(out, `"current_level": 1`) || !strings.Contains(out, `"timber": 24`) {
		t.Fatalf("json %s err%v", out, err)
	}
}

// The list uses the player's word: a row reading "silo" sent AI players
// looking for a building status calls Granary (Kleio 2026-11-01).
func TestBuildListNamesTheGranary(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/worlds/world/provinces":
			fmt.Fprint(w, `[{"id":"petras","name":"Petras"}]`)
		case "/api/v1/worlds/world/provinces/petras":
			fmt.Fprint(w, `{"settlement":{"name":"Petras","build_quotes":[{"type":"silo","current_level":0,"next_level":1,"costs":{"stone":8,"timber":12},"duration_game_days":8}]}}`)
		case "/api/v1/buildings":
			fmt.Fprint(w, `[{"type":"silo","costs":{"timber":12},"purpose":"Stores more food"}]`)
		}
	})
	cmd := buildCmd()
	cmd.SetArgs([]string{"--list", "Petras"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || !strings.Contains(out, "\ngranary ") || strings.Contains(out, "silo") {
		t.Fatalf("err%v: %s", err, out)
	}
}

func TestAlreadyMarchingNamesRedirect(t *testing.T) {
	err := alreadyMarchingHint(fmt.Errorf("the unit is already marching; redirect it by choosing a new destination"), "67cfc031", 56, 44)
	if !strings.Contains(err.Error(), "keryx unit redirect --unit 67cfc031 --to 56,44") {
		t.Fatal(err)
	}
	if other := fmt.Errorf("no route"); alreadyMarchingHint(other, "u", 1, 2) != other {
		t.Fatal("other errors must pass through unchanged")
	}
}

// A redirect can carry explore (Timothy 2026-11-01): it goes as a march order.
func TestRedirectCarriesExplore(t *testing.T) {
	const id = "11111111-2222-3333-4444-555555555555"
	var path string
	var body map[string]any
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			path = r.URL.Path
			json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(202)
			fmt.Fprint(w, `{"status":"order_dispatched"}`)
		}
	})
	cmd := unitRedirectCmd()
	cmd.SetArgs([]string{"--unit", id, "--q", "9", "--r", "-1", "--intent", "explore", "--ticks", "20"})
	if _, err := captureStdout(t, cmd.Execute); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "/units/"+id+"/march") || body["intent"] != "explore" || body["ticks"] != float64(20) || body["target_q"] != float64(9) {
		t.Fatalf("%s %v", path, body)
	}
	cmd = unitRedirectCmd()
	cmd.SetArgs([]string{"--unit", id, "--q", "9", "--r", "-1"})
	if _, err := captureStdout(t, cmd.Execute); err != nil || !strings.HasSuffix(path, "/recall") {
		t.Fatalf("plain redirect left the recall path: %s %v", path, err)
	}
}
