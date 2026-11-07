package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const riteOracleCult = `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[
 {"id":"minoan_asterion_bird_oracle","name":"The Bird Oracle","god":"Asterion","god_id":"asterion","settlement_id":"capital","city_name":"Knossos",
  "offering":{"silver":10,"wine":8},"affordable":true,"can_prepare_offering":true,
  "targets":{"unit":[],"city":{"count":0},"realm":{"count":0},"kind":"place",
   "cities":[{"id":"bbbbbbbb-0000-0000-0000-000000000001","name":"Mycenae","where":"held by Agamemnon, near (20,20)"}],
   "places":[{"id":"Sunken Cape","name":"Sunken Cape","where":"cape, near (70,70)"}]}}]}`

func riteOracleServer(t *testing.T, posted *map[string]any) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteOracleCult)
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

func TestRiteList_ShowsTheCitiesAndNamedPlacesTheBirdsCanWatch(t *testing.T) {
	var posted map[string]any
	riteOracleServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--target <name>", "Mycenae", "Sunken Cape", "within 2 hexes", "told nothing"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "×0") || regexpTick.MatchString(out) {
		t.Fatalf("no price per scope step or internal time wanted:\n%s", out)
	}
}

func TestRiteCast_BirdOracleByCityIdOrPlaceName(t *testing.T) {
	var posted map[string]any
	riteOracleServer(t, &posted)
	for target, want := range map[string]string{"mycenae": "bbbbbbbb-0000-0000-0000-000000000001", "Sunken Cape": "Sunken Cape", "sunken": "Sunken Cape"} {
		c := riteCmd()
		c.SetArgs([]string{"--prayer", "minoan_asterion_bird_oracle", "--target", target, "--yes"})
		if _, err := captureStdout(t, c.Execute); err != nil {
			t.Fatal(err)
		}
		if posted["target"] != want || posted["target_scope"] != nil {
			t.Fatalf("--target %q posted %v, want target %q", target, posted, want)
		}
	}
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_asterion_bird_oracle", "--target", "Nowhere", "--yes"})
	if _, err := captureStdout(t, c.Execute); err == nil || !strings.Contains(err.Error(), "Sunken Cape") {
		t.Fatalf("an unknown place must list the ones on the map: %v", err)
	}
}
