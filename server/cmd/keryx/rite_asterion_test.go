package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const riteAsterionCult = `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[
 {"id":"minoan_asterion_thread_v2","name":"The Thread","god":"Asterion","god_id":"asterion","settlement_id":"capital","city_name":"Knossos",
  "offering":{"wine":3,"silver":2},"affordable":true,"can_prepare_offering":true,"scope_cost_factors":{"unit":1,"city":2,"realm":4},
  "targets":{"unit":[{"id":"33333333-3333-3333-3333-333333333333","name":"Caravan to Phaistos","where":"arrives in 4 game days"}],
   "city":{"name":"Knossos","count":1},"realm":{"count":1},"always":true}},
 {"id":"minoan_asterion_horns","name":"The Bull's Horns","god":"Asterion","god_id":"asterion","settlement_id":"capital","city_name":"Knossos",
  "offering":{"livestock":5,"silver":2},"affordable":true,"can_prepare_offering":true,"scope_cost_factors":{"unit":1,"city":2,"realm":4},
  "targets":{"unit":[{"id":"44444444-4444-4444-4444-444444444444","name":"Royal Guard","where":"in Knossos"}],"city":{"name":"Knossos","count":1},"realm":{"count":1}}}]}`

func riteAsterionServer(t *testing.T, posted *map[string]any) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteAsterionCult)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/rite") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			*posted = map[string]any{}
			_ = json.Unmarshal(raw, posted)
			fmt.Fprint(w, `{"success":true,"mood":"Content","prayer":"minoan_asterion_thread_v2","message":"ok"}`)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
}

func TestRiteList_TheThreadNamesCaravansAndTheHornsNameTroops(t *testing.T) {
	var posted map[string]any
	riteAsterionServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Caravan to Phaistos", "arrives in 4 game days", "every land caravan of Knossos", "silver×4 wine×6", "Royal Guard", "the 1 supported by Knossos"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Runner") {
		t.Fatalf("the Thread never reaches a Runner, but --list talks of one:\n%s", out)
	}
}

func TestRiteCast_TheThreadOnANamedCaravan(t *testing.T) {
	var posted map[string]any
	riteAsterionServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_asterion_thread_v2", "--scope", "unit", "--target", "phaistos", "--yes"})
	if _, err := captureStdout(t, c.Execute); err != nil {
		t.Fatal(err)
	}
	if posted["target_scope"] != "unit" || posted["target"] != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("posted %v", posted)
	}
}

func TestCargoNote_TheOwnersCaravanSaysWhenTheThreadIsOn(t *testing.T) {
	m := map[string]any{"blessings": []any{"the Thread, 12 game days"}}
	if got := cargoBlessingNote(m); got != " · the Thread, 12 game days" {
		t.Fatalf("note %q", got)
	}
	if got := cargoBlessingNote(map[string]any{}); got != "" {
		t.Fatalf("a bare caravan has no note, got %q", got)
	}
}
