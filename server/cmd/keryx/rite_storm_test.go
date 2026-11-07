package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const riteStormCult = `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[
 {"id":"minoan_stillness_calm","name":"Still the Storm","god":"The Sea Daimons","god_id":"sea_daimons_stillness","settlement_id":"capital","city_name":"Knossos",
  "offering":{"fish":10,"oil":6},"affordable":true,"can_prepare_offering":true,
  "targets":{"unit":[],"city":{"count":0},"realm":{"count":0},"kind":"storm","storms":[{"id":"aaaaaaaa-0000-0000-0000-000000000001","name":"Old Grey","where":"last seen near (4,4)"}]}},
 {"id":"minoan_stillness_wake","name":"Wake the Storm","god":"The Sea Daimons","god_id":"sea_daimons_stillness","settlement_id":"capital","city_name":"Knossos",
  "offering":{"fish":10,"wine":6},"affordable":true,"can_prepare_offering":true,
  "targets":{"unit":[],"city":{"count":0},"realm":{"count":0},"kind":"hex"}}]}`

func riteStormServer(t *testing.T, posted *map[string]any) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteStormCult)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/rite") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			*posted = map[string]any{}
			_ = json.Unmarshal(raw, posted)
			fmt.Fprint(w, `{"success":true,"mood":"Content","prayer":"x","message":"ok"}`)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
}

func TestRiteList_ShowsNamedStormsAndAsksAHexForTheWake(t *testing.T) {
	var posted map[string]any
	riteStormServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--storm <name>", "Old Grey", "last seen near (4,4)", "--hex q,r"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "×0") {
		t.Fatalf("a storm prayer must not print a price per scope step:\n%s", out)
	}
}

func TestRiteCast_StormByNameAndHexAsGiven(t *testing.T) {
	var posted map[string]any
	riteStormServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_stillness_calm", "--storm", "old grey", "--yes"})
	if _, err := captureStdout(t, c.Execute); err != nil {
		t.Fatal(err)
	}
	if posted["target"] != "aaaaaaaa-0000-0000-0000-000000000001" || posted["target_scope"] != nil {
		t.Fatalf("posted %v", posted)
	}
	c = riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_stillness_wake", "--hex", "12,-3", "--yes"})
	if _, err := captureStdout(t, c.Execute); err != nil {
		t.Fatal(err)
	}
	if posted["target"] != "12,-3" {
		t.Fatalf("posted %v", posted)
	}
	c = riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_stillness_wake", "--hex", "twelve", "--yes"})
	if _, err := captureStdout(t, c.Execute); err == nil {
		t.Fatal("a garbled --hex must be refused before anything is offered")
	}
	c = riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_stillness_calm", "--storm", "Nope", "--yes"})
	if _, err := captureStdout(t, c.Execute); err == nil || !strings.Contains(err.Error(), "Old Grey") {
		t.Fatalf("an unknown storm must list the ones on the map: %v", err)
	}
}
