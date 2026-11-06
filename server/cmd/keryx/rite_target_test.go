package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const riteTargetCult = `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[
 {"id":"minoan_stillness_hull","name":"Stout Hulls","god":"The Sea Daimons","god_id":"sea_daimons_stillness","settlement_id":"capital","city_name":"Knossos",
  "offering":{"fish":8,"oil":4},"affordable":true,"can_prepare_offering":true,"scope_cost_factors":{"unit":1,"city":2,"realm":4},
  "targets":{"unit":[{"id":"11111111-1111-1111-1111-111111111111","name":"Horns of Crete","where":"in Knossos"},{"id":"22222222-2222-2222-2222-222222222222","name":"Horns of Naxos","where":"sailing at (3,1)"}],
   "city":{"name":"Knossos","count":2},"realm":{"count":3}}}]}`

func riteTargetServer(t *testing.T, posted *map[string]any) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteTargetCult)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/rite") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			*posted = map[string]any{}
			_ = json.Unmarshal(raw, posted)
			fmt.Fprint(w, `{"success":true,"mood":"Content","prayer":"minoan_stillness_hull","message":"ok"}`)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
}

func TestRiteList_ShowsTargetsAndThePricePerStep(t *testing.T) {
	var posted map[string]any
	riteTargetServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Horns of Crete", "in Knossos", "Horns of Naxos", "--scope unit", "×1", "fish×16 oil×8", "city", "2 units of Knossos", "×4", "fish×32 oil×16", "all 3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--list lacks %q:\n%s", want, out)
		}
	}
}

func TestRiteCast_TargetByNameAndScope(t *testing.T) {
	var posted map[string]any
	riteTargetServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_stillness_hull", "--scope", "unit", "--target", "horns of crete", "--yes"})
	if _, err := captureStdout(t, c.Execute); err != nil {
		t.Fatal(err)
	}
	if posted["target_scope"] != "unit" || posted["target"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("posted %v", posted)
	}
	// An id is passed through; realm needs no target.
	c = riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_stillness_hull", "--scope", "realm", "--yes"})
	if _, err := captureStdout(t, c.Execute); err != nil {
		t.Fatal(err)
	}
	if posted["target_scope"] != "realm" || posted["target"] != nil {
		t.Fatalf("realm posted %v", posted)
	}
}

func TestRiteCast_TargetErrorsNameTheChoices(t *testing.T) {
	var posted map[string]any
	riteTargetServer(t, &posted)
	for name, args := range map[string][]string{
		"unknown name":    {"--prayer", "minoan_stillness_hull", "--scope", "unit", "--target", "Argo", "--yes"},
		"ambiguous name":  {"--prayer", "minoan_stillness_hull", "--scope", "unit", "--target", "horns", "--yes"},
		"bad scope":       {"--prayer", "minoan_stillness_hull", "--scope", "world", "--yes"},
		"unit with no id": {"--prayer", "minoan_stillness_hull", "--scope", "unit", "--yes"},
	} {
		posted = nil
		c := riteCmd()
		c.SetArgs(args)
		_, err := captureStdout(t, c.Execute)
		if err == nil || posted != nil {
			t.Fatalf("%s: err=%v posted=%v — must refuse before casting", name, err, posted)
		}
		if name != "bad scope" && !strings.Contains(err.Error(), "Horns of Crete") {
			t.Fatalf("%s: the error should list the ships: %v", name, err)
		}
	}
}
