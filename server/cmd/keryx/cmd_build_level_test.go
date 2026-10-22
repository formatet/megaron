package main

import (
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
